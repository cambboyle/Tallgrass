package gen3

import (
	"encoding/binary"
	"fmt"
)

// Pokémon R.O.W.E. (built from the pokeemerald decomp) stores Pokémon in
// its own packed format, unencrypted: 52 bytes in the PC, 76 in the party.
// Worked out from the game's GetMonData/GetBoxMonData (ROWE 2.1.1,
// 0x08097938 / 0x08097AFC):
//
//	0x00 personality u32       0x04 OT ID u32       0x08 nickname (10)
//	0x14 u32: move 1 (bits 0–9), experience (10–30), shiny (31)
//	0x18 u32: move 2 (0–9), move 3 (10–19)
//	0x1A u16: friendship (bits 6–13)
//	0x1C u32: species (bits 0–11), form (12–16)
//	0x1E u16: move 4 (bits 1–10)
//	0x20 EVs (6 bytes)         0x26 held item (10 bits)
//	0x27 egg (bit 7)           0x28 met level (bits 0–6), Poké Ball (7–11)
//	0x2A met location          0x2B OT name (7)
//	0x32 markings (bits 0–3), ability slot (4–5), OT gender (6)
//
// The party adds PP at 0x34, status 0x38, level 0x3C, mail 0x3D, HP 0x3E
// and the six stats from 0x40. The game reports every IV as 0, and stats
// match that, so IVs stay zero. Nature isn't stored apart from
// personality as far as the code shows.
const (
	ROWEBoxSize   = 52
	ROWEPartySize = 76
)

// DecodeROWE parses a ROWE Pokémon (PC or party); nil for an empty slot.
// Species is the base species; Form says which form (see ResolveForm).
func DecodeROWE(b []byte) (*Pokemon, error) {
	if len(b) < ROWEBoxSize {
		return nil, fmt.Errorf("need %d bytes, got %d", ROWEBoxSize, len(b))
	}
	le := binary.LittleEndian
	w14, w18, w1C := le.Uint32(b[0x14:]), le.Uint32(b[0x18:]), le.Uint32(b[0x1C:])
	p := &Pokemon{
		Personality: le.Uint32(b[0:4]),
		OTID:        le.Uint32(b[4:8]),
		Nickname:    DecodeText(b[8:18]),
		OTName:      DecodeText(b[0x2B:0x32]),
		Species:     int(w1C & 0xFFF),
		Form:        int(w1C >> 12 & 0x1F),
	}
	if p.Species == 0 {
		return nil, nil
	}
	p.HeldItem = int(le.Uint16(b[0x26:]) & 0x3FF)
	p.Experience = w14 >> 10 & 0x1FFFFF
	p.Friendship = int(le.Uint16(b[0x1A:]) >> 6 & 0xFF)
	for _, id := range []uint32{w14 & 0x3FF, w18 & 0x3FF, w18 >> 10 & 0x3FF, uint32(le.Uint16(b[0x1E:])) >> 1 & 0x3FF} {
		if id != 0 {
			p.Moves = append(p.Moves, Move{ID: int(id)})
		}
	}
	ev := b[0x20:0x26]
	p.EVs = Stats{HP: int(ev[0]), Attack: int(ev[1]), Defense: int(ev[2]), Speed: int(ev[3]), SpAtk: int(ev[4]), SpDef: int(ev[5])}
	p.MetLocation = int(b[0x2A])
	p.MetLevel = int(b[0x28] & 0x7F)

	p.setIVsAndPersonalityTraits(0)
	p.IsEgg = b[0x27]&0x80 != 0
	p.AbilitySlot = int(b[0x32] >> 4 & 3)
	p.IsShiny = p.IsShiny || w14>>31 != 0

	if len(b) >= ROWEPartySize {
		for i := range p.Moves {
			p.Moves[i].PP = int(b[0x34+i])
		}
		p.Status = le.Uint32(b[0x38:])
		p.Condition = condition(p.Status)
		p.Level = int(b[0x3C])
		p.CurrentHP = int(le.Uint16(b[0x3E:]))
		s := func(o int) int { return int(le.Uint16(b[o:])) }
		p.Stats = Stats{HP: s(0x40), Attack: s(0x42), Defense: s(0x44), Speed: s(0x46), SpAtk: s(0x48), SpDef: s(0x4A)}
	}
	return p, nil
}

// ResolveForm turns a base species and form into the game's species ID
// for that form (Alolan Meowth rather than Meowth), using a table of
// pointers, one per species, to u16 lists of form species ending 0xFFFF.
// Species without forms have a null pointer.
func ResolveForm(read ReadMemory, table uint32, p *Pokemon) error {
	if p == nil || p.Form == 0 || table == 0 {
		return nil
	}
	ptr, err := read(table+uint32(p.Species)*4, 4)
	if err != nil {
		return err
	}
	list := binary.LittleEndian.Uint32(ptr)
	if list == 0 {
		return nil
	}
	b, err := read(list, 2*(p.Form+1))
	if err != nil {
		return err
	}
	for i := 0; i <= p.Form; i++ {
		if binary.LittleEndian.Uint16(b[2*i:]) == 0xFFFF {
			return nil // a form the table doesn't list: keep the base species
		}
	}
	p.Species = int(binary.LittleEndian.Uint16(b[2*p.Form:]))
	return nil
}

// DecodeAs parses a Pokémon in a profile's format: "" (vanilla or CFRU
// party layout, encrypted or not) or "rowe".
func DecodeAs(format string, b []byte, encrypted bool) (*Pokemon, error) {
	if format == "rowe" {
		return DecodeROWE(b)
	}
	return Decode(b, encrypted)
}
