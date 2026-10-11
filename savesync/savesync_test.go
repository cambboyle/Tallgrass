package savesync

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDevice is a dashboard with one save, whose versions are its history.
type fakeDevice struct {
	mu       sync.Mutex
	versions [][]byte // newest last
	inUse    bool
	refuse   bool
	pin      string
	uploads  int
}

func (d *fakeDevice) set(b string) {
	d.mu.Lock()
	d.versions = append(d.versions, []byte(b))
	d.mu.Unlock()
}

func (d *fakeDevice) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if r.URL.Path == "/api/auth/login" {
		var req struct{ PIN string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.PIN != d.pin {
			http.Error(w, "Wrong PIN", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "tallgrass_session", Value: "token"})
		return
	}
	if d.pin != "" {
		if c, err := r.Cookie("tallgrass_session"); err != nil || c.Value != "token" {
			http.Error(w, "sign in", http.StatusUnauthorized)
			return
		}
	}
	id := func(i int) string { return fmt.Sprintf("v%d", i) }
	switch r.URL.Path {
	case "/api/saves":
		type v struct{ ID, Hash string }
		vs := []v{}
		for i := len(d.versions) - 1; i >= 0; i-- {
			vs = append(vs, v{id(i), hashOf(d.versions[i])})
		}
		json.NewEncoder(w).Encode(map[string]any{"files": []any{
			map[string]any{"path": "gba/Pokémon Game.srm", "inUse": d.inUse, "versions": vs},
			map[string]any{"path": "../escape.srm", "versions": vs},
		}})
	case "/api/saves/download":
		for i, b := range d.versions {
			if id(i) == r.URL.Query().Get("version") && r.URL.Query().Get("file") == "gba/Pokémon Game.srm" {
				w.Write(b)
				return
			}
		}
		http.NotFound(w, r)
	case "/api/saves/upload":
		if d.refuse {
			http.Error(w, "not a Gen 3 save", http.StatusBadRequest)
			return
		}
		b, _ := io.ReadAll(r.Body)
		d.uploads++
		d.versions = append(d.versions, b)
		w.WriteHeader(http.StatusNoContent) // as the real dashboard does
	}
}

func TestSync(t *testing.T) {
	dev := &fakeDevice{}
	dev.set("one")
	srv := httptest.NewServer(dev)
	defer srv.Close()
	dir := t.TempDir()
	s, err := New(srv.URL, dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Logf = t.Logf
	s.Now = func() time.Time { return time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC) }
	local := filepath.Join(dir, "Pokémon Game.sav")
	read := func() string { b, _ := os.ReadFile(local); return string(b) }
	once := func() {
		t.Helper()
		if err := s.Once(); err != nil {
			t.Fatal(err)
		}
	}

	once()
	if read() != "one" {
		t.Fatalf("pulled %q", read())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.srm")); err == nil {
		t.Error("a path outside the folder was written")
	}

	// Edited here (OpenHome saved it): uploaded, once.
	os.WriteFile(local, []byte("edited"), 0o644)
	once()
	once()
	if dev.uploads != 1 || string(dev.versions[len(dev.versions)-1]) != "edited" {
		t.Fatalf("uploads %d, device has %q", dev.uploads, dev.versions[len(dev.versions)-1])
	}

	// Played on the device: pulled.
	dev.set("played")
	once()
	if read() != "played" {
		t.Fatalf("after playing: %q", read())
	}

	// Edited while the game is open: waits, then uploads.
	dev.inUse = true
	os.WriteFile(local, []byte("edit 2"), 0o644)
	once()
	if dev.uploads != 1 {
		t.Fatal("uploaded while the game was running")
	}
	dev.inUse = false
	once()
	if dev.uploads != 2 {
		t.Fatal("not uploaded after the game closed")
	}

	// Both changed: the local copy is kept aside, the device's pulled.
	os.WriteFile(local, []byte("edit 3"), 0o644)
	dev.set("played 2")
	once()
	if read() != "played 2" {
		t.Fatalf("after a conflict: %q", read())
	}
	aside, _ := os.ReadFile(filepath.Join(dir, "Conflicts", "Pokémon Game 20261011T120000Z.sav"))
	if string(aside) != "edit 3" {
		t.Fatalf("conflict copy = %q", aside)
	}

	// Refused uploads aren't retried until the file changes again.
	dev.refuse = true
	os.WriteFile(local, []byte("broken"), 0o644)
	once()
	once()
	if dev.uploads != 2 || s.st.Files["gba/Pokémon Game.srm"].Rejected == "" {
		t.Fatalf("refused upload: uploads %d, state %+v", dev.uploads, s.st.Files)
	}

	// The state survives a restart.
	s2, err := New(srv.URL, dir)
	if err != nil || s2.st.Files["gba/Pokémon Game.srm"] != s.st.Files["gba/Pokémon Game.srm"] {
		t.Fatalf("reloaded state %+v %v", s2.st, err)
	}
	if _, err := New("http://elsewhere:8080", dir); err == nil || !strings.Contains(err.Error(), "synced with") {
		t.Errorf("another device in the same folder: %v", err)
	}
}

func TestLocalNames(t *testing.T) {
	files := []remoteFile{{Path: "gba/Unbound.sav"}, {Path: "gba/Unbound.srm"}, {Path: "nds/HGG.dsv"}, {Path: "gbc/Crystal.srm"}, {Path: "gba/Crystal.srm"}}
	got := localNames(files)
	want := map[string]string{
		"gba/Unbound.srm": "Unbound.sav", "gba/Unbound.sav": "gba/Unbound.sav",
		"nds/HGG.dsv": "HGG.dsv", "gba/Crystal.srm": "Crystal.sav", "gbc/Crystal.srm": "gbc/Crystal.srm",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s → %q, want %q", k, got[k], v)
		}
	}
}

func TestSyncPIN(t *testing.T) {
	dev := &fakeDevice{pin: "1234"}
	dev.set("one")
	srv := httptest.NewServer(dev)
	defer srv.Close()
	s, _ := New(srv.URL, t.TempDir())
	s.Logf = t.Logf
	if err := s.Once(); err != ErrPINNeeded {
		t.Fatalf("no session: %v", err)
	}
	if err := s.SignIn("0000"); err == nil {
		t.Fatal("a wrong PIN signed in")
	}
	if err := s.SignIn("1234"); err != nil {
		t.Fatal(err)
	}
	if err := s.Once(); err != nil {
		t.Fatal(err)
	}
}
