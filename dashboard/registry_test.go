package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"tallgrass/cloud"
	"tallgrass/profile"
)

// Registry profiles go behind local ones and in front of bundled ones,
// profiles needing a newer Tallgrass are skipped, and the download is
// cached for next time.
func TestRegistry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/rest/v1/game_profiles" || r.Header.Get("apikey") != "anon" {
			http.Error(w, "bad request", 400)
			return
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"slug": "fixed", "version": 2, "profile": map[string]any{"name": "Registry Hack", "romSha1": []string{"aaa"}, "partyAddress": "0x02024284", "partySlots": 6, "pokemonSize": 100}},
			{"slug": "future", "version": 1, "profile": map[string]any{"name": "Future Hack", "minTallgrass": "9.0.0", "romSha1": []string{"bbb"}, "partyAddress": "0x02024284", "partySlots": 6, "pokemonSize": 100}},
		})
	}))
	defer srv.Close()

	bundled := []*profile.Profile{{Name: "Bundled Hack", ROMSHA1: []string{"aaa", "ccc"}}}
	local := []*profile.Profile{{Name: "Local Hack", ROMSHA1: []string{"ddd"}}}
	cache := filepath.Join(t.TempDir(), "registry.json")
	s := New(testProfile, func(uint32, int) ([]byte, error) { return nil, errors.New("no game") })
	s.EnableRegistry(cloud.New(srv.URL, "anon"), cache, "0.1.17", local, bundled)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(cache); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	names := func() (out []string) {
		for _, p := range s.profileList() {
			out = append(out, p.Name)
		}
		return
	}
	if got := names(); len(got) != 3 || got[0] != "Local Hack" || got[1] != "Registry Hack" || got[2] != "Bundled Hack" {
		t.Fatalf("profiles = %v", got)
	}
	if p := s.profileFor("aaa", "x.gba"); p == nil || p.Name != "Registry Hack" {
		t.Errorf("aaa uses %v; the registry's fix should win over the bundled profile", p)
	}

	// Next start, offline: the cached download is used straight away.
	s2 := New(testProfile, func(uint32, int) ([]byte, error) { return nil, errors.New("no game") })
	s2.EnableRegistry(cloud.New("http://127.0.0.1:1", "anon"), cache, "0.1.17", nil, bundled)
	if p := s2.profileFor("aaa", "x.gba"); p == nil || p.Name != "Registry Hack" {
		t.Errorf("offline start uses %v", p)
	}
}
