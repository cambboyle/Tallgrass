package gen3

import "encoding/binary"

// MapData is what one map holds, read from its header and events: where
// it leads, items lying around, shops and trainers. IDs only; names come
// from the caller.
type MapData struct {
	Group, Num  int
	Section     int // region map section (the area's name)
	Connections []MapConnection
	Warps       []MapRef // doors and stairs, one per destination
	Items       []MapItem
	Shops       [][]int // each shop's item IDs
	Trainers    []int
}

type MapRef struct{ Group, Num int }

type MapConnection struct {
	Direction string // "north", "south", "east", "west", "dive", "emerge"
	MapRef
}

type MapItem struct {
	Item     int
	Quantity int
	Hidden   bool
	X, Y     int
}

var directions = map[byte]string{1: "south", 2: "north", 3: "west", 4: "east", 5: "dive", 6: "emerge"}

// ReadMaps reads every map in the map groups table. Each group is a list of
// map header pointers; the lists sit back to back, so one ends where the
// next begins.
//
// Map header: layout, events, map scripts, connections (pointers), music,
// layout ID, region map section (byte 0x14). Events: counts of people,
// warps, coord events and background events, then a pointer to each list.
func ReadMaps(read ReadMemory, groups uint32) ([]MapData, error) {
	le := binary.LittleEndian
	ptr := func(addr uint32) uint32 {
		b, err := read(addr, 4)
		if err != nil {
			return 0
		}
		return le.Uint32(b)
	}
	var lists []uint32
	for g := 0; g < 64; g++ {
		p := ptr(groups + uint32(g*4))
		// The table ends at the first non-pointer, or at a pointer back into
		// the table itself (Unbound has one right after its last group).
		if !isROMPointer(p) || (p >= groups && p < groups+64*4) {
			break
		}
		lists = append(lists, p)
	}
	var maps []MapData
	for g, list := range lists {
		end := list + 256*4
		for _, other := range lists {
			if other > list && other < end {
				end = other
			}
		}
		// Hacks leave erased slots inside a group (Unbound's group 4 starts
		// with one), so skip bad entries; a long run of them means the list
		// has ended and the rest is whatever else sits in the ROM.
		bad := 0
		for n := 0; list+uint32(n*4) < end && bad < 16; n++ {
			header := ptr(list + uint32(n*4))
			h := mapHeader(read, header)
			if h == nil {
				bad++
				continue
			}
			bad = 0
			m := MapData{Group: g, Num: n, Section: int(h[0x14])}
			readConnections(read, le.Uint32(h[12:]), &m)
			if events := le.Uint32(h[4:]); isROMPointer(events) {
				readEvents(read, events, &m)
			}
			maps = append(maps, m)
		}
	}
	return maps, nil
}

// mapHeader returns a map's 24-byte header if it looks like one: pointers
// to the layout (always set), events, scripts and connections (each set or
// null). Anything else is an erased or unrelated entry.
func mapHeader(read ReadMemory, header uint32) []byte {
	if !isROMPointer(header) {
		return nil
	}
	h, err := read(header, 0x18)
	if err != nil {
		return nil
	}
	le := binary.LittleEndian
	ok := func(p uint32) bool { return p == 0 || isROMPointer(p) }
	if !isROMPointer(le.Uint32(h[0:])) || !ok(le.Uint32(h[4:])) || !ok(le.Uint32(h[8:])) || !ok(le.Uint32(h[12:])) {
		return nil
	}
	return h
}

func readConnections(read ReadMemory, p uint32, m *MapData) {
	if !isROMPointer(p) {
		return
	}
	hdr, err := read(p, 8)
	if err != nil {
		return
	}
	count, list := int(binary.LittleEndian.Uint32(hdr)), binary.LittleEndian.Uint32(hdr[4:])
	if count <= 0 || count > 16 || !isROMPointer(list) {
		return
	}
	b, err := read(list, count*12)
	if err != nil {
		return
	}
	for i := 0; i < count; i++ {
		c := b[i*12:]
		if dir, ok := directions[c[0]]; ok {
			m.Connections = append(m.Connections, MapConnection{Direction: dir, MapRef: MapRef{int(c[8]), int(c[9])}})
		}
	}
}

