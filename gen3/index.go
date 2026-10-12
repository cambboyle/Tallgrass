package gen3

import (
	"encoding/binary"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Indexes across a whole ROM, built once and cached by the caller.

// EvolvesFrom maps each species to what evolves into it. Species past the
// end of the evolution table read as junk, so an entry only counts if both
// species have real names and the method is a plausible number.
func (t SpeciesTables) EvolvesFrom(read ReadMemory, names *Names, maxSpecies int) (map[int][]Evolution, error) {
	le := binary.LittleEndian
	stride := t.EvosPerSpecies * 8
	b, err := read(t.Evolutions, maxSpecies*stride)
	if err != nil {
		return nil, err
	}
	valid := func(species int) (string, bool) {
		n, err := names.Species(species)
		return n, err == nil && n != "" && !strings.Contains(n, "?")
	}
	out := map[int][]Evolution{}
	for from := 1; from < maxSpecies; from++ {
		fromName, ok := valid(from)
		if !ok {
			continue
		}
		for i := 0; i < t.EvosPerSpecies; i++ {
			e := b[from*stride+i*8:]
			method, param, target, extra := int(le.Uint16(e)), int(le.Uint16(e[2:])), int(le.Uint16(e[4:])), int(le.Uint16(e[6:]))
			if method == 0 || method > 64 || target <= 0 || target >= maxSpecies {
				continue
			}
			if _, ok := valid(target); !ok {
				continue
			}
			out[target] = append(out[target], Evolution{Species: from, Name: fromName, How: t.evolutionMethod(read, names, method, param, extra)})
		}
	}
	return out, nil
}

// Found is one place a species appears in the wild.
type Found struct {
	Area     string `json:"area"`
	Method   string `json:"method"`
	When     string `json:"when,omitempty"` // time-of-day periods, if not all day
	MinLevel int    `json:"minLevel"`
	MaxLevel int    `json:"maxLevel"`
	Chance   int    `json:"chance"` // best chance across those periods
}

// MapSections reads a map's region map section from the game's map header
// table (gMapGroups: per group, a list of map header pointers; the section
// is byte 0x14 of a header).
func MapSections(read ReadMemory, groups, xor uint32) func(group, num uint8) (int, error) {
	le := binary.LittleEndian
	return func(group, num uint8) (int, error) {
		g, err := read(groups+uint32(group)*4, 4)
		if err != nil {
			return 0, err
		}
		h, err := read(le.Uint32(g)^xor+uint32(num)*4, 4)
		if err != nil {
			return 0, err
		}
		sec, err := read(le.Uint32(h)+0x14, 1)
		if err != nil {
			return 0, err
		}
		return int(sec[0]), nil
	}
}

// FoundIn lists, for every species, where it appears in the wild: every
// map in the main encounter table and the time-of-day tables.
func (w *Wild) FoundIn(areaName func(group, num uint8) string) (map[int][]Found, error) {
	tables := []uint32{w.main}
	for _, p := range w.periods {
		if p.Headers != 0 {
			tables = append(tables, p.Headers)
		}
	}
	seen := map[[2]uint8]bool{}
	fb := NewFoundBuilder(len(w.periods))
	for _, t := range tables {
		headers, err := readWildHeaders(w.read, t, w.layout)
		if err != nil {
			return nil, err
		}
		for _, h := range headers {
			k := [2]uint8{h.Group, h.Num}
			if seen[k] {
				continue
			}
			seen[k] = true
			periods, err := w.ForMap(h.Group, h.Num)
			if err != nil {
				return nil, fmt.Errorf("map %d.%d: %w", h.Group, h.Num, err)
			}
			fb.Add(areaName(h.Group, h.Num), periods)
		}
	}
	return fb.Result(), nil
}

// FoundBuilder collects where each species appears, merging an area's
// methods across times of day ("Grass, Morning and Day, Lv 3–5").
type FoundBuilder struct {
	periods int
	acc     map[foundKey]*foundAgg
	order   []foundKey
}

type foundKey struct {
	species      int
	area, method string
}

type foundAgg struct {
	Found
	periods []string
}

// NewFoundBuilder takes the number of time-of-day periods the game has.
func NewFoundBuilder(periods int) *FoundBuilder {
	return &FoundBuilder{periods: periods, acc: map[foundKey]*foundAgg{}}
}

// Add records an area's encounters. Adding the same area twice (several
// maps share a name) merges them.
func (fb *FoundBuilder) Add(area string, periods []PeriodEncounters) {
	for _, p := range periods {
		for _, m := range p.Methods {
			for _, e := range m.Encounters {
				k := foundKey{e.Species, area, m.Method}
				a, ok := fb.acc[k]
				if !ok {
					a = &foundAgg{Found: Found{Area: area, Method: m.Method, MinLevel: e.MinLevel, MaxLevel: e.MaxLevel}}
					fb.acc[k] = a
					fb.order = append(fb.order, k)
				}
				a.MinLevel, a.MaxLevel = min(a.MinLevel, e.MinLevel), max(a.MaxLevel, e.MaxLevel)
				a.Chance = max(a.Chance, e.Chance)
				if p.Name != "" && !slices.Contains(a.periods, p.Name) {
					a.periods = append(a.periods, p.Name)
				}
			}
		}
	}
}

// Result lists each species' places, most likely first.
func (fb *FoundBuilder) Result() map[int][]Found {
	out := map[int][]Found{}
	for _, k := range fb.order {
		a := fb.acc[k]
		if len(a.periods) > 0 && len(a.periods) < fb.periods {
			a.When = strings.Join(a.periods, ", ")
		}
		out[k.species] = append(out[k.species], a.Found)
	}
	for _, list := range out {
		sort.SliceStable(list, func(i, j int) bool { return list[i].Chance > list[j].Chance })
	}
	return out
}
