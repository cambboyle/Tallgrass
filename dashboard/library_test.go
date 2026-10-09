package dashboard

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tallgrass/profile"
	"tallgrass/saves"
)

func TestLibrary(t *testing.T) {
	roms, drastic := t.TempDir(), t.TempDir()
	write := func(path, data string) {
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(data), 0o644)
	}
	write(filepath.Join(roms, "gba", "Red.gba"), "a gba rom")
	write(filepath.Join(roms, "gba", "Red.srm"), "save")
	write(filepath.Join(roms, "nds", "Gold.nds"), "a ds rom")
	write(filepath.Join(drastic, "Gold.dsv"), "ds save")

	sum := sha1.Sum([]byte("a gba rom"))
	prof := *testProfile
	prof.Name, prof.ROMSHA1, prof.SaveFile = "Red Hack", []string{hex.EncodeToString(sum[:])}, &profile.SaveFile{}
	cache := filepath.Join(t.TempDir(), "hashes.json")

	newServer := func() *Server {
		s := New(testProfile, memory(6))
		w := saves.NewWatcher(saves.RetroPieSources(roms, drastic), saves.NewBackups(t.TempDir()))
		w.Scan()
		s.EnableSaves(w, []*profile.Profile{&prof})
		s.EnableHashCache(cache)
		return s
	}
	type library struct {
		ROMs    []LibraryROM
		Hashing bool
	}
	get := func(s *Server) library {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/library", nil))
		var l library
		json.Unmarshal(rec.Body.Bytes(), &l)
		return l
	}

	s := newServer()
	if l := get(s); !l.Hashing || len(l.ROMs) != 2 || l.ROMs[0].SHA1 != "" {
		t.Fatalf("first look: %+v", l)
	}
	var l library
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if l = get(s); !l.Hashing {
			break
		}
	}
	byFile := map[string]LibraryROM{}
	for _, r := range l.ROMs {
		byFile[r.File] = r
	}
	red, gold := byFile["Red.gba"], byFile["Gold.nds"]
	if red.Profile != "Red Hack" || !red.Viewable || len(red.Saves) != 1 || red.System != "gba" {
		t.Errorf("Red = %+v", red)
	}
	if gold.Profile != "" || gold.SHA1 == "" || len(gold.Saves) != 1 || gold.Saves[0].Path != "nds/Gold.dsv" {
		t.Errorf("Gold = %+v", gold)
	}

	// A restart finds the hashes in the cache file.
	if l := get(newServer()); l.Hashing {
		t.Error("hashes should have been loaded from the cache file")
	}
}
