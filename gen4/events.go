package gen4

// Zone events: each map header names an events file (u16 at 0x10) in
// /fielddata/eventdata/zone_event.narc (Platinum) or /a/0/3/2
// (HeartGold/SoulSilver). It holds four lists, each a u32 count then
// entries:
//
//   - background events (signs, hidden items), 20 bytes: script u16,
//     type u16, x, z, y (s32), then a u32
//   - people, 32 bytes: ID, graphics, movement, type, flag, script,
//     facing, sight range, 2 unknown, x range, y range, x, y (all u16),
//     z (s32)
//   - warps, 12 bytes: x, z, destination map, destination warp (u16),
//     then a u32
//   - triggers, 16 bytes
//
// Script IDs carry meaning by range: 3000+ are trainers (ID = script −
// 2999), 7000+ item balls, 8000+ hidden items.
type zoneEvents struct {
	signs  []zoneSign
	people []zonePerson
	warps  []int // destination map IDs
}

type zoneSign struct{ script, x, z int }

type zonePerson struct{ gfx, script, x, z int }

const (
	scriptTrainers    = 3000
	scriptDoubles     = 5000 // second trainer of a double battle
	scriptItemBalls   = 7000
	scriptHiddenItems = 8000
)

func parseZoneEvents(b []byte) (zoneEvents, bool) {
	var ev zoneEvents
	o := 0
	count := func(size int) (int, bool) {
		if o+4 > len(b) {
			return 0, false
		}
		n := int(le.Uint32(b[o:]))
		o += 4
		if n < 0 || n > 512 || o+n*size > len(b) {
			return 0, false
		}
		return n, true
	}
	n, ok := count(20)
	if !ok {
		return ev, false
	}
	for i := 0; i < n; i++ {
		e := b[o+i*20:]
		ev.signs = append(ev.signs, zoneSign{int(le.Uint16(e)), int(int32(le.Uint32(e[4:]))), int(int32(le.Uint32(e[8:])))})
	}
	o += n * 20
	if n, ok = count(32); !ok {
		return ev, false
	}
	for i := 0; i < n; i++ {
		e := b[o+i*32:]
		ev.people = append(ev.people, zonePerson{gfx: int(le.Uint16(e[2:])), script: int(le.Uint16(e[10:])),
			x: int(le.Uint16(e[24:])), z: int(le.Uint16(e[26:]))})
	}
	o += n * 32
	if n, ok = count(12); !ok {
		return ev, false
	}
	for i := 0; i < n; i++ {
		ev.warps = append(ev.warps, int(le.Uint16(b[o+i*12+4:])))
	}
	return ev, true
}
