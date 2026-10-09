package dashboard

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tallgrass/gen3"
	"tallgrass/profile"
	"tallgrass/saves"
)

// The real Unbound save through the save viewer, with its ROM found by hash.
func TestUnboundSave(t *testing.T) {
	romPath, savePath := os.Getenv("TALLGRASS_UNBOUND_ROM"), os.Getenv("TALLGRASS_UNBOUND_SAVE")
	if romPath == "" || savePath == "" {
		t.Skip("set TALLGRASS_UNBOUND_ROM and TALLGRASS_UNBOUND_SAVE to run")
	}
	roms := t.TempDir()
	os.MkdirAll(filepath.Join(roms, "gba"), 0o755)
	os.Symlink(romPath, filepath.Join(roms, "gba", "Unbound.gba"))
	data, _ := os.ReadFile(savePath)
	os.WriteFile(filepath.Join(roms, "gba", "Unbound.sav"), data, 0o644)

	all, err := profile.All()
	if err != nil {
		t.Fatal(err)
	}
	s := New(all[0], memory(6))
	w := saves.NewWatcher([]saves.Source{{System: "gba", SaveDir: filepath.Join(roms, "gba"), ROMDir: filepath.Join(roms, "gba")}},
		saves.NewBackups(t.TempDir()))
	w.Scan()
	s.EnableSaves(w, all)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/saves/view?file=gba/Unbound.sav", nil))
	var v SaveView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if v.Game != "Pokémon Unbound" || v.Trainer.Name != "Cameron" || len(v.Party) != 6 {
		t.Fatalf("view: %s, %q, %d in party", v.Game, v.Trainer.Name, len(v.Party))
	}
	lead := v.Party[0]
	t.Logf("party 1: %s Lv %d, met %q, moves %v; box 1: %d Pokémon; dex %d/%d", lead.SpeciesName, lead.Level, lead.MetArea,
		lead.MoveNames, len(v.Boxes[0].Mons), v.Pokedex.Caught, v.Pokedex.Seen)
	if lead.SpeciesName != "Gabite" || lead.Level != 24 || lead.MetArea == "" || lead.MoveNames[0] == "" {
		t.Errorf("lead = %+v", lead)
	}
	if b := v.Boxes[0].Mons[0]; b == nil || b.SpeciesName != "Vanillite" || b.Level < 2 {
		t.Errorf("box 1 slot 1 = %+v", b)
	}
	if len(v.Boxes) != 19 || v.HiddenBoxes != 6 || v.Boxes[14].Name != "Box15" {
		t.Errorf("%d boxes, %d hidden, box 15 named %q", len(v.Boxes), v.HiddenBoxes, v.Boxes[14].Name)
	}
	if v.Pokedex.Caught != 20 || v.Pokedex.Seen != 69 {
		t.Errorf("dex %d caught / %d seen", v.Pokedex.Caught, v.Pokedex.Seen)
	}

	// Encounter tables through the profile's day/night periods.
	ub, _ := profile.Load("unbound")
	rom, _ := gen3.OpenROM(romPath)
	defer rom.Close()
	mainTable, _ := ub.WildEncounters.Headers.Resolve(rom.Read)
	var periods []gen3.Period
	for _, p := range ub.WildEncounters.Periods {
		addr, _ := p.Headers.Resolve(rom.Read)
		periods = append(periods, gen3.Period{Name: p.Name, Headers: addr, FromHour: p.FromHour, ToHour: p.ToHour})
	}
	got, err := gen3.NewWild(rom.Read, mainTable, periods).ForMap(3, 69)
	if err != nil || len(got) != 3 {
		t.Fatalf("map 3.69 periods: %d, %v", len(got), err)
	}
	names := gen3.NewNames(rom.Read, gen3.NameTables{})
	for _, p := range got {
		var list []string
		for _, e := range p.Methods[0].Encounters {
			n, _ := names.Species(e.Species)
			list = append(list, n)
		}
		t.Logf("map 3.69 %s / %s: %v", p.Name, p.Methods[0].Method, list)
	}
}
