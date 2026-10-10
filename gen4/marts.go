package gen4

// Poké Marts. Map scripts open a shop with one of two commands:
//
//   - the standard mart (HeartGold/SoulSilver 0x113): every Poké Mart
//     clerk. Its stock is one table of (item, tier) pairs; the game turns
//     the player's badge count into a tier and offers every item up to it.
//   - a specialty shop (0x114 <id>): a fixed list, from a table of lists
//     (Goldenrod Dept. Store floors, herb shop and so on).
//
// The command table, the two handlers and their tables are found from the
// game's code, so hacks that move or extend them still work.

// Marts is the game's shop data.
type Marts struct {
	Standard []MartItem // stock of the standard Poké Mart
	Special  [][]int    // specialty shops' item lists, by ID
	ops      [2]uint16  // script commands: standard mart, specialty shop
	setVar   uint16     // script commands to set a var and call a shared script
	callStd  uint16
	std      [2]uint16 // shared scripts that open the standard mart, a specialty shop
}

type MartItem struct {
	Item   int
	Badges int // badges needed before the clerk sells it
}

// martOps are the shop script commands per family. Tier → badges needed
// comes from the standard mart handler's switch (HeartGold/SoulSilver:
// 0 badges → tier 1, 1–2 → 2, 3–4 → 3, 5–6 → 4, 7 → 5, 8+ → 6).
var martOps = map[string]struct {
	standard, special uint16
	tierBadges        []int // by tier
	setVar, callStd   uint16
	std               [2]uint16
}{
	"HeartGold/SoulSilver": {0x113, 0x114, []int{0, 0, 1, 3, 5, 7, 8}, 0x29, 0x14, [2]uint16{0x800, 0x804}},
}

// findMarts reads the shop tables. Nil if the game's layout isn't known or
// the code doesn't look as expected.
func findMarts(a *ARM9, family string, items int) *Marts {
	ops, ok := martOps[family]
	if !ok {
		return nil
	}
	cmds := scriptCommands(a)
	if int(ops.special) >= len(cmds) {
		return nil
	}
	m := &Marts{ops: [2]uint16{ops.standard, ops.special}, setVar: ops.setVar, callStd: ops.callStd, std: ops.std}
	inARM9 := func(v uint32) bool { return v >= a.Base && v < a.Base+uint32(len(a.Data)) }

	// Standard mart: a literal pointing at (item, tier) pairs.
	for _, lit := range thumbLiterals(a, cmds[ops.standard], 0x100) {
		if !inARM9(lit) {
			continue
		}
		var stock []MartItem
		for p := lit; ; p += 4 {
			b, err := a.Read(p, 4)
			if err != nil {
				break
			}
			item, tier := int(le.Uint16(b)), int(le.Uint16(b[2:]))
			if item == 0 || item >= items || tier < 1 || tier >= len(ops.tierBadges) {
				break
			}
			stock = append(stock, MartItem{Item: item, Badges: ops.tierBadges[tier]})
		}
		if len(stock) >= 5 {
			m.Standard = stock
			break
		}
	}

	// Specialty shops: a literal pointing at a table of pointers to
	// 0xFFFF-terminated item lists.
	for _, lit := range thumbLiterals(a, cmds[ops.special], 0x60) {
		if !inARM9(lit) {
			continue
		}
		var lists [][]int
		for p := lit; ; p += 4 {
			b, err := a.Read(p, 4)
			if err != nil || !inARM9(le.Uint32(b)) {
				break
			}
			list, ok := itemList(a, le.Uint32(b), items)
			if !ok {
				break
			}
			lists = append(lists, list)
		}
		if len(lists) >= 5 {
			m.Special = lists
			break
		}
	}
	if m.Standard == nil && m.Special == nil {
		return nil
	}
	return m
}

