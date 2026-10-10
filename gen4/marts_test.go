package gen4

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// Shops in every HeartGold/SoulSilver-family ROM in TALLGRASS_DS: the
// standard Poké Mart sells Poké Balls from the start, some map has one,
// and Goldenrod Dept. Store's 4th floor sells Protein.
func TestROMMarts(t *testing.T) {
	dir := os.Getenv("TALLGRASS_DS")
	if dir == "" {
		t.Skip("set TALLGRASS_DS to a folder of DS ROMs")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".nds") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			d, err := LoadData(dir + "/" + e.Name())
			if err != nil || d.Layout.Family != "HeartGold/SoulSilver" {
				t.Skip("not HeartGold/SoulSilver")
			}
			x, err := d.extras()
			if err != nil {
				t.Fatal(err)
			}
			m := findMarts(x.arm9, d.Layout.Family, len(d.Items))
			if m == nil || len(m.Standard) == 0 || len(m.Special) == 0 {
				t.Fatalf("marts = %+v", m)
			}
			if i := slices.IndexFunc(m.Standard, func(it MartItem) bool { return Name(d.Items, it.Item) == "Poké Ball" }); i < 0 || m.Standard[i].Badges != 0 {
				t.Errorf("standard mart = %v", m.Standard)
			}
			maps, err := d.Maps()
			if err != nil {
				t.Fatal(err)
			}
			standard, protein := 0, false
			for _, mp := range maps {
				for _, s := range mp.Shops {
					if s.Badges != nil {
						standard++
					}
					for _, it := range s.Items {
						protein = protein || Name(d.Items, it) == "Protein"
					}
				}
			}
			if standard < 5 || !protein {
				t.Errorf("%d maps with a standard mart; Protein sold: %v", standard, protein)
			}
		})
	}
}
