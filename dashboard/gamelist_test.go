package dashboard

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tallgrass/profile"
	"tallgrass/saves"
)

func TestGamelistAPI(t *testing.T) {
	roms := t.TempDir()
	gba := filepath.Join(roms, "gba")
	os.MkdirAll(gba, 0o755)
	os.WriteFile(filepath.Join(gba, "1636 - hack (patched).gba"), []byte("a known rom"), 0o644)
	os.WriteFile(filepath.Join(gba, "Unknown.gba"), []byte("something else"), 0o644)
	sum := sha1.Sum([]byte("a known rom"))
	prof := *testProfile
	prof.Name, prof.ROMSHA1, prof.SaveFile = "Pokémon Known Hack", []string{hex.EncodeToString(sum[:])}, &profile.SaveFile{}

	s := New(testProfile, func(uint32, int) ([]byte, error) { return nil, errors.New("no game") })
	s.EnableSaves(saves.NewWatcher([]saves.Source{{System: "gba", SaveDir: gba, ROMDir: gba}}, saves.NewBackups(t.TempDir())), []*profile.Profile{&prof})
	lists := t.TempDir()
	s.EnableGamelists(lists)
	for _, r := range s.saves.ROMs() {
		s.hashROM(r.Path)
	}
	h := s.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/library/gamelist", nil))
	var v struct{ Games []GamelistGame }
	json.Unmarshal(rec.Body.Bytes(), &v)
	if len(v.Games) != 1 || v.Games[0].Name != "Pokémon Known Hack" || v.Games[0].File != "1636 - hack (patched).gba" {
		t.Fatalf("preview = %s", rec.Body)
	}

	req := httptest.NewRequest("POST", "/api/library/gamelist", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	if !s.gamelistWanted() {
		t.Error("asking for names should switch them on")
	}
	b, _ := os.ReadFile(filepath.Join(lists, "gba", "gamelist.xml"))
	if !strings.Contains(string(b), "<name>Pokémon Known Hack</name>") || strings.Contains(string(b), "Unknown") {
		t.Errorf("gamelist =\n%s", b)
	}
}
