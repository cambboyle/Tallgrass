package gen4

import (
	"errors"
	"sort"

	"tallgrass/gen3"
)

// Wild encounters, HeartGold/SoulSilver family. Each map header (24 bytes,
// in the game's code, which sits decompressed in RAM) names the map's wild
// encounter file (byte 0, 0xFF for none) and location name (byte 0x12). The
// encounter files (196 bytes) hold rates, then grass slots by time of day,
// surfing, Rock Smash and fishing.
//
// Platinum's headers name the encounter file with a u16 at 0x0E (0xFFFF
// for none) and the location at 0x12 too; its encounter files are 424
// bytes (see parsePlatinumEncounters).
const (
	HGSSMapHeaders     = 0x020F6BE0 // in DS RAM
	PlatinumMapHeaders = 0x020E601C
	mapHeaderSize      = 24
	encFileSize        = 196
	ptEncFileSize      = 0x1A8
)

var (
	grassRates = []int{20, 20, 10, 10, 10, 10, 5, 5, 4, 4, 1, 1}
	waterRates = []int{60, 30, 5, 4, 1}
	rodRates   = []int{40, 40, 15, 4, 1}
	smashRates = []int{90, 10}
	// HeartGold/SoulSilver's times of day.
	hgssPeriods = []gen3.Period{
		{Name: "Morning", FromHour: 4, ToHour: 10},
		{Name: "Day", FromHour: 10, ToHour: 20},
		{Name: "Night", FromHour: 20, ToHour: 4},
	}
)

// MapHeader is what the Route section needs from a map's header.
type MapHeader struct {
	WildBank int // -1 if the map has no wild Pokémon
	Section  int // index into Data.Locations
}

// ParseMapHeader reads a HeartGold/SoulSilver map header.
func ParseMapHeader(b []byte) MapHeader {
	h := MapHeader{WildBank: int(b[0]), Section: int(b[0x12])}
	if b[0] == 0xFF {
		h.WildBank = -1
	}
	return h
}

// MapHeader reads a map header in this game's format.
func (l Layout) MapHeader(b []byte) MapHeader {
	if l.Family != "Platinum" {
		return ParseMapHeader(b)
	}
	h := MapHeader{WildBank: int(le.Uint16(b[0x0E:])), Section: int(b[0x12])}
	if h.WildBank == 0xFFFF {
		h.WildBank = -1
	}
	return h
}

// realHeader tells map headers from what follows the table: every real
// header has day and night music IDs (1000 and up), at 0x0C in
// HeartGold/SoulSilver and 0x0A in Platinum.
func (l Layout) realHeader(b []byte) bool {
	at := 0x0C
	if l.Family == "Platinum" {
		at = 0x0A
	}
	day, night := le.Uint16(b[at:]), le.Uint16(b[at+2:])
	return day >= 1000 && day <= 2000 && night >= 1000 && night <= 2000
}

// parseEncounters reads an encounter file in this game's format.
func (l Layout) parseEncounters(b []byte) ([]gen3.PeriodEncounters, error) {
	if l.Family == "Platinum" {
		if len(b) < ptEncFileSize {
			return nil, errors.New("encounter file too short")
		}
		return parsePlatinumEncounters(b), nil
	}
	if len(b) < encFileSize {
		return nil, errors.New("encounter file too short")
	}
	return parseHGSSEncounters(b), nil
}

// parsePlatinumEncounters reads Platinum's format: grass rate (u32), 12
// grass slots (level u32, species u32), then species that replace slots
// during swarms, the day and the night (2 each, u32), Poké Radar (4) and
// Game Boy Advance slot (2 per game) species, then surfing, an unused
// method, and the three rods: a rate (u32) and 5 slots (max level u8, min
// level u8, pad, species u32) each. The day and night species replace grass
// slots 2 and 3; morning uses the base slots. Swarms, the radar and GBA
// slots are left out (they need something special).
func parsePlatinumEncounters(b []byte) []gen3.PeriodEncounters {
	water := []gen3.MethodEncounters{}
	fish := func(name string, off int) {
		if le.Uint32(b[off:]) == 0 {
			return
		}
		slots := make([][3]int, 5)
		for i := range slots {
			e := b[off+4+i*8:]
			slots[i] = [3]int{int(e[1]), int(e[0]), int(le.Uint32(e[4:]))}
		}
		rates := waterRates
		if name != "Surfing" {
			rates = rodRates
		}
		if m := merge(name, slots, rates); len(m.Encounters) > 0 {
			water = append(water, m)
		}
	}
	fish("Surfing", 0xCC)
	fish("Old Rod", 0x124)
	fish("Good Rod", 0x150)
	fish("Super Rod", 0x17C)

	var periods []gen3.PeriodEncounters
	for i, p := range hgssPeriods {
		var methods []gen3.MethodEncounters
		if le.Uint32(b) != 0 {
			grass := make([][3]int, 12)
			for s := range grass {
				lv := int(le.Uint32(b[4+s*8:]))
				grass[s] = [3]int{lv, lv, int(le.Uint32(b[8+s*8:]))}
			}
			if i > 0 { // day (0x6C) or night (0x74) replace slots 2 and 3
				for k := 0; k < 2; k++ {
					if sp := int(le.Uint32(b[0x6C+(i-1)*8+k*4:])); sp != 0 {
						grass[2+k][2] = sp
					}
				}
			}
			if m := merge("Grass", grass, grassRates); len(m.Encounters) > 0 {
				methods = append(methods, m)
			}
		}
		methods = append(methods, water...)
		if len(methods) > 0 {
			periods = append(periods, gen3.PeriodEncounters{Period: p, Methods: methods})
		}
	}
	return periods
}

