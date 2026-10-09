package gen4

import (
	"fmt"
	"strings"

	"tallgrass/gen3"
)

// SpeciesInfo reads a species page's data: base stats, types and abilities
// from the personal archive; learnsets and evolutions from their archives.
// Learnsets are packed u16s (level<<9 | move) in vanilla games, or (move,
// level) u16 pairs in hacks with more than 511 moves; both are detected.
func (d *Data) SpeciesInfo(species int) (*gen3.SpeciesInfo, error) {
	if species <= 0 || species >= len(d.personal) {
		return nil, fmt.Errorf("no species %d", species)
	}
	p := d.personal[species]
	info := &gen3.SpeciesInfo{Species: species, Name: Name(d.Species, species),
		EvolvesTo: []gen3.Evolution{}, LevelMoves: []gen3.LevelMove{},
		TMMoves: []gen3.TMMove{}, TutorMoves: []string{}, EggMoves: []string{}}
	if len(p) >= 0x18 {
		info.BaseStats = gen3.Stats{HP: int(p[0]), Attack: int(p[1]), Defense: int(p[2]), Speed: int(p[3]), SpAtk: int(p[4]), SpDef: int(p[5])}
		info.Types = []string{Name(d.Types, int(p[6]))}
		if p[7] != p[6] {
			info.Types = append(info.Types, Name(d.Types, int(p[7])))
		}
		for _, a := range []byte{p[0x16], p[0x17]} {
			if name := Name(d.Abilities, int(a)); a != 0 && name != "" && (len(info.Abilities) == 0 || info.Abilities[0] != name) {
				info.Abilities = append(info.Abilities, name)
			}
		}
	}

	rom, err := OpenROM(d.romPath)
	if err != nil {
		return nil, err
	}
	defer rom.Close()
	if d.Layout.Learnsets != "" {
		b, err := rom.ArchiveFile(d.Layout.Learnsets, species)
		if err != nil {
			return nil, fmt.Errorf("learnset: %w", err)
		}
		info.LevelMoves = d.learnset(b)
	}
	if d.Layout.Evolutions != "" {
		b, err := rom.ArchiveFile(d.Layout.Evolutions, species)
		if err != nil {
			return nil, fmt.Errorf("evolutions: %w", err)
		}
		for o := 0; o+6 <= len(b); o += 6 {
			method, param, target := int(le.Uint16(b[o:])), int(le.Uint16(b[o+2:])), int(le.Uint16(b[o+4:]))
			if method == 0 || target == 0 {
				continue
			}
			info.EvolvesTo = append(info.EvolvesTo, gen3.Evolution{Species: target, Name: d.speciesName(target),
				How: d.evolutionMethod(method, param)})
		}
	}
	if x, err := d.extras(); err == nil {
		info.TMMoves = d.machineMoves(species, x.tms)
		info.TutorMoves = d.tutorMoves(species, x)
		for _, m := range x.eggs[species] {
			info.EggMoves = append(info.EggMoves, Name(d.Moves, m))
		}
	}
	return info, nil
}

func (d *Data) learnset(b []byte) []gen3.LevelMove {
	moves := []gen3.LevelMove{}
	if len(b) < 2 {
		return moves
	}
	// Every Pokémon learns something at level 1 or more; in the packed
	// format a level-0 first entry can't happen, so it means pairs.
	pairs := le.Uint16(b)>>9 == 0 && len(b) >= 4
	if pairs {
		for o := 0; o+4 <= len(b); o += 4 {
			move, level := le.Uint16(b[o:]), le.Uint16(b[o+2:])
			if move == 0xFFFF || move == 0 {
				break
			}
			moves = append(moves, gen3.LevelMove{Level: int(level), Move: Name(d.Moves, int(move))})
		}
		return moves
	}
	for o := 0; o+2 <= len(b); o += 2 {
		v := le.Uint16(b[o:])
		if v == 0xFFFF {
			break
		}
		moves = append(moves, gen3.LevelMove{Level: int(v >> 9), Move: Name(d.Moves, int(v&0x1FF))})
	}
	return moves
}

