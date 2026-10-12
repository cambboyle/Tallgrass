package gen3

import (
	"encoding/binary"
	"slices"
	"testing"
)

// put writes little-endian u32s into b at off.
func put(b []byte, off int, vs ...uint32) {
	for i, v := range vs {
		binary.LittleEndian.PutUint32(b[off+i*4:], v)
	}
}

// mapHeaderBytes is a 24-byte map header: layout, events, scripts and
// connections pointers, then music, layout ID, region map section.
func mapHeaderBytes(events uint32, section byte) []byte {
	h := make([]byte, 0x18)
	put(h, 0, 0x08100000, events, 0x08100100, 0)
	h[0x14] = section
	return h
}

// personBytes is a 24-byte object event: trainer type at 12, script at 16.
func personBytes(trainerType uint16, script uint32) []byte {
	o := make([]byte, 24)
	binary.LittleEndian.PutUint16(o[12:], trainerType)
	binary.LittleEndian.PutUint32(o[16:], script)
	return o
}

// trainerBattle is a "trainerbattle" command: 0x5C, type, trainer, local
// ID, then two text pointers.
func trainerBattle(typ byte, trainer uint16) []byte {
	b := make([]byte, 14)
	b[0], b[1] = 0x5C, typ
	binary.LittleEndian.PutUint16(b[2:], trainer)
	put(b, 6, 0x08200000, 0x08200100)
	return b
}

// padded extends a script to n bytes, since the code reads fixed-size
// windows of script data.
func padded(b []byte, n int) []byte { return append(b, make([]byte, n-len(b))...) }

// A group whose first slot was erased (Unbound's group 4) still yields the
// maps after it, and a group table that ends in a pointer back into itself
// (Unbound again) doesn't produce a phantom group.
func TestReadMapsSkipsErasedSlots(t *testing.T) {
	const groups, list0, hdr1, hdr2, events1, people1 = 0x08350000, 0x08351000, 0x08352000, 0x08352100, 0x08353000, 0x08354000

	table := make([]byte, 16)
	put(table, 0, list0, groups) // group 0, then a pointer back into the table

	lst := make([]byte, 16)
	put(lst, 0, 0xF7F7F7F7, hdr1, 0xFFFFFFFF, hdr2)

	ev := make([]byte, 20)
	ev[0] = 1 // one person
	put(ev, 4, people1)

	// Map 1 has a trainer whose script is a plain trainerbattle.
	script := padded(trainerBattle(0, 77), 128)
	read := regions{
		groups: table, list0: lst,
		hdr1: mapHeaderBytes(events1, 7), hdr2: mapHeaderBytes(0, 9),
		events1: ev, people1: personBytes(1, 0x08355000),
		0x08355000: script,
		0x08100000: make([]byte, 16), 0x08200000: make([]byte, 16),
	}.read

	maps, err := ReadMaps(read, groups)
	if err != nil {
		t.Fatal(err)
	}
	if len(maps) != 2 {
		t.Fatalf("read %d maps, want 2 (slots 1 and 3; the erased ones skipped, no phantom group): %+v", len(maps), maps)
	}
	if maps[0].Group != 0 || maps[0].Num != 1 || maps[0].Section != 7 || !slices.Equal(maps[0].Trainers, []int{77}) {
		t.Errorf("first map = %+v", maps[0])
	}
	if maps[1].Num != 3 || maps[1].Section != 9 {
		t.Errorf("second map = %+v", maps[1])
	}
}

// Gym leaders and bosses in some hacks open with text and branch several
// times before the battle, so their battle command lies well past the first
// 72 bytes. Objects marked as trainers are followed that far; others aren't,
// since shared scripts that far in could belong to someone else.
func TestDeepTrainerScripts(t *testing.T) {
	const groups, list0, hdr, events, people = 0x08350000, 0x08351000, 0x08352000, 0x08353000, 0x08354000
	const npcScript, tail = 0x08360000, 0x08361000

	// 100 bytes of text commands, then "goto tail"; the battle is in tail.
	body := make([]byte, 100)
	for i := range body {
		body[i] = 0x0F // loadword-ish filler that doesn't parse as a jump or battle
	}
	goTo := make([]byte, 5)
	goTo[0] = 0x05
	put(goTo, 1, tail)
	script := padded(append(body, goTo...), 320)

	build := func(trainerType uint16) ReadMemory {
		table := make([]byte, 8)
		put(table, 0, list0)
		lst := make([]byte, 4)
		put(lst, 0, hdr)
		ev := make([]byte, 20)
		ev[0] = 1
		put(ev, 4, people)
		return regions{
			groups: table, list0: lst, hdr: mapHeaderBytes(events, 1),
			events: ev, people: personBytes(trainerType, npcScript),
			npcScript: script, tail: padded(trainerBattle(8, 6), 320),
			0x08100000: make([]byte, 16), 0x08200000: make([]byte, 16),
		}.read
	}

	for _, c := range []struct {
		name        string
		trainerType uint16
		want        []int
	}{
		{"a trainer object", 1, []int{6}},
		{"a plain NPC", 0, nil},
	} {
		maps, err := ReadMaps(build(c.trainerType), groups)
		if err != nil || len(maps) != 1 {
			t.Fatalf("%s: %v %+v", c.name, err, maps)
		}
		if !slices.Equal(maps[0].Trainers, c.want) {
			t.Errorf("%s: trainers = %v, want %v", c.name, maps[0].Trainers, c.want)
		}
	}
}