// FindLocation finds the player's current position in a live general
// block. The game keeps five location records (map, warp, x, z, facing as
// 32-bit values) back to back: the current position first, then warp and
// respawn points. Only the first updates on every step; the others change
// at doors. The block is found as five plausible records, at least one of
// them a warp of -1 (a free position rather than a door). Its offset differs
// between games and hacks. Returns the first record's offset, or -1.
func FindLocation(general []byte) int {
	const records, size = 5, 20
	plausible := func(o int) (ok, free bool) {
		mp, warp := int32(le.Uint32(general[o:])), int32(le.Uint32(general[o+4:]))
		x, z, dir := int32(le.Uint32(general[o+8:])), int32(le.Uint32(general[o+12:])), int32(le.Uint32(general[o+16:]))
		ok = mp >= 0 && mp < 2000 && warp >= -1 && warp < 256 && x >= 0 && x < 4096 && z >= 0 && z < 4096 && dir >= 0 && dir <= 3
		return ok, warp == -1 && mp > 0
	}
	for o := 0; o+records*size <= len(general); o += 4 {
		if mp, x := le.Uint32(general[o:]), le.Uint32(general[o+8:]); mp == 0 || mp >= 2000 || x == 0 {
			continue
		}
		free := false
		for r := 0; r < records; r++ {
			ok, f := plausible(o + r*size)
			if !ok {
				free = false
				break
			}
			free = free || f
		}
		if free {
			return o
		}
	}
	return -1
}

// Encounters reads a wild encounter file (HeartGold/SoulSilver format) as
// one set of methods per time of day.
func (d *Data) Encounters(bank int) ([]gen3.PeriodEncounters, error) {
	if d.Layout.Encounters == "" {
		return nil, errors.New("no encounter data for this game")
	}
	rom, err := OpenROM(d.romPath)
	if err != nil {
		return nil, err
	}
	defer rom.Close()
	b, err := rom.ArchiveFile(d.Layout.Encounters, bank)
	if err != nil {
		return nil, err
	}
	return d.Layout.parseEncounters(b)
}

func parseHGSSEncounters(b []byte) []gen3.PeriodEncounters {
	rates := b[:6]                       // grass, surf, Rock Smash, old, good, super rod
	slots := func(off, n int) [][3]int { // min level, max level, species
		out := make([][3]int, n)
		for i := range out {
			e := b[off+i*4:]
			out[i] = [3]int{int(e[0]), int(e[1]), int(le.Uint16(e[2:]))}
		}
		return out
	}
	water := []gen3.MethodEncounters{}
	add := func(list *[]gen3.MethodEncounters, name string, rate byte, s [][3]int, chances []int) {
		if rate == 0 {
			return
		}
		if m := merge(name, s, chances); len(m.Encounters) > 0 {
			*list = append(*list, m)
		}
	}
	add(&water, "Surfing", rates[1], slots(0x64, 5), waterRates)
	add(&water, "Rock Smash", rates[2], slots(0x78, 2), smashRates)
	add(&water, "Old Rod", rates[3], slots(0x80, 5), rodRates)
	add(&water, "Good Rod", rates[4], slots(0x94, 5), rodRates)
	add(&water, "Super Rod", rates[5], slots(0xA8, 5), rodRates)

	var periods []gen3.PeriodEncounters
	for i, p := range hgssPeriods {
		var methods []gen3.MethodEncounters
		if rates[0] != 0 {
			grass := make([][3]int, 12)
			for s := range grass {
				lv := int(b[0x08+s])
				grass[s] = [3]int{lv, lv, int(le.Uint16(b[0x14+i*24+s*2:]))}
			}
			if m := merge("Grass", grass, grassRates); len(m.Encounters) > 0 {
				methods = append(methods, m)
			}
		}
		methods = append(methods, water...)
		if len(methods) > 0 {
			periods = append(periods, gen3.PeriodEncounters{Period: p, Methods: methods})
		}
	}
	return periods
}

// merge combines slots of the same species, most likely first.
func merge(method string, slots [][3]int, chances []int) gen3.MethodEncounters {
	by := map[int]*gen3.Encounter{}
	var order []*gen3.Encounter
	for i, s := range slots {
		if s[2] == 0 {
			continue
		}
		e, ok := by[s[2]]
		if !ok {
			e = &gen3.Encounter{Species: s[2], MinLevel: s[0], MaxLevel: s[1]}
			by[s[2]] = e
			order = append(order, e)
		}
		e.Chance += chances[i]
		e.MinLevel, e.MaxLevel = min(e.MinLevel, s[0]), max(e.MaxLevel, s[1])
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].Chance > order[j].Chance })
	m := gen3.MethodEncounters{Method: method}
	for _, e := range order {
		m.Encounters = append(m.Encounters, *e)
	}
	return m
}
