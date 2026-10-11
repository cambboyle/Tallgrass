package gen3

import (
	"encoding/hex"
	"testing"
)

// A real ROWE 2.1.1 party Pokémon: Cam's Alolan Meowth, Lv 10, read live.
const roweMeowth = "2347d0d350c5e78cc7d9e3ebe8dcffd4b7a1dd18fca00f002d2880913410582800000000000000000a0200bdd5e1d9e6e3e210000a282319000000000aff1c001c000c000c0017000f000d00"

func TestDecodeROWE(t *testing.T) {
	b, _ := hex.DecodeString(roweMeowth)
	p, err := DecodeROWE(b)
	if err != nil {
		t.Fatal(err)
	}
	var moves, pp []int
	for _, m := range p.Moves {
		moves, pp = append(moves, m.ID), append(pp, m.PP)
	}
	if p.Nickname != "Meowth" || p.OTName != "Cameron" || p.Species != 52 || p.Form != 1 || p.Level != 10 ||
		p.Experience != 1000 || p.CurrentHP != 28 || p.Stats.HP != 28 || p.Stats.Speed != 23 || p.Stats.SpAtk != 15 {
		t.Errorf("decoded %+v", p)
	}
	// Fake Out, Growl, Scratch, Bite with their full PP.
	if len(moves) != 4 || moves[0] != 252 || moves[1] != 45 || moves[2] != 10 || moves[3] != 44 ||
		pp[0] != 10 || pp[1] != 40 || pp[2] != 35 || pp[3] != 25 {
		t.Errorf("moves %v, PP %v", moves, pp)
	}

	// Form 1 of Meowth is its own species (Alolan Meowth, 1560 in ROWE).
	rom := map[uint32][]byte{0x085DC330 + 52*4: {0x00, 0x10, 0x00, 0x09}, 0x09001000: {52, 0, 0x18, 0x06, 0x21, 0x06, 0xFF, 0xFF}}
	read := func(addr uint32, n int) ([]byte, error) {
		for base, data := range rom {
			if addr >= base && int(addr-base)+n <= len(data) {
				return data[addr-base : int(addr-base)+n], nil
			}
		}
		return make([]byte, n), nil
	}
	if err := ResolveForm(read, 0x085DC330, p); err != nil || p.Species != 1560 {
		t.Errorf("ResolveForm: species %d, %v", p.Species, err)
	}
}

// IVs from Trainer Skill levels (skills 16–21 = HP, Atk, Def, SpA, SpD,
// Spe): 3 × level + 1, so 1 untouched and 31 at the maximum of 10.
func TestROWESkillIVs(t *testing.T) {
	skills := make([]byte, 32)
	skills[16], skills[17], skills[21] = 10, 3, 0
	got := ROWESkillIVs(skills)
	want := Stats{HP: 31, Attack: 10, Defense: 1, SpAtk: 1, SpDef: 1, Speed: 1}
	if got != want {
		t.Errorf("ROWESkillIVs = %+v, want %+v", got, want)
	}
}