func readEvents(read ReadMemory, events uint32, m *MapData) {
	le := binary.LittleEndian
	ev, err := read(events, 20)
	if err != nil {
		return
	}
	people, warps, bgs := int(ev[0]), int(ev[1]), int(ev[3])

	// People: 24 bytes each, script pointer at 16.
	if p := le.Uint32(ev[4:]); people > 0 && isROMPointer(p) {
		if objs, err := read(p, people*24); err == nil {
			seen := map[int]bool{}
			for i := 0; i < people; i++ {
				o := objs[i*24:]
				script := le.Uint32(o[16:])
				ids := scriptTrainers(read, script)
				if tt := le.Uint16(o[12:]); len(ids) == 0 && tt >= 1 && tt <= 3 {
					ids = scriptTrainersDeep(read, script)
				}
				for _, id := range ids {
					if !seen[id] {
						seen[id] = true
						m.Trainers = append(m.Trainers, id)
					}
				}
				if item, qty, ok := scriptFindItem(read, script); ok {
					m.Items = append(m.Items, MapItem{Item: item, Quantity: qty, X: int(int16(le.Uint16(o[4:]))), Y: int(int16(le.Uint16(o[6:])))})
				}
				if shop := scriptShop(read, script); shop != nil {
					m.Shops = append(m.Shops, shop)
				}
			}
		}
	}

	// Warps: x, y, elevation, warp ID, map number, map group (8 bytes).
	if p := le.Uint32(ev[8:]); warps > 0 && isROMPointer(p) {
		if b, err := read(p, warps*8); err == nil {
			seen := map[MapRef]bool{}
			for i := 0; i < warps; i++ {
				to := MapRef{int(b[i*8+7]), int(b[i*8+6])}
				if !seen[to] && to != (MapRef{m.Group, m.Num}) && to.Group != 0x7F {
					seen[to] = true
					m.Warps = append(m.Warps, to)
				}
			}
		}
	}

	// Background events: x, y, elevation, kind, then data. Kind 7 is a
	// hidden item whose low 16 bits are the item; FireRed packs the
	// quantity into bits 24–30.
	if p := le.Uint32(ev[16:]); bgs > 0 && isROMPointer(p) {
		if b, err := read(p, bgs*12); err == nil {
			for i := 0; i < bgs; i++ {
				e := b[i*12:]
				if e[5] != 7 {
					continue
				}
				data := le.Uint32(e[8:])
				qty := int(data >> 24 & 0x7F)
				if qty == 0 {
					qty = 1
				}
				if item := int(data & 0xFFFF); item != 0 {
					m.Items = append(m.Items, MapItem{Item: item, Quantity: qty, Hidden: true,
						X: int(int16(le.Uint16(e[0:]))), Y: int(int16(le.Uint16(e[2:])))})
				}
			}
		}
	}
}

// scriptFindItem recognises an item ball: setorcopyvar 0x8000 = item,
// setorcopyvar 0x8001 = quantity, then the standard "find item" script.
func scriptFindItem(read ReadMemory, script uint32) (item, qty int, ok bool) {
	if !isROMPointer(script) {
		return 0, 0, false
	}
	b, err := read(script, 10)
	if err != nil || b[0] != 0x1A || b[1] != 0x00 || b[2] != 0x80 || b[5] != 0x1A || b[6] != 0x01 || b[7] != 0x80 {
		return 0, 0, false
	}
	return int(binary.LittleEndian.Uint16(b[3:])), int(binary.LittleEndian.Uint16(b[8:])), true
}

// scriptShop finds a pokemart command (0x86, then a pointer to a list of
// item IDs ending in 0) near the start of a clerk's script.
func scriptShop(read ReadMemory, script uint32) []int {
	if !isROMPointer(script) {
		return nil
	}
	b, err := read(script, 96)
	if err != nil {
		return nil
	}
	for o := 0; o+5 <= len(b); o++ {
		if b[o] != 0x86 {
			continue
		}
		list := binary.LittleEndian.Uint32(b[o+1:])
		if !isROMPointer(list) {
			continue
		}
		items, err := read(list, 128)
		if err != nil {
			continue
		}
		var shop []int
		for k := 0; k+2 <= len(items); k += 2 {
			v := int(binary.LittleEndian.Uint16(items[k:]))
			if v == 0 {
				if len(shop) > 0 {
					return shop
				}
				break
			}
			if v > 0x3FF {
				break // not an item list
			}
			shop = append(shop, v)
		}
	}
	return nil
}
