package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strconv"

	"tallgrass/gen4"
)

var le = binary.LittleEndian

func main() {
	path := os.Args[1]
	d, err := gen4.LoadData(path)
	if err != nil {
		panic(err)
	}
	r, _ := gen4.OpenROM(path)
	a9, _ := r.ARM9()
	ovs, _ := r.Overlays()
	bins := map[string][]byte{"arm9": a9.Data}
	for i, o := range ovs {
		bins[fmt.Sprintf("ov%d", i)] = o
	}
	for i, o := range ovs {
		bins[fmt.Sprintf("ov%d", i)] = o
	}
	if len(os.Args) > 3 && os.Args[2] == "a9" {
		writeA9(a9.Data, os.Args[3])
		return
	}
	if len(os.Args) > 4 && os.Args[2] == "rng" {
		lo, _ := strconv.ParseUint(os.Args[3], 16, 32)
		hi, _ := strconv.ParseUint(os.Args[4], 16, 32)
		rng(bins, uint32(lo), uint32(hi))
		return
	}
	if len(os.Args) > 4 && os.Args[2] == "raw" {
		raw(a9.Data, os.Args[3], os.Args[4])
		return
	}
	if len(os.Args) > 3 && os.Args[2] == "ptr" {
		t, _ := strconv.ParseUint(os.Args[3], 16, 32)
		for name, b := range bins {
			fmt.Println("==", name)
			ptrs(b, 0, uint32(t))
		}
		return
	}
	if len(os.Args) > 4 {
		dump(a9.Data, d, os.Args[3], os.Args[4])
		return
	}
	if len(os.Args) > 2 {
		lists(bins, d)
		return
	}
	mart := map[int]bool{4: true, 17: true, 18: true, 19: true, 20: true, 21: true, 22: true, 26: true, 78: true, 79: true, 3: true, 2: true}
	for name, b := range bins {
		for off := 0; off+4*10 <= len(b); off += 2 {
			n, hits := 0, 0
			seen := map[int]bool{}
			for p := off; p+4 <= len(b); p += 4 {
				it, bd := int(le.Uint16(b[p:])), le.Uint16(b[p+2:])
				if it == 0 || it > 1000 || bd > 8 {
					break
				}
				if mart[it] && !seen[it] {
					seen[it] = true
					hits++
				}
				n++
			}
			if n >= 10 && hits >= 6 {
				fmt.Printf("%s +%X: %d pairs\n", name, off, n)
				for p := off; p < off+4*n; p += 4 {
					fmt.Printf("   %s (%d badges)\n", gen4.Name(d.Items, int(le.Uint16(b[p:]))), le.Uint16(b[p+2:]))
				}
				off += 4 * n
			}
		}
	}
}
