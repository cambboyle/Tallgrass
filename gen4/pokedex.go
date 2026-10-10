package gen4

import (
	"encoding/binary"
	"math/bits"
)

// The Pokédex in the general block starts with the magic number
// 0xBEEFCAFE, then the caught flags (bit = species − 1). The seen flags
// follow somewhere after: straight after in vanilla games, further on in
// hacks with more species (HeartGold Generations: 0x3FC bytes on).
const dexMagic = 0xBEEFCAFE

// Pokedex says where the caught and seen flags are in a general block.
type Pokedex struct {
	Caught, Seen int // offsets in the general block
	Size         int // bytes per flag array
}

// FindPokedex locates the flags. The seen array must contain every caught
// species and look like flags rather than filler. Known gaps are tried
// first (vanilla 0x40, HeartGold Generations 0x3FC), then a search.
func FindPokedex(general []byte, species int) (Pokedex, bool) {
	size := (species + 7) / 8
	size = (size + 3) &^ 3
	for m := 0; m+4 <= len(general); m += 4 {
		if binary.LittleEndian.Uint32(general[m:]) != dexMagic {
			continue
		}
		caught := m + 4
		if caught+size > len(general) {
			return Pokedex{}, false
		}
		c := general[caught : caught+size]
		fits := func(gap int) bool {
			at := caught + gap
			if at+size > len(general) {
				return false
			}
			s := general[at : at+size]
			return superset(s, c) && ones(s) >= ones(c) && !filler(s, c)
		}
		for _, gap := range []int{0x40, 0x3FC} {
			if gap >= size && fits(gap) {
				return Pokedex{Caught: caught, Seen: caught + gap, Size: size}, true
			}
		}
		for gap := size; gap <= 0x800; gap += 4 {
			if fits(gap) {
				return Pokedex{Caught: caught, Seen: caught + gap, Size: size}, true
			}
		}
		return Pokedex{}, false
	}
	return Pokedex{}, false
}

// filler spots padding: a fully set byte where nothing at all is caught.
func filler(seen, caught []byte) bool {
	for i := range seen {
		if seen[i] == 0xFF && caught[i] == 0 {
			return true
		}
	}
	return false
}

func superset(a, b []byte) bool {
	for i := range b {
		if a[i]&b[i] != b[i] {
			return false
		}
	}
	return true
}

func ones(b []byte) int {
	n := 0
	for _, c := range b {
		n += bits.OnesCount8(c)
	}
	return n
}

// Has reports whether a species' flag is set in a flag array.
func Has(flags []byte, species int) bool {
	i := species - 1
	return i >= 0 && i/8 < len(flags) && flags[i/8]&(1<<(i%8)) != 0
}
