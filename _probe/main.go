package main

import (
	"fmt"
	"os"

	"tallgrass/gen3"
)

func main() {
	b, _ := os.ReadFile(os.Args[1])
	for _, size := range []int{0xFF4} {
		s, err := gen3.ParseSave(b, gen3.SaveLayout{SectorDataSize: size, PartyOffset: 0x238, PartyFormat: "rowe"})
		if err != nil {
			fmt.Println(size, err)
			continue
		}
		fmt.Printf("size %X: counter %d\n", size, s.Counter)
		for _, box := range []int{0, 20} {
			mons, err := s.Box(box, gen3.BoxFormat{Size: gen3.ROWEBoxSize})
			for i, m := range mons {
				if m != nil {
					fmt.Printf("  box %d slot %d: %s species %d (%v)\n", box+1, i+1, m.Nickname, m.Species, err)
				}
			}
			fmt.Println("  name", s.BoxNameInOrder(box, 0x7FFC))
		}
		party, _ := s.Party(false)
		for _, p := range party {
			fmt.Println("  party", p.Nickname, p.Level)
		}
		fmt.Printf("  caught bytes % X\n", s.Block1[0x33DC:0x33DC+18])
	}
}
