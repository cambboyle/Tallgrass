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
	// OtherParty is a second team some hacks give a trainer (R.O.W.E.,
	// likely its harder mode); when the game uses it isn't known.
	OtherParty []TrainerMon `json:"otherParty,omitempty"`
	Areas      []string     `json:"areas,omitempty"` // where they stand, when found
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
//
// Scripts that branch first (decomp hacks' gym leaders: R.O.W.E.'s
// Roxanne checks flags, then jumps to her battle) are followed through
// call, goto, call_if and goto_if, one level deep and only when the script
// has no battle of its own (deeper, shared scripts put trainers on maps
// they aren't on).
func scriptTrainers(read ReadMemory, script uint32) []int {
	var ids []int
	seen := map[uint32]bool{}
	var scan func(at uint32, depth int)
	scan = func(at uint32, depth int) {
		if !isROMPointer(at) || seen[at] {
			return
		}
		seen[at] = true
		b, err := read(at, 72)
		if err != nil {
			return
		}
		le := binary.LittleEndian
		var targets []uint32
		for o := 0; o+10 <= len(b); o++ {
			if b[o] == 0x5C && b[o+1] <= 15 && isROMPointer(le.Uint32(b[o+6:])) {
				if id := int(le.Uint16(b[o+2:])); !slices.Contains(ids, id) {
					ids = append(ids, id)
				}
				o += 9
				continue
			}
			switch {
			case (b[o] == 0x04 || b[o] == 0x05) && o+5 <= len(b) && isROMPointer(le.Uint32(b[o+1:])):
				targets = append(targets, le.Uint32(b[o+1:])) // call, goto
			case (b[o] == 0x06 || b[o] == 0x07) && o+6 <= len(b) && b[o+1] <= 5 && isROMPointer(le.Uint32(b[o+2:])):
				targets = append(targets, le.Uint32(b[o+2:])) // goto_if, call_if
			}
		}
		if depth > 0 && len(ids) == 0 {
			for _, t := range targets {
				scan(t, depth-1)
			}
		}
	}
	scan(script, 1)
	return ids
}

// allROWE reads Pokémon R.O.W.E.'s trainers: 64-byte records (flags,
// class, music, pic, name[12] at 4, items from 0x16, AI at 0x24, party
// pointer at 0x28, party size at 0x3C; a second team at 0x38, size 0x3D,
// goes in OtherParty). Party entries are 40 bytes: a level byte the game replaces
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
		readParty := func(at uint32, n int) ([]TrainerMon, error) {
			mons := []TrainerMon{}
			b, err := read(at, n*entry)
			if err != nil {
				return nil, fmt.Errorf("trainer %d's party: %w", id, err)
			}
			for i := 0; i < n; i++ {
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
				mons = append(mons, m)
			}
			return mons, nil
		}
		if tr.Party, err = readParty(party, count); err != nil {
			return nil, err
		}
		if other, n := le.Uint32(e[0x38:]), int(e[0x3D]); isROMPointer(other) && n > 0 && n <= 6 {
			if tr.OtherParty, err = readParty(other, n); err != nil {
				return nil, err
			}
		}
		if len(tr.Party) > 0 {
			out = append(out, tr)
		}
	}
	return out, nil
}
