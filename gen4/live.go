package gen4

import (
	"errors"

	"tallgrass/gen3"
)

// The DS's 4 MB of main RAM. DraStic maps it at this same address in its
// own process, so DS addresses can be read directly from /proc/<pid>/mem.
const (
	MainRAM     = 0x02000000
	MainRAMSize = 0x400000
)

// FindGeneral scans main RAM for the live copy of the general save block,
// returning its address. The block moves between sessions, so it's found by
// content: a party count of 1-6, a valid party Pokémon after it, and the
// trainer ID at the family's offset matching one of the party's OT IDs.
func FindGeneral(ram []byte, base uint32, l Layout) (uint32, error) {
	for o := l.Party; o+PartySize <= len(ram); o += 4 {
		count := int(ram[o-4])
		if count < 1 || count > 6 || o+count*PartySize > len(ram) {
			continue
		}
		if ram[o-3] != 0 || ram[o-2] != 0 || ram[o-1] != 0 {
			continue // the count is a u32
		}
		general := o - l.Party
		if CheckGeneral(ram[general:], l) {
			return base + uint32(general), nil
		}
	}
	return 0, errors.New("no party found in main RAM")
}

// CheckGeneral reports whether b starts with a plausible general block.
// It's strict (checksums must match, the trainer name must be letters) so
// that scanning 4 MB of RAM can't stumble on something that merely looks
// right; the found block is then read with DecodeLive.
func CheckGeneral(b []byte, l Layout) bool {
	if len(b) < l.Party+PartySize {
		return false
	}
	if c := le.Uint16(b[l.TrainerName:]); c < 0x012B || c >= 0x0145+26 {
		return false // a trainer name starts with a letter
	}
	trainer := le.Uint32(b[l.TrainerName+16:])
	count := int(b[l.Party-4])
	if count < 1 || count > 6 || len(b) < l.Party+count*PartySize {
		return false
	}
	for i := 0; i < count; i++ {
		p, err := Decode(b[l.Party+i*PartySize : l.Party+(i+1)*PartySize])
		if err != nil || p == nil {
			return false
		}
		if p.OTID == trainer {
			return true
		}
	}
	return false
}

// GeneralSize is how much of the general block live reads need.
func (l Layout) GeneralSize() int { return l.Party + 6*PartySize }

// LiveParty decodes the party from a live general block.
func LiveParty(general []byte, l Layout) (name string, id uint32, party []*gen3.Pokemon, err error) {
	s := &Save{General: general}
	name, id = s.Trainer(l)
	party, err = s.party(l, DecodeLive)
	return name, id, party, err
}

// ReadLiveParty reads the party from a running game, retrying a few times:
// a read can catch the game halfway through re-encrypting a Pokémon.
func ReadLiveParty(read func(addr uint32, n int) ([]byte, error), general uint32, l Layout) (string, uint32, []*gen3.Pokemon, error) {
	var lastErr error
	for try := 0; try < 8; try++ {
		g, err := read(general, l.GeneralSize())
		if err != nil {
			return "", 0, nil, err
		}
		name, id, party, err := LiveParty(g, l)
		if err == nil {
			return name, id, party, nil
		}
		lastErr = err
	}
	return "", 0, nil, lastErr
}
