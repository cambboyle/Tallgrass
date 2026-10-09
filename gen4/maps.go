package gen4

import (
	"fmt"
	"sort"
)

// MapInfo is what one DS map holds: where its doors lead, the items lying
// around (and hidden) and the trainers standing on it.
type MapInfo struct {
	ID       int
	Section  int // location name index
	Warps    []int
	Items    []MapItem
	Trainers []int
}

type MapItem struct {
	Item     string
	Quantity int
	Hidden   bool
	X, Z     int
}

var eventArchives = map[string]string{
	"Platinum":             "/fielddata/eventdata/zone_event.narc",
	"HeartGold/SoulSilver": "/a/0/3/2",
}

// Script command that starts a trainer battle (trainer u16, partner u16).
var trainerBattleOps = map[string]uint16{
	"Platinum":             0x00E5,
	"HeartGold/SoulSilver": 0x00D5,
}

var scriptArchives = map[string]string{
	"Platinum":             "/fielddata/script/scr_seq.narc",
	"HeartGold/SoulSilver": "/a/0/1/2",
}

// Maps reads every map's events. Item balls name their item through a
// script file of "set 0x8008 = item, 0x8009 = quantity" scripts; hidden
// items through a table in the game's code (item, quantity, …; 8 bytes
// each). Script IDs index both (see zoneEvents).
//
// Event files use the whole region's coordinates and sometimes include a
// neighbouring map's objects; an object listed on several maps is kept
// only on those whose own objects surround it.
func (d *Data) Maps() ([]MapInfo, error) {
	x, err := d.extras()
	if err != nil {
		return nil, err
	}
	evPath, scrPath := eventArchives[d.Layout.Family], scriptArchives[d.Layout.Family]
	if evPath == "" || d.Layout.MapHeaders == 0 {
		return nil, fmt.Errorf("no map events for %s", d.Layout.Family)
	}
	rom, err := OpenROM(d.romPath)
	if err != nil {
		return nil, err
	}
	defer rom.Close()
	events, err := rom.Archive(evPath)
	if err != nil {
		return nil, err
	}
	var balls [][2]int
	scripts, err := rom.Archive(scrPath)
	if err == nil {
		balls = findItemBalls(scripts)
	}
	hidden := findHiddenItems(x.arm9.Data, len(d.Items))

	var maps []MapInfo
	parsed := []zoneEvents{}
	for id := 0; ; id++ {
		hb, err := x.arm9.Read(d.Layout.MapHeaders+uint32(id*mapHeaderSize), mapHeaderSize)
		if err != nil || !d.Layout.realHeader(hb) {
			break
		}
		h := d.Layout.MapHeader(hb)
		ev := zoneEvents{}
		if e := int(le.Uint16(hb[0x10:])); e < len(events) {
			ev, _ = parseZoneEvents(events[e])
		}
		m := MapInfo{ID: id, Section: h.Section}
		// Battles started by the map's own scripts: gym leaders, rivals
		// and other scripted fights.
		// The script file is a u16 at 0x04 in Platinum, 0x06 in
		// HeartGold/SoulSilver.
		scriptAt := 6
		if d.Layout.Family == "Platinum" {
			scriptAt = 4
		}
		if sf := int(le.Uint16(hb[scriptAt:])); sf < len(scripts) {
			for _, t := range scriptBattles(scripts[sf], trainerBattleOps[d.Layout.Family], len(d.TrainerNames)) {
				if !containsInt(m.Trainers, t) {
					m.Trainers = append(m.Trainers, t)
				}
			}
		}
		maps = append(maps, m)
		parsed = append(parsed, ev)
	}

	// Where each script-numbered object appears, to settle duplicates.
	type spot struct{ m, x, z int }
	seen := map[int][]spot{}
	for i, ev := range parsed {
		for _, s := range ev.signs {
			seen[s.script] = append(seen[s.script], spot{i, s.x, s.z})
		}
		for _, p := range ev.people {
			seen[p.script] = append(seen[p.script], spot{i, p.x, p.z})
		}
	}
	// bounds is the box around a map's objects other than one script.
	bounds := func(m, except int) (minX, maxX, minZ, maxZ int, ok bool) {
		minX, minZ = 1<<30, 1<<30
		maxX, maxZ = -1<<30, -1<<30
		for _, s := range parsed[m].signs {
			if s.script != except {
				minX, maxX, minZ, maxZ, ok = min(minX, s.x), max(maxX, s.x), min(minZ, s.z), max(maxZ, s.z), true
			}
		}
		for _, p := range parsed[m].people {
			if p.script != except {
				minX, maxX, minZ, maxZ, ok = min(minX, p.x), max(maxX, p.x), min(minZ, p.z), max(maxZ, p.z), true
			}
		}
		return
	}
	belongs := func(script, m, px, pz int) bool {
		spots := seen[script]
		if len(spots) < 2 {
			return true
		}
		inside := 0
		for _, s := range spots {
			if x0, x1, z0, z1, ok := bounds(s.m, script); ok && s.x >= x0 && s.x <= x1 && s.z >= z0 && s.z <= z1 {
				inside++
			}
		}
		if inside == 0 {
			return true // can't tell: keep it everywhere
		}
		x0, x1, z0, z1, ok := bounds(m, script)
		return ok && px >= x0 && px <= x1 && pz >= z0 && pz <= z1
	}

	for i, ev := range parsed {
		m := &maps[i]
		for _, w := range ev.warps {
			if w != m.ID && w < len(maps) && !containsInt(m.Warps, w) {
				m.Warps = append(m.Warps, w)
			}
		}
		for _, p := range ev.people {
			switch {
			case p.script >= scriptTrainers && p.script < scriptDoubles+1000:
				id := p.script - scriptTrainers + 1
				if p.script >= scriptDoubles {
					id = p.script - scriptDoubles + 1
				}
				if !containsInt(m.Trainers, id) && belongs(p.script, i, p.x, p.z) {
					m.Trainers = append(m.Trainers, id)
				}
			case p.script >= scriptItemBalls && p.script < scriptHiddenItems:
				n := p.script - scriptItemBalls
				if n < len(balls) && balls[n][0] != 0 && belongs(p.script, i, p.x, p.z) {
					m.Items = append(m.Items, MapItem{Item: Name(d.Items, balls[n][0]), Quantity: balls[n][1], X: p.x, Z: p.z})
				}
			}
		}
		for _, s := range ev.signs {
			n := s.script - scriptHiddenItems
			if n >= 0 && n < len(hidden) && hidden[n][0] != 0 && belongs(s.script, i, s.x, s.z) {
				m.Items = append(m.Items, MapItem{Item: Name(d.Items, hidden[n][0]), Quantity: hidden[n][1], Hidden: true, X: s.x, Z: s.z})
			}
		}
		sort.Ints(m.Trainers)
	}
	return maps, nil
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// scriptBattles finds trainer battle commands in a map's script file: the
// command, a real trainer ID, then a partner that's 0 or another trainer.
func scriptBattles(b []byte, op uint16, trainers int) []int {
	var out []int
	if op == 0 {
		return nil
	}
	for o := 0; o+6 <= len(b); o++ {
		if le.Uint16(b[o:]) != op {
			continue
		}
		t, partner := int(le.Uint16(b[o+2:])), int(le.Uint16(b[o+4:]))
		if t > 0 && t < trainers && partner < trainers {
			out = append(out, t)
			if partner > 0 {
				out = append(out, partner)
			}
		}
	}
	return out
}

// findItemBalls finds the script file whose scripts all start "set 0x8008
// = item; set 0x8009 = quantity" (opcode 0x28 in Platinum, 0x29 in
// HeartGold/SoulSilver) and returns each script's (item, quantity).
func findItemBalls(files [][]byte) [][2]int {
	for _, b := range files {
		offs := scriptOffsets(b)
		if len(offs) < 50 {
			continue
		}
		var out [][2]int
		good := 0
		for _, o := range offs {
			if o+12 <= len(b) && (b[o] == 0x28 || b[o] == 0x29) && b[o+1] == 0 && le.Uint16(b[o+2:]) == 0x8008 &&
				b[o+6] == b[o] && le.Uint16(b[o+8:]) == 0x8009 {
				out = append(out, [2]int{int(le.Uint16(b[o+4:])), int(le.Uint16(b[o+10:]))})
				good++
			} else {
				out = append(out, [2]int{})
			}
		}
		if good*10 >= len(offs)*9 {
			return out
		}
	}
	return nil
}

// scriptOffsets reads a script file's header: u32 offsets, each relative
// to the byte after it, ending with the marker 0xFD13.
func scriptOffsets(b []byte) []int {
	var offs []int
	for o := 0; o+4 <= len(b); o += 4 {
		if le.Uint16(b[o:]) == 0xFD13 {
			return offs
		}
		at := o + 4 + int(le.Uint32(b[o:]))
		if at <= 0 || at >= len(b) {
			return nil
		}
		offs = append(offs, at)
	}
	return nil
}

// findHiddenItems finds the hidden item table in the game's code: the
// longest run of 8-byte entries starting item (u16, a real item) and
// quantity (u8, 1–10). Returns (item, quantity) by hidden item number.
func findHiddenItems(arm9 []byte, items int) [][2]int {
	bestAt, best := 0, 0
	for o := 0; o+8 <= len(arm9); o += 4 {
		run := 0
		for ; o+(run+1)*8 <= len(arm9); run++ {
			e := arm9[o+run*8:]
			item, qty := int(le.Uint16(e)), e[2]
			if item == 0 || item >= items || qty == 0 || qty > 10 {
				break
			}
		}
		if run > best {
			best, bestAt = run, o
		}
		if run > 0 {
			o += (run - 1) * 8
		}
	}
	if best < 100 {
		return nil
	}
	out := make([][2]int, best)
	for i := range out {
		e := arm9[bestAt+i*8:]
		out[i] = [2]int{int(le.Uint16(e)), int(e[2])}
	}
	return out
}
