package main

import "fmt"

// ptrs finds u32 references to a RAM address in arm9.
func ptrs(a9 []byte, base uint32, target uint32) {
	for p := 0; p+4 <= len(a9); p += 4 {
		if le.Uint32(a9[p:]) == target {
			fmt.Printf("ref to %08X at +%X\n", target, p)
			for q := max(0, p-0x40); q < min(len(a9)-4, p+0x80); q += 4 {
				fmt.Printf("  +%X %08X\n", q, le.Uint32(a9[q:]))
			}
		}
	}
}
