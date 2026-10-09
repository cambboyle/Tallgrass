package gen3

import "testing"

func TestDecompressLZ77(t *testing.T) {
	// "AB" as literals, then a back-reference (distance 2, length 4) that
	// overlaps its own output: ABABAB.
	src := []byte{0x10, 6, 0, 0, 0b00100000, 'A', 'B', 0x10, 0x01}
	out, err := decompressLZ77(src)
	if err != nil || string(out) != "ABABAB" {
		t.Errorf("got %q, %v", out, err)
	}

	if _, err := decompressLZ77(src[:6]); err == nil {
		t.Error("expected error for truncated data")
	}
}

func TestShiny(t *testing.T) {
	b := make([]byte, PartySize)
	b[32] = 1 // species
	// Personality 0x00010000, OT ID 0x00000001: halves XOR to 0 ^ 1 ^ 1 ^ 0 = 0.
	b[2], b[4] = 1, 1
	p, _ := Decode(b, false)
	if !p.IsShiny {
		t.Error("expected shiny")
	}
}
