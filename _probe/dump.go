package main

import (
	"fmt"
	"strconv"

	"tallgrass/gen4"
)

// dump prints u16 words from arm9 [from, to) with item names.
func dump(a9 []byte, d *gen4.Data, from, to string) {
	f, _ := strconv.ParseUint(from, 16, 32)
	t, _ := strconv.ParseUint(to, 16, 32)
	line, start := "", int(f)
	for p := int(f); p < int(t); p += 2 {
		v := int(le.Uint16(a9[p:]))
		if v == 0xFFFF {
			fmt.Printf("+%X: %s END\n", start, line)
			line, start = "", p+2
			continue
		}
		line += fmt.Sprintf("%d:%s ", v, gen4.Name(d.Items, v))
	}
	fmt.Println(line)
}
