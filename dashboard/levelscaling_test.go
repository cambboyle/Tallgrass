package dashboard

import (
	"encoding/binary"
	"fmt"
	"testing"

	"tallgrass/profile"
)

// R.O.W.E.'s level scaling: off unless its flags are set; then the level
// is cap(2) + cap(5) for the difficulty and stage, 100 at stage 18, or a
// flat 5 with only flag 0xE3A.
func TestScaledLevel(t *testing.T) {
	const flags, vars, table = 0x02001000, 0x02002000, 0x09000000
	mem := map[uint32]byte{}
	setFlag := func(n int, on bool) {
		b := mem[flags+uint32(n/8)]
		if on {
			b |= 1 << (n % 8)
		} else {
			b &^= 1 << (n % 8)
		}
		mem[flags+uint32(n/8)] = b
	}
	setVar := func(n, v int) {
		mem[vars+uint32(2*(n-0x4000))] = byte(v)
		mem[vars+uint32(2*(n-0x4000))+1] = byte(v >> 8)
	}
	setCap := func(difficulty, kind, stage, v int) {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], uint16(v))
		at := table + uint32(difficulty*304+kind*38+stage*2)
		mem[at], mem[at+1] = b[0], b[1]
	}
	read := func(addr uint32, n int) ([]byte, error) {
		out := make([]byte, n)
		for i := range out {
			out[i] = mem[addr+uint32(i)]
		}
		return out, nil
	}
	ref := func(a uint32) profile.Ref {
		var r profile.Ref
		if err := r.UnmarshalJSON([]byte(fmt.Sprintf("%q", fmt.Sprintf("0x%08X", a)))); err != nil {
			t.Fatal(err)
		}
		return r
	}
	prof := &profile.Profile{Name: "ROWE", PartyAddress: 0x02024284, PartySlots: 6, PokemonSize: 76,
		Flags: ref(flags), Vars: ref(vars), ROWELevelScaling: &profile.ROWELevelScaling{Table: table}}
	s := New(prof, read)

	setVar(0x407A, 2) // difficulty
	setVar(0x4061, 3) // stage
	setCap(2, 2, 3, 25)
	setCap(2, 5, 3, 3)
	check := func(want int, wantOK bool) {
		t.Helper()
		got, ok, err := s.scaledLevel()
		if err != nil || ok != wantOK || (ok && got != want) {
			t.Errorf("scaledLevel = %d, %v, %v; want %d, %v", got, ok, err, want, wantOK)
		}
	}
	check(0, false) // option off
	setFlag(0x2D, true)
	check(28, true)
	setFlag(0xDEE, true) // stage 18: everything at 100
	check(100, true)
	setFlag(0xDEE, false)
	setFlag(0x2D, false)
	setFlag(0xE3A, true)
	check(5, true)
}
