package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tallgrass/saves"
)

func TestSavesAPI(t *testing.T) {
	roms := t.TempDir()
	os.MkdirAll(filepath.Join(roms, "gba"), 0o755)
	os.WriteFile(filepath.Join(roms, "gba", "Game.srm"), []byte("save data"), 0o644)
	watcher := saves.NewWatcher([]saves.Source{{System: "gba", SaveDir: filepath.Join(roms, "gba"), ROMDir: filepath.Join(roms, "gba")}}, saves.NewBackups(t.TempDir()))
	watcher.Scan()
	watcher.Scan()

	game := false
	s := New(testProfile, func(uint32, int) ([]byte, error) {
		if game {
			return memory(6)(0x02024284, 600)
		}
		return nil, errors.New("no game")
	})
	s.EnableSaves(watcher, nil)
	h := s.Handler()
	do := func(method, url, contentType, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	var list struct {
		Files []saves.SaveFile
	}
	json.Unmarshal(do("GET", "/api/saves", "", "").Body.Bytes(), &list)
	if len(list.Files) != 1 || len(list.Files[0].Versions) != 1 {
		t.Fatalf("list = %+v", list)
	}
	id := list.Files[0].Versions[0].ID

	if rec := do("GET", "/api/saves/download?file=gba/Game.srm&version="+id, "", ""); rec.Body.String() != "save data" ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "Game 2") {
		t.Errorf("download = %d %q %q", rec.Code, rec.Body, rec.Header().Get("Content-Disposition"))
	}
	if rec := do("GET", "/api/saves/download?file=gba/Nope.srm&version="+id, "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown file: %d", rec.Code)
	}

	body := `{"file":"gba/Game.srm","version":"` + id + `"}`
	if rec := do("POST", "/api/saves/restore", "application/x-www-form-urlencoded", body); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("form post: %d", rec.Code)
	}

	game = true
	s.update(s.Snapshot())
	if rec := do("POST", "/api/saves/restore", "application/json", body); rec.Code != http.StatusConflict {
		t.Errorf("restore while playing: %d", rec.Code)
	}

	game = false
	s.update(s.Snapshot())
	if rec := do("POST", "/api/saves/restore", "application/json", body); rec.Code != http.StatusNoContent {
		t.Errorf("restore: %d %s", rec.Code, rec.Body)
	}
}
