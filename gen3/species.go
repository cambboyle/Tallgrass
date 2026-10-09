package gen3

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// SpeciesTables locates per-species data in a Gen 3 ROM.
type SpeciesTables struct {
	BaseStats      uint32
	TypeNames      uint32 // 7 bytes per type name
	AbilityNames   uint32
	AbilityNameLen int    // 13 in vanilla, 17 in CFRU hacks
	HiddenAbility  bool   // CFRU: a third ability at base stats byte 26
	Learnsets      uint32 // pointer per species
	CFRULearnsets  bool   // 3-byte (move, level) entries instead of packed u16s
	Evolutions     uint32
	EvosPerSpecies int // 5 in vanilla, 16 in CFRU hacks

	// Optional (0 = not known for this game).
	TMMoves           uint32 // u16 per TM, HMs after the TMs
	TMLearnsets       uint32
	TMLearnsetSize    int
	TutorMoves        uint32
	TutorLearnsets    uint32
	TutorLearnsetSize int
	EggMoves          uint32

	// EvoScheme names the hack family's extra evolution methods: "" for
	// vanilla, "cfru", or a hack whose later methods differ ("radical-red",
	// "unbound").
	EvoScheme string
	// AreaName names a region map section, for location-based evolutions.
	AreaName func(section int) string
}

// SpeciesInfo is what a species page shows.
type SpeciesInfo struct {
	Species     int         `json:"species"`
	Name        string      `json:"name"`
	Types       []string    `json:"types"`
	BaseStats   Stats       `json:"baseStats"`
	Abilities   []string    `json:"abilities"`
	Hidden      string      `json:"hiddenAbility,omitempty"`
	EvolvesTo   []Evolution `json:"evolvesTo"`
	EvolvesFrom []Evolution `json:"evolvesFrom"`
	FoundIn     []Found     `json:"foundIn"`
	LevelMoves  []LevelMove `json:"levelMoves"`
	TMMoves     []TMMove    `json:"tmMoves"`
	TutorMoves  []string    `json:"tutorMoves"`
	EggMoves    []string    `json:"eggMoves"`
}

// TMMove is a move learnt from a TM or HM.
type TMMove struct {
	Machine string `json:"machine"` // "TM01", "HM03"
	Move    string `json:"move"`
}

type Evolution struct {
	Species int    `json:"species"`
	Name    string `json:"name"`
	How     string `json:"how"`
}

type LevelMove struct {
	Level int    `json:"level"`
	Move  string `json:"move"`
}

// Species reads a species' data. names supplies species, move, item and
// type-independent names.
func (t SpeciesTables) Species(read ReadMemory, names *Names, species int) (*SpeciesInfo, error) {
	info := &SpeciesInfo{Species: species, EvolvesTo: []Evolution{}, LevelMoves: []LevelMove{},
		TMMoves: []TMMove{}, TutorMoves: []string{}, EggMoves: []string{}}
	info.Name, _ = names.Species(species)

	b, err := read(t.BaseStats+uint32(species*baseStatsSize), baseStatsSize)
	if err != nil {
		return nil, err
	}
	info.BaseStats = Stats{HP: int(b[0]), Attack: int(b[1]), Defense: int(b[2]), Speed: int(b[3]), SpAtk: int(b[4]), SpDef: int(b[5])}
	for i, ty := range []byte{b[6], b[7]} {
		if i == 1 && ty == b[6] {
			break // single-typed
		}
		name, err := t.fixedName(read, t.TypeNames, 7, int(ty))
		if err != nil {
			return nil, err
		}
		info.Types = append(info.Types, name)
	}
	ability := func(id byte) string {
		if id == 0 {
			return ""
		}
		n, _ := t.fixedName(read, t.AbilityNames, t.AbilityNameLen, int(id))
		return n
	}
	for _, id := range []byte{b[22], b[23]} {
		if a := ability(id); a != "" && (len(info.Abilities) == 0 || info.Abilities[0] != a) {
			info.Abilities = append(info.Abilities, a)
		}
	}
	if t.HiddenAbility {
		info.Hidden = ability(b[26])
		for _, a := range info.Abilities {
			if a == info.Hidden {
				info.Hidden = "" // some hacks repeat a regular ability as the hidden one
			}
		}
	}

	if err := t.readLearnset(read, names, species, info); err != nil {
		return nil, fmt.Errorf("learnset: %w", err)
	}
	if err := t.readEvolutions(read, names, species, info); err != nil {
		return nil, fmt.Errorf("evolutions: %w", err)
	}
	if err := t.readMachineMoves(read, names, species, info); err != nil {
		return nil, fmt.Errorf("TM moves: %w", err)
	}
	if err := t.readTutorMoves(read, names, species, info); err != nil {
		return nil, fmt.Errorf("tutor moves: %w", err)
	}
	if err := t.readEggMoves(read, names, species, info); err != nil {
		return nil, fmt.Errorf("egg moves: %w", err)
	}
	return info, nil
}

func (t SpeciesTables) fixedName(read ReadMemory, table uint32, size, id int) (string, error) {
	return ReadName(read, table+uint32(id*size), size)
}

