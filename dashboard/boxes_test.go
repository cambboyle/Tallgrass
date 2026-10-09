package dashboard

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tallgrass/gen3"
	"tallgrass/profile"
	"tallgrass/saves"
)

// Live boxes from fake RAM built out of a real Radical Red save, with the
// ROM found by hash in a temporary ROMs folder.
func TestLiveBoxes(t *testing.T) {
	romPath, savePath := os.Getenv("TALLGRASS_ROM"), os.Getenv("TALLGRASS_SAVE")
	if romPath == "" || savePath == "" {
		t.Skip("set TALLGRASS_ROM and TALLGRASS_SAVE to run")
	}
	roms := filepath.Join(t.TempDir(), "roms")
	os.MkdirAll(filepath.Join(roms, "gba"), 0o755)
	if err := os.Symlink(romPath, filepath.Join(roms, "gba", "Some Name.gba")); err != nil {
		t.Fatal(err)
	}

	prof, _ := profile.Load("radical-red")
	all, _ := profile.All()
	data, _ := os.ReadFile(savePath)
	save, err := gen3.ParseSave(data, gen3.SaveLayout{SectorDataSize: 0xFF0})
	if err != nil {
		t.Fatal(err)
	}
	ptr := func(a uint32) []byte { return binary.LittleEndian.AppendUint32(nil, a) }
	mem := map[uint32][]byte{
		0x02024284: save.Block1[0x38 : 0x38+600], // party, so the poller sees a game
		0x03005008: ptr(0x02025000), 0x02025000: save.Block1,
		0x0300500C: ptr(0x02024000), 0x02024000: save.Block2,
		0x03005010: ptr(0x02029000), 0x02029000: save.Storage,
	}
	read := func(addr uint32, n int) ([]byte, error) {
		for base, b := range mem {
			if addr >= base && int(addr-base)+n <= len(b) {
				return b[addr-base : int(addr-base)+n], nil
			}
		}
		return nil, fmt.Errorf("unmapped %08X+%d", addr, n)
	}

	s := New(prof, read)
	s.EnableSaves(saves.NewWatcher([]saves.Source{{System: "gba", SaveDir: filepath.Join(roms, "gba"), ROMDir: filepath.Join(roms, "gba")}}, saves.NewBackups(t.TempDir())), all)
	s.update(s.Snapshot())

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/boxes", nil))
	var v SaveView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if v.Version != "live" || v.Trainer.Name != "Cameron" || len(v.Party) == 0 || v.Party[0].SpeciesName != "Cyndaquil" ||
		len(v.Boxes) != 19 || v.Boxes[0].Name != "Box1" || v.Boxes[18].Name != "Box19" || v.HiddenBoxes != 6 {
		t.Errorf("view = %+v", v)
	}
}

// The game data export from a real ROM (set TALLGRASS_ROM to Radical
// Red): every species and trainer, with moves and places.
func TestExportRadicalRed(t *testing.T) {
	path := os.Getenv("TALLGRASS_ROM")
	if path == "" {
		t.Skip("set TALLGRASS_ROM to the Radical Red ROM")
	}
	all, err := profile.All()
	if err != nil {
		t.Fatal(err)
	}
	gd, err := New(all[0], nil).ExportFile(path, all)
	if err != nil {
		t.Fatal(err)
	}
	if gd.Version != ExportVersion || len(gd.Species) < 1000 || len(gd.Trainers) < 700 {
		t.Fatalf("version %d, %d species, %d trainers", gd.Version, len(gd.Species), len(gd.Trainers))
	}
	var brock bool
	for _, tr := range gd.Trainers {
		if tr.Name == "Brock" && len(tr.Areas) > 0 && len(tr.Party) > 0 && len(tr.Party[0].Moves) > 0 {
			brock = true
		}
	}
	if !brock {
		t.Error("no placed Brock with a custom-move team")
	}
	var viridian *MapView
	items, shops := 0, 0
	for _, m := range gd.Maps {
		items += len(m.Items)
		shops += len(m.Shops)
		if m.ID == "3.1" {
			viridian = m
		}
	}
	if viridian == nil || viridian.Area != "Viridian City" || len(viridian.Connections) != 3 || items < 200 || shops < 20 {
		t.Errorf("Viridian City %+v; %d items, %d shops", viridian, items, shops)
	}
	for _, sp := range gd.Species {
		if sp.Name == "Charmander" && (len(sp.TMMoves) == 0 || len(sp.EggMoves) == 0 || len(sp.FoundIn) == 0) {
			t.Errorf("Charmander: %d TMs, %d egg moves, found in %v", len(sp.TMMoves), len(sp.EggMoves), sp.FoundIn)
		}
	}
}