// itemList reads u16 items up to 0xFFFF.
func itemList(a *ARM9, at uint32, items int) ([]int, bool) {
	var list []int
	for p := at; len(list) <= 64; p += 2 {
		b, err := a.Read(p, 2)
		if err != nil {
			return nil, false
		}
		v := int(le.Uint16(b))
		if v == 0xFFFF {
			return list, len(list) > 0
		}
		if v == 0 || v >= items {
			return nil, false
		}
		list = append(list, v)
	}
	return nil, false
}

// scriptCommands finds the script command table: the longest run of Thumb
// function pointers into main RAM (hundreds of entries, some in overlays;
// nothing else comes close). Returns each handler's address.
func scriptCommands(a *ARM9) []uint32 {
	isFn := func(v uint32) bool { return v&1 == 1 && v > a.Base && v < MainRAM+MainRAMSize }
	bestAt, best := 0, 0
	for o := 0; o+4 <= len(a.Data); o += 4 {
		n := 0
		for o+(n+1)*4 <= len(a.Data) && isFn(le.Uint32(a.Data[o+n*4:])) {
			n++
		}
		if n > best {
			bestAt, best = o, n
		}
		if n > 0 {
			o += (n - 1) * 4
		}
	}
	if best < 500 {
		return nil
	}
	out := make([]uint32, best)
	for i := range out {
		out[i] = le.Uint32(a.Data[bestAt+i*4:]) &^ 1
	}
	return out
}

// thumbLiterals returns the words loaded by "ldr rN, [pc, #imm]" in the
// first n bytes of a Thumb function, in order.
func thumbLiterals(a *ARM9, fn uint32, n int) []uint32 {
	var out []uint32
	for pc := fn; pc < fn+uint32(n); pc += 2 {
		b, err := a.Read(pc, 2)
		if err != nil {
			break
		}
		ins := le.Uint16(b)
		if ins&0xF800 != 0x4800 {
			continue
		}
		lit := (pc+4)&^3 + uint32(ins&0xFF)*4
		if w, err := a.Read(lit, 4); err == nil {
			out = append(out, le.Uint32(w))
		}
	}
	return out
}

// scriptMarts finds the shops a map's script file opens. Clerks either
// run a shop command directly, or set var 0x8004 to the list and call a
// shared script that does (HeartGold/SoulSilver: callstd 0x800 standard,
// 0x804 specialty). A command's bytes can also be another command's
// offset, so matches right after a jump or movement are skipped.
func (m *Marts) scriptMarts(b []byte) (standard bool, special []int) {
	if m == nil {
		return false, nil
	}
	addSpecial := func(id int) {
		if id < len(m.Special) && !containsInt(special, id) {
			special = append(special, id)
		}
	}
	afterJump := func(o int) bool {
		return o >= 2 && (b[o-2] == 0x16 || b[o-2] == 0x1A) && b[o-1] == 0 || // jump, call
			o >= 3 && (b[o-3] == 0x1C || b[o-3] == 0x1D) && b[o-2] == 0 || // conditional jump, call
			o >= 4 && b[o-4] == 0x5E && b[o-3] == 0 // apply movement (object, offset)
	}
	// Code starts after the header of script offsets.
	start := len(b)
	for _, off := range scriptOffsets(b) {
		start = min(start, off)
	}
	for o := start; o+4 <= len(b); o++ {
		if afterJump(o) {
			continue
		}
		op, arg := le.Uint16(b[o:]), int(le.Uint16(b[o+2:]))
		switch {
		case op == m.ops[0] && arg == 0 && m.Standard != nil:
			standard = true
		case op == m.ops[1] && arg > 0: // 0x114 0x0000 is too often a jump offset; list 0 comes via callstd
			addSpecial(arg)
		case op == m.setVar && arg == 0x8004 && o+10 <= len(b) && le.Uint16(b[o+6:]) == m.callStd:
			switch le.Uint16(b[o+8:]) {
			case m.std[0]:
				standard = standard || m.Standard != nil
			case m.std[1]:
				addSpecial(int(le.Uint16(b[o+4:])))
			}
		}
	}
	return standard, special
}
