package gen3

import (
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
)

// TrainerTables locates trainer data in a Gen 3 ROM (gTrainers and
// gTrainerClassNames; the same layout in FireRed, Emerald and CFRU hacks).
type TrainerTables struct {
	Trainers     uint32
	ClassNames   uint32
	ClassNameLen int    // 13
	Format       string // "" (vanilla/CFRU) or "rowe" (see allROWE)
}

// Trainer is one trainer battle as the docs show it.
type Trainer struct {
	ID     int          `json:"id"`
	Class  string       `json:"class"`
	Name   string       `json:"name"`
	Double bool         `json:"double,omitempty"`
	Items  []string     `json:"items,omitempty"` // what they use in battle
	Party  []TrainerMon `json:"party"`
	Areas  []string     `json:"areas,omitempty"` // where they stand, when found
	// Maps where they stand, as "group.num"; used to list a map's trainers.
	Maps []string `json:"maps,omitempty"`
}

type TrainerMon struct {
	Species     int      `json:"species"`
	SpeciesName string   `json:"speciesName"`
	Level       int      `json:"level"` // 0 when the game sets it at battle time (scales with the player)
	Item        string   `json:"item,omitempty"`
	Moves       []string `json:"moves,omitempty"` // empty: its level-up moves
}

const trainerSize = 40

// Party entry flags: custom moves and/or a held item.
const (
	partyCustomMoves = 1
	partyHeldItem    = 2
)

// All reads every trainer. Trainer 0 is a blank placeholder; the
// table ends at the first entry that doesn't look like a trainer.
func (t TrainerTables) All(read ReadMemory, names *Names) ([]Trainer, error) {
	if t.Format == "rowe" {
		return t.allROWE(read, names)
	}
	le := binary.LittleEndian
	var out []Trainer
	for id := 1; id < 4096; id++ {
		e, err := read(t.Trainers+uint32(id*trainerSize), trainerSize)
		if err != nil {
			break
		}
		flags, class, size, party := e[0], int(e[1]), int(e[32]), le.Uint32(e[36:])
		if flags > 3 || size > 6 || (size > 0 && !isROMPointer(party)) {
			break
		}
		if size == 0 {
			continue // unused slot
		}
		tr := Trainer{ID: id, Double: le.Uint32(e[24:])&1 != 0, Party: []TrainerMon{}}
		tr.Class, _ = ReadName(read, t.ClassNames+uint32(class*t.ClassNameLen), t.ClassNameLen)
		tr.Class = strings.NewReplacer("PKMN", "Pokémon", "Pkmn", "Pokémon").Replace(tr.Class)
		tr.Name = TitleCase(DecodeText(e[4:16]))
		for i := 0; i < 4; i++ {
			if item := int(le.Uint16(e[16+i*2:])); item != 0 {
				name, _ := names.Item(item)
				tr.Items = append(tr.Items, name)
			}
		}
		entry := 8
		if flags&partyCustomMoves != 0 {
			entry = 16
		}
		b, err := read(party, size*entry)
		if err != nil {
			return nil, fmt.Errorf("trainer %d's party: %w", id, err)
		}
		for i := 0; i < size; i++ {
			p := b[i*entry:]
			// iv u16, level u16, species u16, then item u16 and/or 4 moves.
			m := TrainerMon{Level: int(le.Uint16(p[2:])), Species: int(le.Uint16(p[4:]))}
			m.SpeciesName, _ = names.Species(m.Species)
			if flags&partyHeldItem != 0 {
				if item := int(le.Uint16(p[6:])); item != 0 {
					m.Item, _ = names.Item(item)
				}
			}
			if flags&partyCustomMoves != 0 {
				for k := 0; k < 4; k++ {
					if mv := int(le.Uint16(p[8+k*2:])); mv != 0 {
						name, _ := names.Move(mv)
						m.Moves = append(m.Moves, name)
					}
				}
			}
			if m.Species != 0 {
				tr.Party = append(tr.Party, m)
			}
		}
		if len(tr.Party) > 0 {
			out = append(out, tr)
		}
	}
	return out, nil
}

func isROMPointer(p uint32) bool { return p >= 0x08000000 && p < 0x0A000000 }

