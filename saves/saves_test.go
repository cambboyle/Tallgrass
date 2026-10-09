package saves

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupsDedupeAndRead(t *testing.T) {
	b := NewBackups(t.TempDir())
	t0 := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)

	for i, tc := range []struct {
		data  string
		saved bool
	}{{"one", true}, {"one", false}, {"two", true}, {"one", true}} {
		saved, err := b.Save("gba/Game.srm", []byte(tc.data), t0.Add(time.Duration(i)*time.Minute))
		if err != nil || saved != tc.saved {
			t.Fatalf("save %d (%s): saved=%v err=%v", i, tc.data, saved, err)
		}
	}

	versions, _ := b.Versions("gba/Game.srm")
	if len(versions) != 3 || !versions[0].At.Equal(t0.Add(3*time.Minute)) {
		t.Fatalf("versions = %+v", versions)
	}
	data, err := b.Read("gba/Game.srm", versions[1].ID)
	if err != nil || string(data) != "two" {
		t.Errorf("read = %q, %v", data, err)
	}
	for _, bad := range []string{"../x.gz", "nope", versions[1].ID + "/../x"} {
		if _, err := b.Read("gba/Game.srm", bad); err == nil {
			t.Errorf("read %q should fail", bad)
		}
	}
}

// writeSave writes a save file with an explicit modification time.
func writeSave(t *testing.T, root, rel, data string, mod time.Time) {
	t.Helper()
	p := filepath.Join(root, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(p, mod, mod)
}

func TestWatcher(t *testing.T) {
	root := t.TempDir()
	w := NewWatcher([]Source{
		{System: "gba", SaveDir: filepath.Join(root, "gba"), ROMDir: filepath.Join(root, "gba")},
		{System: "gb", SaveDir: filepath.Join(root, "gb"), ROMDir: filepath.Join(root, "gb")},
	}, NewBackups(t.TempDir()))
	t0 := time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)
	writeSave(t, root, "gba/Game.srm", "day 1", t0)
	writeSave(t, root, "gba/Game.gba", "rom", t0)     // not a save
	writeSave(t, root, "snes/Other.srm", "other", t0) // not a watched system

	count := func() int {
		files, err := w.Files()
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Fatalf("files = %+v", files)
		}
		return len(files[0].Versions)
	}

	w.Scan()
	if count() != 0 {
		t.Error("first sighting should wait for the file to settle")
	}
	w.Scan()
	w.Scan()
	if count() != 1 {
		t.Errorf("after settling: %d versions", count())
	}

	writeSave(t, root, "gba/Game.srm", "day 2", t0.Add(time.Hour))
	w.Scan()
	w.Scan()
	if count() != 2 {
		t.Errorf("after a change: %d versions", count())
	}

	// A fresh watcher (restart) doesn't duplicate the latest version.
	w2 := NewWatcher([]Source{{System: "gba", SaveDir: filepath.Join(root, "gba"), ROMDir: filepath.Join(root, "gba")}}, w.backups)
	w2.Scan()
	w2.Scan()
	if count() != 2 {
		t.Errorf("after restart: %d versions", count())
	}

	files, _ := w.Files()
	oldest := files[0].Versions[1]
	if err := w.Restore("gba/Game.srm", oldest.ID, true); !errors.Is(err, ErrGameRunning) {
		t.Errorf("restore while running: %v", err)
	}
	if err := w.Restore("gba/../../etc/passwd", oldest.ID, false); err == nil {
		t.Error("restore of unknown path should fail")
	}
	if err := w.Restore("gba/Game.srm", oldest.ID, false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "gba/Game.srm"))
	if !bytes.Equal(got, []byte("day 1")) {
		t.Errorf("restored contents = %q", got)
	}
	w.Scan()
	w.Scan()
	// day 1, day 2, then the restored day 1 as the newest version.
	files, _ = w.Files()
	if v := files[0].Versions; len(v) != 3 {
		t.Errorf("versions after restore = %+v", v)
	}
}

// DraStic keeps saves away from the ROMs.
func TestDSSaves(t *testing.T) {
	roms, drastic := t.TempDir(), t.TempDir()
	w := NewWatcher(RetroPieSources(roms, drastic), NewBackups(t.TempDir()))
	t0 := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	writeSave(t, drastic, "HeartGold.dsv", "ds save", t0)
	writeSave(t, roms, "nds/HeartGold.nds", "rom", t0)
	writeSave(t, roms, "gba/Red.srm", "gba save", t0)
	writeSave(t, roms, "gba/Red.gba", "rom", t0)
	w.Scan()
	w.Scan()

	files, _ := w.Files()
	if len(files) != 2 {
		t.Fatalf("files = %+v", files)
	}
	for _, f := range files {
		if len(f.Versions) != 1 {
			t.Errorf("%s has %d versions", f.Path, len(f.Versions))
		}
	}
	if rom, err := w.ROMFor("nds/HeartGold.dsv"); err != nil || rom != filepath.Join(roms, "nds", "HeartGold.nds") {
		t.Errorf("ROMFor = %q, %v", rom, err)
	}
	if got := w.SaveFor(filepath.Join(roms, "nds", "HeartGold.nds")); len(got) != 1 || got[0] != "nds/HeartGold.dsv" {
		t.Errorf("SaveFor = %v", got)
	}
	if roms := w.ROMs(); len(roms) != 2 {
		t.Errorf("ROMs = %v", roms)
	}
}
