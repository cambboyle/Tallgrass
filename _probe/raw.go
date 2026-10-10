package main

import (
	"fmt"
	"strconv"
)

func raw(b []byte, from, to string) {
	f, _ := strconv.ParseUint(from, 16, 32)
	t, _ := strconv.ParseUint(to, 16, 32)
	for p := int(f); p < int(t); p += 16 {
		fmt.Printf("+%X:", p)
		for q := p; q < p+16; q += 2 {
			fmt.Printf(" %5d", le.Uint16(b[q:]))
		}
		fmt.Println()
	}
}