// TrainerMaps finds which maps each trainer stands on (see ReadMaps).
// Returns trainer ID -> maps.
func TrainerMaps(read ReadMemory, groups uint32) (map[int][][2]uint8, error) {
	maps, err := ReadMaps(read, groups)
	if err != nil {
		return nil, err
	}
	out := map[int][][2]uint8{}
	for _, m := range maps {
		for _, id := range m.Trainers {
			out[id] = append(out[id], [2]uint8{uint8(m.Group), uint8(m.Num)})
		}
	}
	return out, nil
}

// scriptTrainers finds trainerbattle commands (0x5C, type, trainer u16,
// local ID u16, intro text pointer) in the first bytes of a script. Most
// trainers' scripts start with one; gym leaders' usually check flags first,
// and hacks may hold a second battle for a harder difficulty. CFRU hacks
// add battle types past vanilla's 0–9 (Unbound uses 13).
func scriptTrainers(read ReadMemory, script uint32) []int {
	if !isROMPointer(script) {
		return nil
	}
	b, err := read(script, 72)
	if err != nil {
		return nil
	}
	var ids []int
	for o := 0; o+10 <= len(b); o++ {
		if b[o] == 0x5C && b[o+1] <= 15 && isROMPointer(binary.LittleEndian.Uint32(b[o+6:])) {
			ids = append(ids, int(binary.LittleEndian.Uint16(b[o+2:])))
			o += 9
		}
	}
	return ids
}

// allROWE reads Pokémon R.O.W.E.'s trainers: 64-byte records (flags,
// class, music, pic, name[12] at 4, items from 0x16, AI at 0x24, party
// pointer at 0x28, party size at 0x3C; a second, harder party at 0x38 is
// left out). Party entries are 40 bytes: a level byte the game replaces
// with a scaled level at battle time, species at 0xA, held item at 0xE,
// and a pool of up to six moves from 0x12 that the game picks from.
func (t TrainerTables) allROWE(read ReadMemory, names *Names) ([]Trainer, error) {
	const size, entry = 64, 40
	le := binary.LittleEndian
	var out []Trainer
	for id := 1; id < 4096; id++ {
		e, err := read(t.Trainers+uint32(id*size), size)
		if err != nil {
			break
		}
		class, count, party := int(e[1]), int(e[0x3C]), le.Uint32(e[0x28:])
		if e[0] > 0x0F || count > 6 || (count > 0 && !isROMPointer(party)) {
			break
		}
		if count == 0 {
			continue
		}
		tr := Trainer{ID: id, Party: []TrainerMon{}}
		tr.Class, _ = ReadName(read, t.ClassNames+uint32(class*t.ClassNameLen), t.ClassNameLen)
		tr.Class = strings.NewReplacer("PKMN", "Pokémon", "Pkmn", "Pokémon").Replace(tr.Class)
		tr.Name = TitleCase(DecodeText(e[4:16]))
		for i := 0; i < 4; i++ {
			if item := int(le.Uint16(e[0x16+i*2:])); item != 0 {
				if name, _ := names.Item(item); name != "" && !strings.Contains(name, "?") {
					tr.Items = append(tr.Items, name)
				}
			}
		}
		b, err := read(party, count*entry)
		if err != nil {
			return nil, fmt.Errorf("trainer %d's party: %w", id, err)
		}
		for i := 0; i < count; i++ {
			p := b[i*entry:]
			m := TrainerMon{Species: int(le.Uint16(p[0xA:]))}
			if m.Species == 0 {
				continue
			}
			m.SpeciesName, _ = names.Species(m.Species)
			if item := int(le.Uint16(p[0xE:])); item != 0 {
				if name, _ := names.Item(item); !strings.Contains(name, "?") {
					m.Item = name
				}
			}
			for k := 0; k < 6; k++ {
				if mv := int(le.Uint16(p[0x12+k*2:])); mv != 0 {
					name, _ := names.Move(mv)
					if !slices.Contains(m.Moves, name) {
						m.Moves = append(m.Moves, name)
					}
				}
			}
			tr.Party = append(tr.Party, m)
		}
		if len(tr.Party) > 0 {
			out = append(out, tr)
		}
	}
	return out, nil
}
