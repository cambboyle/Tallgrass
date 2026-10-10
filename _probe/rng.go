package main

import "fmt"

// rng finds u32 words in [lo, hi) in every binary.
func rng(bins map[string][]byte, lo, hi uint32) {
	for name, b := range bins {
		for p := 0; p+4 <= len(b); p += 4 {
			if v := le.Uint32(b[p:]); v >= lo && v < hi {
				fmt.Printf("%s +%X: %08X\n", name, p, v)
			}
		}
	}
}
