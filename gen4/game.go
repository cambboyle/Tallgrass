package gen4

import (
	"fmt"
	"strings"
)

// Layout is where a game family keeps things, in its save and its ROM.
type Layout struct {
	Family      string
	TrainerName int // in the general block; trainer ID and secret ID follow it
	Party       int // first party Pokémon; the count is 4 bytes before
	BoxStart    int // in the storage block
	BoxStride   int // bytes per box (30 Pokémon plus any padding)
	NamesGap    int // bytes between the last box and the box names (0x28 each)
	Personal    string
	Growth      string
	Messages    string
	Sprites     string // Pokémon sprite archive
	Encounters  string // wild encounter files ("" if not supported yet)
	Learnsets   string
	Evolutions  string
	MapHeaders  uint32 // map header table in DS RAM (0 if not supported yet)
	EggMoves    string // egg move archive; "" = search the overlays (Platinum keeps them in overlay 5)
}

var layouts = map[string]Layout{
	// Game codes' first three letters; the fourth is the region.
	"CPU": {Family: "Platinum", TrainerName: 0x68, Party: 0xA0, BoxStart: 4, BoxStride: 0xFF0,
		Personal: "/poketool/personal/pl_personal.narc", Growth: "/poketool/personal/pl_growtbl.narc",
		Messages: "/msgdata/pl_msg.narc", Sprites: "/poketool/pokegra/pl_pokegra.narc",
		Learnsets: "/poketool/personal/wotbl.narc", Evolutions: "/poketool/personal/evo.narc",
		Encounters: "/fielddata/encountdata/pl_enc_data.narc", MapHeaders: PlatinumMapHeaders},
	"IPK": hgss, // HeartGold
	"IPG": hgss, // SoulSilver
}

var hgss = Layout{Family: "HeartGold/SoulSilver", TrainerName: 0x64, Party: 0x98, BoxStride: 0x1000, NamesGap: 8,
	Personal: "/a/0/0/2", Growth: "/a/0/0/3", Messages: "/a/0/2/7", Sprites: "/a/0/0/4",
	Encounters: "/a/0/3/7", MapHeaders: HGSSMapHeaders, Learnsets: "/a/0/3/3", Evolutions: "/a/0/3/4",
	EggMoves: "/a/2/2/9"}

// LayoutFor returns the layout for a DS game code.
func LayoutFor(code string) (Layout, bool) {
	if len(code) < 3 {
		return Layout{}, false
	}
	l, ok := layouts[code[:3]]
	return l, ok
}

// boxCount fits as many boxes (and their names) as the storage block holds:
// vanilla games have 18, some hacks more (HeartGold Generations has 30).
func (l Layout) boxCount(storage int) int {
	return max(0, (storage-l.BoxStart-l.NamesGap)/(l.BoxStride+0x28))
}

// Data is what the box viewer needs from a Gen 4 ROM: names and levels.
type Data struct {
	Layout                                             Layout
	Species, Moves, Items, Abilities, Locations, Types []string
	TrainerNames, TrainerClasses                       []string
	personal                                           [][]byte   // base stats etc., per species
	growthRate                                         []byte     // per species
	exp                                                [][]uint32 // per growth rate, by level
	romPath                                            string     // for reading sprites on demand
	extra                                              *romExtras // loaded on first use
}

// LoadData reads names and level tables from a ROM. Name lists are found by
// content (entry 1 of the species list is Bulbasaur, and so on), which also
// works for hacks that add lists or move them.
func LoadData(path string) (*Data, error) {
	rom, err := OpenROM(path)
	if err != nil {
		return nil, err
	}
	defer rom.Close()
	layout, ok := LayoutFor(rom.Code)
	if !ok {
		return nil, fmt.Errorf("game code %s isn't a supported Gen 4 game", rom.Code)
	}
	d := &Data{Layout: layout, romPath: path, extra: &romExtras{}}

	files, err := rom.Archive(layout.Messages)
	if err != nil {
		return nil, err
	}
	var lists [][]string
	for _, f := range files {
		if m, err := decodeMessages(f); err == nil {
			lists = append(lists, m)
		} else {
			lists = append(lists, nil) // keep archive order: trainer names precede classes
		}
	}
	// Trainer classes start "Pokémon Trainer", "Pokémon Trainer",
	// "Youngster"; trainer names are the file before them.
	for i, l := range lists {
		if len(l) > 2 && strings.EqualFold(l[2], "Youngster") && i > 0 {
			d.TrainerClasses, d.TrainerNames = l, lists[i-1]
			for k, c := range d.TrainerClasses {
				d.TrainerClasses[k] = strings.ReplaceAll(titleCaseIfCaps(c), "PKMN", "Pokémon")
			}
			break
		}
	}
	entry1 := func(want string) []string {
		var caps []string
		for _, l := range lists {
			if len(l) > 1 && strings.EqualFold(l[1], want) {
				if l[1] != strings.ToUpper(l[1]) {
					return l // prefer mixed case
				}
				if caps == nil {
					caps = l
				}
			}
		}
		for i := range caps {
			caps[i] = titleCase(caps[i])
		}
		return caps
	}
	d.Species, d.Moves, d.Items, d.Abilities = entry1("Bulbasaur"), entry1("Pound"), entry1("Master Ball"), entry1("Stench")
	for _, l := range lists {
		for _, name := range l[:min(len(l), 40)] {
			if name == "Twinleaf Town" || name == "New Bark Town" {
				d.Locations = l
				break
			}
		}
		if d.Locations != nil {
			break
		}
	}
	if d.Species == nil || d.Moves == nil {
		return nil, fmt.Errorf("couldn't find name lists in %s", layout.Messages)
	}

	personal, err := rom.Archive(layout.Personal)
	if err != nil {
		return nil, err
	}
	d.personal = personal
	d.Types = findTypes(lists)
	d.growthRate = make([]byte, len(personal))
	for i, p := range personal {
		if len(p) > 0x13 {
			d.growthRate[i] = p[0x13]
		}
	}
	growth, err := rom.Archive(layout.Growth)
	if err != nil {
		return nil, err
	}
	for _, g := range growth {
		row := make([]uint32, len(g)/4)
		for i := range row {
			row[i] = le.Uint32(g[i*4:])
		}
		d.exp = append(d.exp, row)
	}
	return d, nil
}

// Level works out a boxed Pokémon's level from its experience.
func (d *Data) Level(species int, exp uint32) int {
	if species >= len(d.growthRate) || int(d.growthRate[species]) >= len(d.exp) {
		return 0
	}
	row := d.exp[d.growthRate[species]]
	level := 1
	for lv := 2; lv < len(row) && lv <= 100 && row[lv] <= exp; lv++ {
		level = lv
	}
	return level
}

// Name looks up an ID in a name list; "" if out of range.
func Name(list []string, id int) string {
	if id < 0 || id >= len(list) {
		return ""
	}
	return list[id]
}
