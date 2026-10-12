package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"tallgrass/cloud"
	"tallgrass/saves"
)

// Cloud sync uploads each backup version once (and the Journey when it
// changes), only when switched on and the device is linked.
func TestCloudSync(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/functions/v1/backup-upload" || r.Header.Get("Authorization") != "Bearer device-key" {
			http.Error(w, `{"error":"no"}`, 401)
			return
		}
		mu.Lock()
		got = append(got, r.Header.Get("X-Kind")+" "+r.Header.Get("X-File")+" "+r.Header.Get("X-Version"))
		mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	gba := filepath.Join(dir, "gba")
	os.MkdirAll(gba, 0o755)
	os.WriteFile(filepath.Join(gba, "Game.srm"), []byte("save one"), 0o644)
	w := saves.NewWatcher([]saves.Source{{System: "gba", SaveDir: gba, ROMDir: gba}}, saves.NewBackups(t.TempDir()))
	w.Scan()
	w.Scan()

	s := New(testProfile, func(uint32, int) ([]byte, error) { return nil, errors.New("no game") })
	s.EnableSaves(w, nil)
	link := filepath.Join(dir, "link.json")
	s.EnableCloud(cloud.New(srv.URL, "anon"), link, "https://example.com")
	s.EnableCloudSync(filepath.Join(dir, "sync.json"))
	defer s.cloud.sync.started.Wait() // the run that switching it on starts writes into dir

	s.syncOnce() // off: nothing
	if len(got) != 0 {
		t.Fatalf("uploaded while off: %v", got)
	}
	req := httptest.NewRequest("POST", "/api/cloud/sync", strings.NewReader(`{"enabled": true}`))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(httptest.NewRecorder(), req)
	s.syncOnce() // on but not linked: nothing
	b, _ := json.Marshal(cloud.Link{Token: "device-key"})
	os.WriteFile(link, b, 0o600)
	s.syncOnce()
	s.syncOnce() // nothing new
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || !strings.HasPrefix(got[0], "save gba/Game.srm ") || strings.HasSuffix(got[0], ".gz") {
		t.Errorf("uploads = %q", got)
	}
}
