package gen4

import (
	"errors"
	"fmt"

	"tallgrass/gen3"
)

// A Gen 4 save is 512 KB: two copies (at 0 and 0x40000) of a "general"
// block (trainer, party) and a "storage" block (PC). Each block ends in a
// footer: ... save counter (u32), block size (u32), magic 0x20060623, block
// ID (u16), CRC16-CCITT of the block before the footer (u16). Footers are 16
// bytes in HeartGold/SoulSilver and 20 in Diamond/Pearl/Platinum, so blocks
// are found by their footers rather than at fixed offsets; that also copes
// with hacks that resize them.
const (
	saveSize    = 0x80000
	footerMagic = 0x20060623
)

// Save is the newest valid copy of each block.
type Save struct {
	General, Storage []byte
	Counter          uint32 // save counter of the general block
}

type block struct {
	data    []byte
	counter uint32
}

// ParseSave reads a raw save, or a DraStic/DeSmuME .dsv (the raw save plus
// a 122-byte footer).
func ParseSave(data []byte) (*Save, error) {
	if len(data) < saveSize {
		return nil, fmt.Errorf("save is %d bytes; a Gen 4 save is %d", len(data), saveSize)
	}
	data = data[:saveSize]

	best := map[uint16]block{}
	for at := 8; at+8 <= len(data); at += 4 {
		if le.Uint32(data[at:]) != footerMagic {
			continue
		}
		size := int(le.Uint32(data[at-4:]))
		id := le.Uint16(data[at+4:])
		crc := le.Uint16(data[at+6:])
		start := at + 8 - size
		if size < 0x100 || start < 0 || id > 1 {
			continue
		}
		for _, footer := range []int{0x10, 0x14} {
			if crc16(data[start:start+size-footer]) != crc {
				continue
			}
			counter := le.Uint32(data[at-8:])
			if b, ok := best[id]; !ok || counter > b.counter {
				best[id] = block{data[start : start+size-footer], counter}
			}
			break
		}
	}
	general, ok := best[0]
	if !ok {
		return nil, errors.New("no valid Gen 4 save block found")
	}
	return &Save{General: general.data, Storage: best[1].data, Counter: general.counter}, nil
}

// crc16 is CRC16-CCITT (polynomial 0x1021, initial 0xFFFF).
func crc16(b []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, x := range b {
		crc ^= uint16(x) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// Trainer returns the player's name and combined ID (trainer ID | secret ID << 16).
func (s *Save) Trainer(l Layout) (string, uint32) {
	g := s.General
	return DecodeText(g[l.TrainerName : l.TrainerName+16]), le.Uint32(g[l.TrainerName+16:])
}

func (s *Save) Party(l Layout) ([]*gen3.Pokemon, error) { return s.party(l, Decode) }

func (s *Save) party(l Layout, decode func([]byte) (*gen3.Pokemon, error)) ([]*gen3.Pokemon, error) {
	count := min(int(s.General[l.Party-4]), 6)
	var party []*gen3.Pokemon
	for i := 0; i < count; i++ {
		off := l.Party + i*PartySize
		p, err := decode(s.General[off : off+PartySize])
		if err != nil {
			return nil, fmt.Errorf("party slot %d: %w", i+1, err)
		}
		party = append(party, p)
	}
	return party, nil
}

// Boxes is how many PC boxes this save has.
func (s *Save) Boxes(l Layout) int { return l.boxCount(len(s.Storage)) }

func (s *Save) Box(l Layout, box int) ([]*gen3.Pokemon, error) {
	mons := make([]*gen3.Pokemon, gen3.BoxSlots)
	for slot := range mons {
		off := l.BoxStart + box*l.BoxStride + slot*BoxSize
		if off+BoxSize > len(s.Storage) {
			return nil, fmt.Errorf("box %d is outside PC storage", box+1)
		}
		p, err := Decode(s.Storage[off : off+BoxSize])
		if err != nil {
			return nil, fmt.Errorf("box %d slot %d: %w", box+1, slot+1, err)
		}
		mons[slot] = p
	}
	return mons, nil
}

func (s *Save) BoxName(l Layout, box int) string {
	off := l.BoxStart + l.boxCount(len(s.Storage))*l.BoxStride + l.NamesGap + box*0x28
	if off+0x28 > len(s.Storage) {
		return ""
	}
	return DecodeText(s.Storage[off : off+0x28])
}