func (t SpeciesTables) readLearnset(read ReadMemory, names *Names, species int, info *SpeciesInfo) error {
	le := binary.LittleEndian
	p, err := read(t.Learnsets+uint32(species*4), 4)
	if err != nil {
		return err
	}
	list := le.Uint32(p)
	entry := 2
	if t.CFRULearnsets {
		entry = 3
	}
	b, err := read(list, 120*entry) // no species learns 120 moves
	if err != nil {
		return err
	}
	for o := 0; o+entry <= len(b); o += entry {
		var move, level int
		if t.CFRULearnsets {
			move, level = int(le.Uint16(b[o:])), int(b[o+2])
			if move == 0 || move == 0xFFFF { // CFRU ends lists with (0, 0xFF)
				break
			}
		} else {
			v := le.Uint16(b[o:])
			if v == 0xFFFF {
				break
			}
			move, level = int(v&0x1FF), int(v>>9)
		}
		name, _ := names.Move(move)
		info.LevelMoves = append(info.LevelMoves, LevelMove{Level: level, Move: name})
	}
	return nil
}

func (t SpeciesTables) readEvolutions(read ReadMemory, names *Names, species int, info *SpeciesInfo) error {
	le := binary.LittleEndian
	b, err := read(t.Evolutions+uint32(species*t.EvosPerSpecies*8), t.EvosPerSpecies*8)
	if err != nil {
		return err
	}
	for i := 0; i < t.EvosPerSpecies; i++ {
		method, param, target, extra := int(le.Uint16(b[i*8:])), int(le.Uint16(b[i*8+2:])), int(le.Uint16(b[i*8+4:])), int(le.Uint16(b[i*8+6:]))
		if method == 0 || target == 0 {
			continue
		}
		if method == 254 && t.EvoScheme != "" {
			// A Mega form lists its way back to the base form, with no item.
			if n, _ := names.Item(param); n == "" || strings.Contains(n, "?") {
				continue
			}
		}
		e := Evolution{Species: target, How: t.evolutionMethod(read, names, method, param, extra)}
		e.Name, _ = names.Species(target)
		info.EvolvesTo = append(info.EvolvesTo, e)
	}
	return nil
}

// evolutionMethod describes how a species evolves. Methods 1–15 are
// vanilla Gen 3. CFRU hacks add 16–28 (the same in every hack we've seen),
// then number later ones their own way, chosen by EvoScheme. extra is the
// entry's last u16, which some CFRU methods use.
func (t SpeciesTables) evolutionMethod(read ReadMemory, names *Names, method, param, extra int) string {
	item := func(id int) string { n, _ := names.Item(id); return n }
	move := func(id int) string { n, _ := names.Move(id); return n }
	species := func(id int) string { n, _ := names.Species(id); return n }
	typeName := func(id int) string { n, _ := t.fixedName(read, t.TypeNames, 7, id); return n }
	area := func(sec int) string {
		if t.AreaName != nil {
			if n := t.AreaName(sec); n != "" {
				return n
			}
		}
		return fmt.Sprintf("a special place (area %d)", sec)
	}
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
		return "Trade holding " + item(param)
	case 7:
		return "Use " + item(param)
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
	}
	if t.EvoScheme == "" {
		return fmt.Sprintf("Special condition (method %d)", method)
	}
	switch method { // CFRU
	case 16:
		return fmt.Sprintf("Level %d while it's raining", param)
	case 17:
		return fmt.Sprintf("Level up knowing a %s-type move", typeName(param))
	case 18:
		return fmt.Sprintf("Level %d with a %s-type Pokémon in the party", param, typeName(extra))
	case 19:
		return "Level up at " + area(param)
	case 20:
		return fmt.Sprintf("Level %d (male)", param)
	case 21:
		return fmt.Sprintf("Level %d (female)", param)
	case 22:
		return fmt.Sprintf("Level %d at night", param)
	case 23:
		return fmt.Sprintf("Level %d during the day", param)
	case 24:
		return "Level up at night holding " + item(param)
	case 25:
		return "Level up during the day holding " + item(param)
	case 26:
		return "Level up knowing " + move(param)
	case 27:
		return "Level up with " + species(param) + " in the party"
	case 28:
		return fmt.Sprintf("Level %d between %02d:00 and %02d:00", param, extra>>8, extra&0xFF)
	case 253:
		return "Gigantamax"
	case 254:
		return "Mega Evolution holding " + item(param)
	}
	switch t.EvoScheme {
	case "radical-red":
		switch method {
		case 30:
			return fmt.Sprintf("Level %d with an amped nature", param)
		case 31:
			return fmt.Sprintf("Level %d with a low-key nature", param)
		}
	case "unbound":
		switch method {
		case 30:
			return "Land 3 critical hits in one battle"
		case 31:
			return fmt.Sprintf("Level %d with an amped nature", param)
		case 32:
			return fmt.Sprintf("Level %d with a low-key nature", param)
		case 34: // regional forms; extra is a condition we haven't decoded
			return fmt.Sprintf("Use %s (regional form; special condition)", item(param))
		case 35:
			return fmt.Sprintf("Level %d (regional form; special condition)", param)
		case 36:
			return fmt.Sprintf("Use %s (regional form; special condition)", item(param))
		case 37, 38:
			return "Take enough recoil damage, then level up"
		case 39:
			return "Use " + item(param) + " (on a full moon)"
		}
	}
	return fmt.Sprintf("Special condition (method %d)", method)
}
