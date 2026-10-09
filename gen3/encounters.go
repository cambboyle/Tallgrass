package gen3

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// WildHeader is one map's entry in a wild encounter table: pointers to its
// land, water, rock smash and fishing encounter lists (0 if it has none).
type WildHeader struct {
	Group, Num uint8
	Info       [4]uint32
}

const wildHeaderSize = 20

// Methods, in the order of WildHeader.Info, with each slot's chance in
// percent. Fishing shares one list of 10 slots across the three rods.
var methods = []struct {
	name      string
	info      int
	firstSlot int
	rates     []int
}{
	{"Grass", 0, 0, []int{20, 20, 10, 10, 10, 10, 5, 5, 4, 4, 1, 1}},
	{"Surfing", 1, 0, []int{60, 30, 5, 4, 1}},
	{"Rock Smash", 2, 0, []int{60, 30, 5, 4, 1}},
	{"Old Rod", 3, 0, []int{70, 30}},
	{"Good Rod", 3, 2, []int{60, 20, 20}},
	{"Super Rod", 3, 5, []int{40, 40, 15, 4, 1}},
}

var slotCounts = [4]int{12, 5, 5, 10}

// Encounter is one species for one method, with its slots merged.
type Encounter struct {
	Species  int `json:"species"`
	MinLevel int `json:"minLevel"`
	MaxLevel int `json:"maxLevel"`
	Chance   int `json:"chance"` // percent
}

type MethodEncounters struct {
	Method     string      `json:"method"`
	Encounters []Encounter `json:"encounters"` // most likely first
}

// Period is a time-of-day encounter table. Hours run from FromHour up to
// (not including) ToHour, wrapping past midnight when ToHour < FromHour.
type Period struct {
	Name     string
	Headers  uint32
	FromHour int
	ToHour   int
}

func (p Period) Contains(hour int) bool {
	if p.FromHour <= p.ToHour {
		return hour >= p.FromHour && hour < p.ToHour
	}
	return hour >= p.FromHour || hour < p.ToHour
}

type PeriodEncounters struct {
	Period
	Methods []MethodEncounters
}

// Wild reads wild encounter tables from ROM, caching each table and map.
// Not safe for concurrent use.
type Wild struct {
	read    ReadMemory
	main    uint32
	periods []Period
	tables  map[uint32][]WildHeader
	maps    map[[2]uint8][]PeriodEncounters
}

// NewWild takes the main table's address and any time-of-day tables. Like
// CFRU, a period table's empty methods fall back to the main table.
func NewWild(read ReadMemory, main uint32, periods []Period) *Wild {
	w := &Wild{read: read, main: main, periods: periods}
	w.Reset()
	return w
}

func (w *Wild) Reset() {
	w.tables = map[uint32][]WildHeader{}
	w.maps = map[[2]uint8][]PeriodEncounters{}
}

// ForMap returns the map's encounters for each period (or one unnamed period
// if the game has no time-of-day tables). Maps without encounters return nil.
func (w *Wild) ForMap(group, num uint8) ([]PeriodEncounters, error) {
	key := [2]uint8{group, num}
	if cached, ok := w.maps[key]; ok {
		return cached, nil
	}

	mainInfo, err := w.lookup(w.main, group, num)
	if err != nil {
		return nil, err
	}

	periods := w.periods
	if len(periods) == 0 {
		periods = []Period{{}}
	}

	var result []PeriodEncounters
	for _, p := range periods {
		info := mainInfo
		if p.Headers != 0 {
			own, err := w.lookup(p.Headers, group, num)
			if err != nil {
				return nil, err
			}
			for i := range info {
				if own[i] != 0 {
					info[i] = own[i]
				}
			}
		}
		methods, err := w.readMethods(info)
		if err != nil {
			return nil, err
		}
		if len(methods) > 0 {
			result = append(result, PeriodEncounters{Period: p, Methods: methods})
		}
	}

	w.maps[key] = result
	return result, nil
}

// lookup finds a map's info pointers in a header table; all zero if absent.
func (w *Wild) lookup(table uint32, group, num uint8) ([4]uint32, error) {
	headers, ok := w.tables[table]
	if !ok {
		var err error
		headers, err = ReadWildHeaders(w.read, table)
		if err != nil {
			return [4]uint32{}, err
		}
		w.tables[table] = headers
	}
	for _, h := range headers {
		if h.Group == group && h.Num == num {
			return h.Info, nil
		}
	}
	return [4]uint32{}, nil
}

// ReadWildHeaders reads a header table up to its 0xFF terminator.
func ReadWildHeaders(read ReadMemory, addr uint32) ([]WildHeader, error) {
	const batch = 64
	var headers []WildHeader
	for {
		b, err := read(addr+uint32(len(headers)*wildHeaderSize), batch*wildHeaderSize)
		if err != nil {
			return nil, err
		}
		for i := 0; i < batch; i++ {
			e := b[i*wildHeaderSize:]
			if e[0] == 0xFF {
				return headers, nil
			}
			h := WildHeader{Group: e[0], Num: e[1]}
			for j := range h.Info {
				h.Info[j] = binary.LittleEndian.Uint32(e[4+j*4:])
			}
			headers = append(headers, h)
		}
		if len(headers) > 2000 {
			return nil, fmt.Errorf("no end marker in wild header table at %08X", addr)
		}
	}
}

// readMethods reads each encounter list (an encounter rate byte, then a
// pointer to slots of min level, max level, species).
func (w *Wild) readMethods(info [4]uint32) ([]MethodEncounters, error) {
	var slots [4][]byte
	for i, ptr := range info {
		if ptr == 0 {
			continue
		}
		hdr, err := w.read(ptr, 8)
		if err != nil {
			return nil, err
		}
		if hdr[0] == 0 { // encounter rate 0: never happens
			continue
		}
		slots[i], err = w.read(binary.LittleEndian.Uint32(hdr[4:]), slotCounts[i]*4)
		if err != nil {
			return nil, err
		}
	}

	var result []MethodEncounters
	for _, m := range methods {
		if slots[m.info] == nil {
			continue
		}
		bySpecies := map[int]*Encounter{}
		var order []*Encounter
		for i, rate := range m.rates {
			s := slots[m.info][(m.firstSlot+i)*4:]
			species := int(binary.LittleEndian.Uint16(s[2:]))
			minLvl, maxLvl := int(s[0]), int(s[1])
			e, ok := bySpecies[species]
			if !ok {
				e = &Encounter{Species: species, MinLevel: minLvl, MaxLevel: maxLvl}
				bySpecies[species] = e
				order = append(order, e)
			}
			e.Chance += rate
			e.MinLevel = min(e.MinLevel, minLvl)
			e.MaxLevel = max(e.MaxLevel, maxLvl)
		}
		sort.SliceStable(order, func(i, j int) bool { return order[i].Chance > order[j].Chance })
		me := MethodEncounters{Method: m.name}
		for _, e := range order {
			me.Encounters = append(me.Encounters, *e)
		}
		result = append(result, me)
	}
	return result, nil
}
