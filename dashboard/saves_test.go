package dashboard

import (
	"encoding/binary"
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

	// Uploading an edited save: only a real Gen 3 save, sent as raw bytes,
	// with the game closed; shorter uploads are padded to the file's size.
	path := filepath.Join(roms, "gba", "Game.srm")
	os.WriteFile(path, fakeGen3Save(0x20000), 0o644)
	edited := fakeGen3Save(0x10000 * 2)
	edited[100] = 0xAB
	upload := func(contentType string, data []byte) int {
		req := httptest.NewRequest("POST", "/api/saves/upload?file=gba/Game.srm", strings.NewReader(string(data)))
		req.Header.Set("Content-Type", contentType)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := upload("application/x-www-form-urlencoded", edited); code != http.StatusUnsupportedMediaType {
		t.Errorf("form upload: %d", code)
	}
	if code := upload("application/octet-stream", []byte("not a save")); code != http.StatusBadRequest {
		t.Errorf("garbage upload: %d", code)
	}
	game = true
	s.update(s.Snapshot())
	if code := upload("application/octet-stream", edited); code != http.StatusConflict {
		t.Errorf("upload while playing: %d", code)
	}
	game = false
	s.update(s.Snapshot())
	before, _ := watcher.Files()
	if code := upload("application/octet-stream", edited[:0x1C000]); code != http.StatusNoContent {
		t.Errorf("upload: %d", code)
	}
	got, _ := os.ReadFile(path)
	if len(got) != 0x20000 || got[100] != 0xAB {
		t.Errorf("uploaded save: %d bytes, byte 100 = %X", len(got), got[100])
	}
	if after, _ := watcher.Files(); len(after[0].Versions) != len(before[0].Versions)+1 {
		t.Errorf("the save before the upload wasn't backed up: %d → %d versions", len(before[0].Versions), len(after[0].Versions))
	}
}

// fakeGen3Save is n bytes whose first 28 sectors carry a Gen 3 footer.
func fakeGen3Save(n int) []byte {
	b := make([]byte, n)
	for i := 0; i < 28; i++ {
		binary.LittleEndian.PutUint32(b[i*0x1000+0xFF8:], 0x08012025)
	}
	return b
}
