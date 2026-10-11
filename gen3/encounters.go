package gen3

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// WildHeader is one map's entry in a wild encounter table: pointers to its
// encounter lists (0 where it has none). Raw has every pointer in the
// header; Info picks land, water, rock smash and fishing from it.
type WildHeader struct {
	Group, Num uint8
	Info       [6]uint32
	Raw        []uint32
}

// WildLayout describes a game's header: its size, and which of its
// pointers are land, water, rock smash and fishing. Vanilla: 20 bytes,
// pointers in that order. ROWE: 32 bytes, land by day and by night, water,
// rock smash, fishing, hidden and one more.
type WildLayout struct {
	HeaderSize int
	Slots      [6]int // land, water, rock smash, fishing, hidden (DexNav), headbutt; -1 for none
}

var vanillaWild = WildLayout{HeaderSize: 20, Slots: [6]int{0, 1, 2, 3, -1, -1}}

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
	// DexNav's hidden Pokémon (decomp hacks): the player picks which to
	// search for, so there are no odds; chances stay 0.
	{"Hidden (DexNav)", 4, 0, []int{0, 0, 0}},
	// Headbutting a tree (R.O.W.E.): a 50% chance of an encounter, then the
	// rock smash slot odds.
	{"Headbutt", 5, 0, []int{60, 30, 5, 4, 1}},
}

var slotCounts = [6]int{12, 5, 5, 10, 3, 5}

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
	// LandSlot, if 0 or more, is the header pointer this period's land
	// encounters use (games keeping day and night in one header); -1 to
	// follow the layout.
	LandSlot int
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
	layout  WildLayout
	main    uint32
	periods []Period
	tables  map[uint32][]WildHeader
	maps    map[[2]uint8][]PeriodEncounters
}

// NewWild takes the main table's address and any time-of-day tables. Like
// CFRU, a period table's empty methods fall back to the main table.
func NewWild(read ReadMemory, main uint32, periods []Period) *Wild {
	w := &Wild{read: read, layout: vanillaWild, main: main, periods: periods}
	w.Reset()
	return w
}

// WithLayout sets a non-vanilla header layout.
func (w *Wild) WithLayout(l WildLayout) *Wild {
	if l.HeaderSize >= 8 {
		w.layout = l
	}
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

	mainRaw, err := w.lookup(w.main, group, num)
	if err != nil {
		return nil, err
	}

	periods := w.periods
	if len(periods) == 0 {
		periods = []Period{{}}
	}

	var result []PeriodEncounters
	for _, p := range periods {
		info := w.info(mainRaw, p.LandSlot)
		if p.Headers != 0 {
			ownRaw, err := w.lookup(p.Headers, group, num)
			if err != nil {
				return nil, err
			}
			own := w.info(ownRaw, -1)
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

// info picks land, water, rock smash and fishing out of a header's
// pointers; landSlot (if 0 or more) overrides where land comes from.
func (w *Wild) info(raw []uint32, landSlot int) [6]uint32 {
	var info [6]uint32
	for i, slot := range w.layout.Slots {
		if i == 0 && landSlot >= 0 {
			slot = landSlot
		}
		if slot >= 0 && slot < len(raw) {
			info[i] = raw[slot]
		}
	}
	return info
}

// lookup finds a map's pointers in a header table; nil if absent.
func (w *Wild) lookup(table uint32, group, num uint8) ([]uint32, error) {
	headers, ok := w.tables[table]
	if !ok {
		var err error
		headers, err = readWildHeaders(w.read, table, w.layout)
		if err != nil {
			return nil, err
		}
		w.tables[table] = headers
	}
	for _, h := range headers {
		if h.Group == group && h.Num == num {
			return h.Raw, nil
		}
	}
	return nil, nil
}

// ReadWildHeaders reads a vanilla-layout header table up to its 0xFF
// terminator.
func ReadWildHeaders(read ReadMemory, addr uint32) ([]WildHeader, error) {
	return readWildHeaders(read, addr, vanillaWild)
}

func readWildHeaders(read ReadMemory, addr uint32, l WildLayout) ([]WildHeader, error) {
	const batch = 64
	size := l.HeaderSize
	var headers []WildHeader
	for {
		b, err := read(addr+uint32(len(headers)*size), batch*size)
		if err != nil {
			return nil, err
		}
		for i := 0; i < batch; i++ {
			e := b[i*size:]
			if e[0] == 0xFF {
				return headers, nil
			}
			h := WildHeader{Group: e[0], Num: e[1]}
			for j := 4; j+4 <= size; j += 4 {
				h.Raw = append(h.Raw, binary.LittleEndian.Uint32(e[j:]))
			}
			for j, slot := range l.Slots {
				if slot >= 0 && slot < len(h.Raw) {
					h.Info[j] = h.Raw[slot]
				}
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
func (w *Wild) readMethods(info [6]uint32) ([]MethodEncounters, error) {
	var slots [6][]byte
	for i, ptr := range info {
		if ptr == 0 {
			continue
		}
		hdr, err := w.read(ptr, 8)
		if err != nil {
			return nil, err
		}
		if hdr[0] == 0 && i < 4 { // encounter rate 0: never happens (hidden and headbutt lists ignore it)
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
