package main

import (
	"fmt"
	"strings"

	"tallgrass/gen4"
)

// lists finds 0xFFFF-terminated u16 item lists containing Poké Ball and Potion.
func lists(bins map[string][]byte, d *gen4.Data) {
	for name, b := range bins {
		start := 0
		for p := 0; p+2 <= len(b); p += 2 {
			v := le.Uint16(b[p:])
			if v == 0xFFFF {
				n := (p - start) / 2
				if n >= 2 && n <= 40 && start%2 == 0 {
					var names []string
					ball, pot := false, false
					for q := start; q < p; q += 2 {
						it := int(le.Uint16(b[q:]))
						ball = ball || it == 4
						pot = pot || it == 17
						names = append(names, gen4.Name(d.Items, it))
					}
					if ball || pot {
						fmt.Printf("%s +%X (%d): %s\n", name, start, n, strings.Join(names, ", "))
					}
				}
				start = p + 2
				continue
			}
			if v == 0 || int(v) >= len(d.Items) {
				start = p + 2
			}
		}
	}
}
