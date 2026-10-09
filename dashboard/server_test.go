package dashboard

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"tallgrass/tracker"
	"testing"
	"time"

	"tallgrass/gen3"
	"tallgrass/profile"
)

var testProfile = &profile.Profile{Name: "Test", PartyAddress: 0x02024284, PartySlots: 6, PokemonSize: 100}

// memory fakes game memory: a level-n species 155 in party slot 1, and every
// name in the ROM reading as "A".
func memory(level byte) gen3.ReadMemory {
	return func(addr uint32, n int) ([]byte, error) {
		if addr == 0x02024284 {
			b := make([]byte, 600)
			b[32] = 155
			b[84] = level
			return b, nil
		}
		b := bytes.Repeat([]byte{0xFF}, n)
		b[0] = 0xBB // "A"
		return b, nil
	}
}

func TestPartyEndpoint(t *testing.T) {
	s := New(testProfile, memory(6))
	s.update(s.Snapshot())

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/party", nil))

	var st struct {
		Connected bool
		Party     []*struct {
			Species, Level int
			SpeciesName    string
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Connected || len(st.Party) != 6 || st.Party[0].Species != 155 || st.Party[0].Level != 6 ||
		st.Party[0].SpeciesName != "A" || st.Party[1] != nil {
		t.Errorf("unexpected state: %s", rec.Body)
	}
}

func TestNoGame(t *testing.T) {
	s := New(testProfile, func(uint32, int) ([]byte, error) { return nil, errors.New("i/o timeout") })
	st := s.Snapshot()
	if st.Connected || st.Error == "" {
		t.Errorf("got %+v, want disconnected with error", st)
	}
}

func TestEventsPushChanges(t *testing.T) {
	s := New(testProfile, memory(6))
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := bufio.NewScanner(resp.Body)

	next := func() string {
		for lines.Scan() {
			if data, ok := strings.CutPrefix(lines.Text(), "data: "); ok {
				return data
			}
		}
		t.Fatal("stream ended")
		return ""
	}

	next() // initial state
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.read = memory(7)
		s.update(s.Snapshot())
	}()
	if got := next(); !strings.Contains(got, `"level":7`) {
		t.Errorf("pushed state missing level 7: %s", got)
	}
}

func TestNuzlockeToggle(t *testing.T) {
	store, err := tracker.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	prof := *testProfile
	prof.Trainer = &profile.Trainer{Name: profile.Ref{Addr: 0x02024000}, ID: profile.Ref{Addr: 0x0202400A}}
	base := memory(6)
	s := New(&prof, func(addr uint32, n int) ([]byte, error) {
		if addr == 0x02024000 || addr == 0x0202400A {
			return []byte{0xBB, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}[:n], nil // trainer "A", ID FFFFFFBB
		}
		return base(addr, n)
	})
	s.EnableTracking(store)
	if st := s.Snapshot(); st.Run == nil || st.Run.Nuzlocke {
		t.Fatalf("run = %+v", st.Run)
	}

	req := httptest.NewRequest("POST", "/api/settings/nuzlocke", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("toggle: %d %s", rec.Code, rec.Body)
	}
	if st := s.Snapshot(); !st.Run.Nuzlocke {
		t.Error("the next poll should apply Nuzlocke mode")
	}
}