// evolutionMethod describes Gen 4's 26 methods; hacks may add more, which
// we don't guess at.
func (d *Data) evolutionMethod(method, param int) string {
	item := func() string { return Name(d.Items, param) }
	switch method {
	case 1:
		return "High friendship"
	case 2:
		return "High friendship, daytime"
	case 3:
		return "High friendship, night"
	case 4:
		return fmt.Sprintf("Level %d", param)
	case 5:
		return "Trade"
	case 6:
		return "Trade holding " + item()
	case 7:
		return "Use " + item()
	case 8:
		return fmt.Sprintf("Level %d, Attack > Defense", param)
	case 9:
		return fmt.Sprintf("Level %d, Attack = Defense", param)
	case 10:
		return fmt.Sprintf("Level %d, Attack < Defense", param)
	case 11, 12:
		return fmt.Sprintf("Level %d (depends on personality)", param)
	case 13:
		return fmt.Sprintf("Level %d", param)
	case 14:
		return fmt.Sprintf("Level %d, with a free party slot", param)
	case 15:
		return "High beauty"
	case 16:
		return "Use " + item() + " (male)"
	case 17:
		return "Use " + item() + " (female)"
	case 18:
		return "Level up holding " + item() + ", daytime"
	case 19:
		return "Level up holding " + item() + ", night"
	case 20:
		return "Level up knowing " + Name(d.Moves, param)
	case 21:
		return "Level up with " + Name(d.Species, param) + " in the party"
	case 22:
		return fmt.Sprintf("Level %d (male)", param)
	case 23:
		return fmt.Sprintf("Level %d (female)", param)
	case 24:
		return "Level up near a magnetic field"
	case 25:
		return "Level up near a Moss Rock"
	case 26:
		return "Level up near an Ice Rock"
	// Methods past vanilla's 26, as HeartGold Generations numbers them.
	case 27:
		return fmt.Sprintf("Level %d during the day", param)
	case 28:
		return fmt.Sprintf("Level %d at night", param)
	case 29:
		return fmt.Sprintf("Level %d at dusk", param)
	case 32:
		return fmt.Sprintf("Level %d with a Dark-type Pokémon in the party", param)
	case 33:
		return "Trade for " + Name(d.Species, param)
	case 34:
		return fmt.Sprintf("Level %d with an amped nature", param)
	case 35:
		return fmt.Sprintf("Level %d with a low-key nature", param)
	case 36:
		return fmt.Sprintf("Land %d critical hits in one battle", param)
	}
	return fmt.Sprintf("Special condition (method %d)", method)
}

// findTypes picks the type-name list (entry 0 "Normal", entry 1 "Fighting").
func findTypes(lists [][]string) []string {
	for _, l := range lists {
		if len(l) > 2 && strings.EqualFold(l[0], "Normal") && strings.EqualFold(l[1], "Fighting") {
			out := make([]string, len(l))
			for i, n := range l {
				out[i] = n
				if n == strings.ToUpper(n) {
					out[i] = titleCase(n)
				}
			}
			return out
		}
	}
	return nil
}

// EvolvesFrom maps each species to what evolves into it, by reading every
// species' evolution file once.
func (d *Data) EvolvesFrom() (map[int][]gen3.Evolution, error) {
	out := map[int][]gen3.Evolution{}
	if d.Layout.Evolutions == "" {
		return out, nil
	}
	rom, err := OpenROM(d.romPath)
	if err != nil {
		return nil, err
	}
	defer rom.Close()
	for from := 1; from < len(d.personal); from++ {
		b, err := rom.ArchiveFile(d.Layout.Evolutions, from)
		if err != nil {
			break // fewer evolution files than species
		}
		for o := 0; o+6 <= len(b); o += 6 {
			method, param, target := int(le.Uint16(b[o:])), int(le.Uint16(b[o+2:])), int(le.Uint16(b[o+4:]))
			if method == 0 || target <= 0 || target >= len(d.personal) {
				continue
			}
			out[target] = append(out[target], gen3.Evolution{Species: from, Name: d.speciesName(from), How: d.evolutionMethod(method, param)})
		}
	}
	return out, nil
}

// speciesName names a species, falling back to its number for forms that
// hacks add past the end of the name list (e.g. regional forms).
func (d *Data) speciesName(id int) string {
	if n := Name(d.Species, id); strings.TrimSpace(n) != "" {
		return n
	}
	return fmt.Sprintf("Species #%d (a form)", id)
}

// SpeciesCount is one past the last species ID with base stats.
func (d *Data) SpeciesCount() int { return len(d.personal) }

// GameCode is the ROM's 4-letter code, e.g. "IPKE".
func (d *Data) GameCode() string {
	code, _ := ReadGameCode(d.romPath)
	return code
}
