package gen3

import (
	"encoding/binary"
	"fmt"
)

// A Gen 3 save is two slots of 14 sectors, written alternately. Each 4 KB
// sector ends with a footer: section ID, checksum, signature, save counter.
// Section 0 is SaveBlock2 (trainer), 1-4 SaveBlock1 (world, party), 5-13
// PC storage. Sections hold SectorDataSize bytes of their block each.
const (
	sectorSize      = 0x1000
	sectorsPerSlot  = 14
	saveSignature   = 0x08012025
	footerID        = 0xFF4
	footerChecksum  = 0xFF6
	footerSignature = 0xFF8
	footerCounter   = 0xFFC
)

// SaveLayout describes how a game packs its save.
type SaveLayout struct {
	Signature      uint32 // footer signature; 0 means vanilla's 0x08012025 (Unbound uses 0x01121999)
	PartyOffset    int    // party in SaveBlock1, count 4 bytes before; 0 means FireRed's 0x38
	SectorDataSize int    // vanilla 0xF80; CFRU hacks use 0xFF0
	// If set, sector checksums must match over these block sizes; a slot
	// that fails falls back to the other one. CFRU hacks write extra data
	// outside the checksummed range, so they leave this off.
	VerifyChecksums                     bool
	Block2Size, Block1Size, StorageSize int
	// ExtraSectors are sectors outside the two save slots that a hack uses
	// for more data (Radical Red keeps boxes 20–22 in sectors 30 and 31).
	// They're joined in order into Save.Extra.
	ExtraSectors []int
}

// Save is one slot of a save file, reassembled.
type Save struct {
	PartyOffset int    // where the party is in Block1 (FireRed 0x38, Emerald 0x238)
	Counter     uint32 // increments on every in-game save
	Block2      []byte // trainer
	Block1      []byte // world, party, items
	Storage     []byte // PC boxes
	Extra       []byte // the layout's ExtraSectors, joined
}

// ParseSave picks the newest valid slot.
func ParseSave(data []byte, layout SaveLayout) (*Save, error) {
	if len(data) < 2*sectorsPerSlot*sectorSize {
		return nil, fmt.Errorf("save is %d bytes, too small for a Gen 3 save", len(data))
	}
	var best *Save
	var errs []error
	for slot := 0; slot < 2; slot++ {
		s, err := parseSlot(data[slot*sectorsPerSlot*sectorSize:], layout)
		if err != nil {
			errs = append(errs, fmt.Errorf("slot %d: %w", slot+1, err))
			continue
		}
		if best == nil || s.Counter > best.Counter {
			best = s
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no valid save slot: %v", errs)
	}
	size := layout.SectorDataSize
	for _, sec := range layout.ExtraSectors {
		if (sec+1)*sectorSize <= len(data) {
			best.Extra = append(best.Extra, data[sec*sectorSize:sec*sectorSize+size]...)
		}
	}
	return best, nil
}

func parseSlot(data []byte, layout SaveLayout) (*Save, error) {
	le := binary.LittleEndian
	size := layout.SectorDataSize
	sections := map[int][]byte{}
	var counter uint32
	for i := 0; i < sectorsPerSlot; i++ {
		sec := data[i*sectorSize : (i+1)*sectorSize]
		sig := le.Uint32(sec[footerSignature:])
		if layout.Signature != 0 && sig != layout.Signature || layout.Signature == 0 && !knownSignature(sig) {
			return nil, fmt.Errorf("sector %d has no save signature", i)
		}
		id := int(le.Uint16(sec[footerID:]))
		if id >= sectorsPerSlot {
			return nil, fmt.Errorf("sector %d has section ID %d", i, id)
		}
		counter = le.Uint32(sec[footerCounter:])
		sections[id] = sec
	}
	if len(sections) != sectorsPerSlot {
		return nil, fmt.Errorf("only %d of %d sections present", len(sections), sectorsPerSlot)
	}

	join := func(first, last, total int) ([]byte, error) {
		var out []byte
		for id := first; id <= last; id++ {
			sec := sections[id]
			if layout.VerifyChecksums {
				n := min(size, total-len(out))
				if got, want := sectorChecksum(sec[:n]), le.Uint16(sec[footerChecksum:]); got != want {
					return nil, fmt.Errorf("section %d checksum %04X, want %04X", id, got, want)
				}
			}
			out = append(out, sec[:size]...)
		}
		return out, nil
	}

	s := &Save{Counter: counter, PartyOffset: layout.PartyOffset}
	var err error
	if s.Block2, err = join(0, 0, layout.Block2Size); err != nil {
		return nil, err
	}
	if s.Block1, err = join(1, 4, layout.Block1Size); err != nil {
		return nil, err
	}
	if s.Storage, err = join(5, 13, layout.StorageSize); err != nil {
		return nil, err
	}
	return s, nil
}

// knownSignature: vanilla's, and Unbound's (other CFRU hacks may share it).
func knownSignature(sig uint32) bool { return sig == saveSignature || sig == 0x01121999 }

// sectorChecksum sums 32-bit words, then folds the halves together.
func sectorChecksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+4 <= len(b); i += 4 {
		sum += binary.LittleEndian.Uint32(b[i:])
	}
	return uint16(sum>>16) + uint16(sum)
}

