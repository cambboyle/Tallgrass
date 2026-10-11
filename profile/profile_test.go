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
