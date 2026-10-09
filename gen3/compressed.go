package gen3

import (
	"encoding/binary"
	"fmt"
)

// CompressedSize is CFRU's PC box format (Radical Red and other CFRU hacks):
// the usual 28-byte header, then the growth/attacks/EVs/misc fields packed
// into 30 bytes, unencrypted. PP, contest stats and ribbons aren't stored.
//
//	0x1C species u16    0x1E item u16      0x20 experience u32
//	0x24 PP bonuses     0x25 friendship    0x26 Poké Ball
//	0x27 4 moves × 10 bits (5 bytes)       0x2C EVs (6 bytes)
//	0x32 Pokérus        0x33 met location  0x34 origins u16
//	0x36 IVs, egg and ability bits u32
const CompressedSize = 58

// DecodeCompressed parses a CFRU compressed box Pokémon; nil for an empty
// slot. Level stays zero (see Levels), and moves have no PP.
func DecodeCompressed(b []byte) (*Pokemon, error) {
	if len(b) < CompressedSize {
		return nil, fmt.Errorf("need %d bytes, got %d", CompressedSize, len(b))
	}
	le := binary.LittleEndian
	p := &Pokemon{
		Personality: le.Uint32(b[0:4]),
		OTID:        le.Uint32(b[4:8]),
		Nickname:    DecodeText(b[8:18]),
		OTName:      DecodeText(b[20:27]),
		Species:     int(le.Uint16(b[0x1C:])),
	}
	if p.Species == 0 {
		return nil, nil
	}
	p.HeldItem = int(le.Uint16(b[0x1E:]))
	p.Experience = le.Uint32(b[0x20:])
	p.Friendship = int(b[0x25])

	// Five bytes hold four 10-bit move IDs, lowest bits first.
	moves := uint64(le.Uint32(b[0x27:])) | uint64(b[0x2B])<<32
	for i := 0; i < 4; i++ {
		if id := int(moves >> (10 * i) & 0x3FF); id != 0 {
			p.Moves = append(p.Moves, Move{ID: id})
		}
	}

	ev := b[0x2C:0x32]
	p.EVs = Stats{HP: int(ev[0]), Attack: int(ev[1]), Defense: int(ev[2]),
		Speed: int(ev[3]), SpAtk: int(ev[4]), SpDef: int(ev[5])}
	p.MetLocation = int(b[0x33])
	p.MetLevel = int(le.Uint16(b[0x34:]) & 0x7F)
	p.setIVsAndPersonalityTraits(le.Uint32(b[0x36:]))
	return p, nil
}
