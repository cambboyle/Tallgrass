package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWithLocal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mine.json"), []byte(`{"name": "My Hack", "partyAddress": "0x02024284", "partySlots": 6, "pokemonSize": 100}`), 0o644)
	os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{"name": `), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o644)
	bundled, _ := All()
	all, errs := WithLocal(bundled, dir)
	if len(errs) != 1 || len(all) != len(bundled)+1 || all[0].Name != "My Hack" {
		t.Errorf("all[0] = %v, %d profiles, errors %v", all[0].Name, len(all), errs)
	}
	if same, errs := WithLocal(bundled, filepath.Join(dir, "missing")); len(same) != len(bundled) || errs != nil {
		t.Errorf("missing folder: %d, %v", len(same), errs)
	}
}

func TestSupports(t *testing.T) {
	p := &Profile{MinTallgrass: "0.1.13"}
	for v, want := range map[string]bool{"0.1.13": true, "0.1.20": true, "0.2.0": true, "0.1.9": false, "0.1.12": false, "dev-abc": true, "": true} {
		if got := p.Supports(v); got != want {
			t.Errorf("Supports(%q) = %v", v, got)
		}
	}
	if !(&Profile{}).Supports("0.1.0") {
		t.Error("a profile without minTallgrass works everywhere")
	}
}
