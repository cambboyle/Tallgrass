package dashboard

import (
	"os"
	"testing"

	"tallgrass/gen3"
	"tallgrass/profile"
)

// Emerald's profile references against the real ROM.
func TestEmeraldROM(t *testing.T) {
	path := os.Getenv("TALLGRASS_EMERALD_ROM")
	if path == "" {
		t.Skip("set TALLGRASS_EMERALD_ROM to run")
	}
	rom, err := gen3.OpenROM(path)
	if err != nil {
		t.Fatal(err)
	}
	defer rom.Close()
	read := rom.Read
	p, err := profile.Load("emerald")
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(r profile.Ref) uint32 {
		a, err := r.Resolve(read)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}

	names := gen3.NewNames(read, nameTables(p))
	table := resolve(p.Location.MapNames)
	stride, offset := p.Location.MapNameTable()
	for section, want := range map[int]string{0: "Littleroot Town", 1: "Oldale Town", 16: "Route 101"} {
		if got, err := names.MapName(table, stride, offset, p.Location.FirstMapSection, section); got != want {
			t.Errorf("map section %d = %q (%v), want %q", section, got, err, want)
		}
	}
	if n, _ := gen3.NewNational(read, resolve(p.Pokedex.SpeciesToNational)).Number(277); n != 252 {
		t.Errorf("Treecko (277) national = %d, want 252", n)
	}
	levels := gen3.NewLevels(read, resolve(p.Experience.BaseStats), resolve(p.Experience.Table), p.Experience.RowLength)
	if lv, _ := levels.Level(277, 135); lv != 5 { // Treecko is Medium Slow: 135 exp = Lv 5
		t.Errorf("Treecko with 135 exp: Lv %d", lv)
	}

	wild := gen3.NewWild(read, resolve(p.WildEncounters.Headers), nil)
	got, err := wild.ForMap(0, 16) // Route 101
	if err != nil || len(got) != 1 {
		t.Fatalf("Route 101: %+v %v", got, err)
	}
	var route []string
	for _, e := range got[0].Methods[0].Encounters {
		n, _ := names.Species(e.Species)
		route = append(route, n)
	}
	t.Logf("Route 101: %v", route)
	if len(route) != 3 || route[0] != "Wurmple" && route[0] != "Poochyena" && route[0] != "Zigzagoon" {
		t.Errorf("Route 101 encounters = %v", route)
	}
	if mv, _ := names.Move(1); mv != "Pound" {
		t.Errorf("move 1 = %q", mv)
	}
	if img, err := gen3.FrontSprite(read, 277, false); err != nil || img.Bounds().Dx() != 64 {
		t.Errorf("Treecko sprite: %v", err)
	}
}