// Trainer returns the player's name and trainer ID.
func (s *Save) Trainer() (string, uint32) {
	return DecodeText(s.Block2[0:8]), binary.LittleEndian.Uint32(s.Block2[0x0A:])
}

// Party decodes the party (count 4 bytes before PartyOffset in SaveBlock1).
func (s *Save) Party(encrypted bool) ([]*Pokemon, error) {
	at := s.PartyOffset
	if at == 0 {
		at = 0x38
	}
	count := min(int(s.Block1[at-4]), 6)
	var party []*Pokemon
	for i := 0; i < count; i++ {
		p, err := Decode(s.Block1[at+i*PartySize:], encrypted)
		if err != nil {
			return nil, err
		}
		party = append(party, p)
	}
	return party, nil
}

// BoxSlots is the number of Pokémon per PC box.
const BoxSlots = 30

// CurrentBox is the box the PC opens on (0-based).
func (s *Save) CurrentBox() int { return int(s.Storage[0]) }

// BoxFormat is how a game stores PC box Pokémon.
type BoxFormat struct {
	Size      int  // BoxSize (vanilla, 80) or CompressedSize (CFRU, 58)
	Encrypted bool // vanilla only
}

func (f BoxFormat) decode(b []byte) (*Pokemon, error) {
	if f.Size == CompressedSize {
		return DecodeCompressed(b)
	}
	return Decode(b, f.Encrypted)
}

// Box decodes one PC box (0-based); empty slots are nil. Boxes follow each
// other from offset 4 of PC storage.
func (s *Save) Box(box int, format BoxFormat) ([]*Pokemon, error) {
	mons := make([]*Pokemon, BoxSlots)
	for slot := range mons {
		off := 4 + (box*BoxSlots+slot)*format.Size
		if off+format.Size > len(s.Storage) {
			return nil, fmt.Errorf("box %d is outside PC storage", box+1)
		}
		p, err := format.decode(s.Storage[off : off+format.Size])
		if err != nil {
			return nil, err
		}
		mons[slot] = p
	}
	return mons, nil
}

// BoxName reads a box's name (0-based box). Names are 9 bytes; boxes 1-14
// sit from namesOffset onwards as in vanilla. CFRU's extra boxes (15+) are
// stored just before Box 1's name in reverse order, so Box 15 is at
// namesOffset-9.
// Data returns a block by name: "block1", "block2", "storage" or "extra".
func (s *Save) Data(block string) []byte {
	switch block {
	case "block1":
		return s.Block1
	case "block2":
		return s.Block2
	case "storage":
		return s.Storage
	case "extra":
		return s.Extra
	}
	return nil
}

// BoxAt reads a box stored outside PC storage: 30 Pokémon back to back from
// offset in the named block (see Data).
func (s *Save) BoxAt(block string, offset int, format BoxFormat) ([]*Pokemon, error) {
	data := s.Data(block)
	mons := make([]*Pokemon, BoxSlots)
	for slot := range mons {
		off := offset + slot*format.Size
		if off < 0 || off+format.Size > len(data) {
			return nil, fmt.Errorf("box is outside the %s block", block)
		}
		p, err := format.decode(data[off : off+format.Size])
		if err != nil {
			return nil, err
		}
		mons[slot] = p
	}
	return mons, nil
}

func (s *Save) BoxName(box, namesOffset int) string {
	off := namesOffset + box*9
	if box >= 14 {
		off = namesOffset - (box-13)*9
	}
	if off < 0 || off+9 > len(s.Storage) {
		return ""
	}
	return DecodeText(s.Storage[off : off+9])
}

// Block returns SaveBlock1 or SaveBlock2 by number, for profile offsets.
func (s *Save) Block(n int) []byte {
	if n == 2 {
		return s.Block2
	}
	return s.Block1
}
