package tracker

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var speciesByNational = map[int]string{16: "Pidgey", 19: "Rattata", 155: "Cyndaquil", 156: "Quilava"}

func newEngine(t *testing.T, store *Store) *Engine {
	return NewEngine(store, "Test", func(n int) (int, string) { return n, speciesByNational[n] })
}

func openStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func cyndaquil(level, hp int) PartyMon {
	return PartyMon{Personality: 0x79D75605, Species: 155, SpeciesName: "Cyndaquil", National: 155,
		Nickname: "Blaze", Level: level, CurrentHP: hp, MetSection: 0x9D, MetArea: "Oak's Lab"}
}

var pidgey = PartyMon{Personality: 0x1234, Species: 16, SpeciesName: "Pidgey", National: 16,
	Nickname: "Pidgey", Level: 3, CurrentHP: 12, MetSection: 0x65, MetArea: "Route 1"}

// game drives the engine with a clock and a mutable game state.
type game struct {
	t     *testing.T
	e     *Engine
	now   time.Time
	obs   Observation
	kinds []string // every event kind emitted, in order
	last  int64    // highest event ID seen
}

func (g *game) tick(d time.Duration) {
	g.t.Helper()
	g.now = g.now.Add(d)
	g.obs.At = g.now
	if err := g.e.Observe(g.obs); err != nil {
		g.t.Fatal(err)
	}
	for _, ev := range g.e.events {
		if ev.ID > g.last {
			g.kinds = append(g.kinds, ev.Kind)
			g.last = ev.ID
		}
	}
}

func (g *game) expect(kinds ...string) {
	g.t.Helper()
	if strings.Join(g.kinds, ",") != strings.Join(kinds, ",") {
		g.t.Errorf("events = %v, want %v", g.kinds, kinds)
	}
	g.kinds = nil
}

func TestRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store := openStore(t, path)
	g := &game{t: t, e: newEngine(t, store), now: time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)}

	// Title screen: nothing loaded, nothing recorded.
	g.tick(time.Second)
	g.expect()

	// Save loaded in Oak's Lab with the starter, already in the Pokédex.
	g.obs = Observation{TrainerID: 0x18CCFAFC, PlayerName: "Cameron", Section: 0x9D, Area: "Oak's Lab",
		Party: []PartyMon{cyndaquil(6, 21)}, DexCaught: []int{155}}
	g.tick(time.Second)
	g.expect(KindStarted, KindNewArea) // the starter waits for a second sighting
	g.tick(time.Second)
	g.expect(KindObtained)

	// Walk to Route 1.
	g.obs.Section, g.obs.Area = 0x65, "Route 1"
	g.tick(time.Second)
	g.expect(KindNewArea)

	// Catch a Pidgey: the dex flag comes first, the party slot after the
	// nickname prompt. That's an ordinary catch, not a PC one.
	g.obs.DexCaught = []int{16, 155}
	g.tick(time.Second)
	g.tick(30 * time.Second)
	g.obs.Party = append(g.obs.Party, pidgey)
	g.tick(time.Second)
	g.tick(time.Second)
	g.tick(5 * time.Minute)
	g.expect(KindObtained)

	// Catch a Rattata that goes to the PC: only the dex flag changes.
	g.obs.DexCaught = []int{16, 19, 155}
	g.tick(time.Second)
	g.tick(time.Minute)
	g.expect()
	g.tick(2 * time.Minute)
	g.expect(KindCaughtPC)

	// Level up, then faint (once, however long HP stays at 0).
	g.obs.Party[0] = cyndaquil(7, 23)
	g.tick(time.Second)
	g.obs.Party[0] = cyndaquil(7, 0)
	g.tick(time.Second)
	g.tick(time.Second)
	g.expect(KindLevelUp, KindFainted)

	// Restart: same state, nothing new.
	g.e = newEngine(t, openStore(t, path))
	g.tick(time.Second)
	g.tick(time.Second)
	g.expect()

	// Revived, evolves, faints again: the death time stays the first faint.
	g.obs.Party[0] = cyndaquil(14, 40)
	g.obs.Party[0].Species, g.obs.Party[0].SpeciesName, g.obs.Party[0].National = 156, "Quilava", 156
	g.obs.DexCaught = []int{16, 19, 155, 156}
	g.tick(time.Second)
	g.obs.Party[0].CurrentHP = 0
	g.tick(5 * time.Minute)
	g.expect(KindEvolved, KindLevelUp, KindFainted)

	v := g.e.View()
	if v.Alive != 1 || v.Dead != 1 {
		t.Errorf("alive/dead = %d/%d", v.Alive, v.Dead)
	}
	blaze := v.Mons[len(v.Mons)-1]
	firstFaint := time.Date(2026, 10, 9, 14, 8, 40, 0, time.UTC)
	if blaze.SpeciesName != "Quilava" || blaze.FaintedAt == nil || !blaze.FaintedAt.Equal(firstFaint) {
		t.Errorf("Blaze = %+v (fainted %v)", blaze, blaze.FaintedAt)
	}
	got := map[string]string{}
	for _, a := range v.Areas {
		got[a.Name] = strings.Join(a.Catches, "; ")
	}
	// Catches show the Pokémon as it is now, so Blaze shows as evolved.
	want := map[string]string{"Oak's Lab": "Blaze (Quilava)", "Route 1": "Pidgey; Rattata (PC)"}
	for area, catches := range want {
		if got[area] != catches {
			t.Errorf("%s catches = %q, want %q", area, got[area], catches)
		}
	}
	if v.Events[0].Kind != KindFainted {
		t.Errorf("newest event = %s", v.Events[0].Kind)
	}

	// A different save is a different run.
	g.obs = Observation{TrainerID: 99, PlayerName: "Red", Section: 0x58, Area: "Pallet Town"}
	g.tick(time.Second)
	g.expect(KindStarted, KindNewArea)
	if v := g.e.View(); v.Player != "Red" || len(v.Mons) != 0 {
		t.Errorf("new run view = %+v", v)
	}
}

func TestIgnoresGarbageParty(t *testing.T) {
	g := &game{t: t, e: newEngine(t, openStore(t, filepath.Join(t.TempDir(), "test.db"))), now: time.Now()}
	g.obs = Observation{TrainerID: 1, PlayerName: "A", Section: -1}
	g.tick(time.Second)
	g.kinds = nil

	// Half-loaded data, and a newcomer seen for a single poll only.
	g.obs.Party = []PartyMon{{Personality: 5, Species: 9999, Level: 0}}
	g.tick(time.Second)
	g.obs.Party = []PartyMon{pidgey}
	g.tick(time.Second)
	g.obs.Party = nil
	g.tick(time.Second)
	g.expect()
}

func TestNuzlockeSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	g := &game{t: t, e: newEngine(t, openStore(t, path)), now: time.Now()}
	g.obs = Observation{TrainerID: 1, PlayerName: "A", Section: -1}
	g.tick(time.Second)
	if g.e.View().Nuzlocke {
		t.Error("Nuzlocke mode should be off by default")
	}
	if err := g.e.SetNuzlocke(true); err != nil {
		t.Fatal(err)
	}

	// Remembered across restarts, per game.
	g.e = newEngine(t, openStore(t, path))
	g.tick(time.Second)
	if !g.e.View().Nuzlocke {
		t.Error("Nuzlocke mode should survive a restart")
	}
	g.e.SetGame("Other")
	g.tick(time.Second)
	if g.e.View().Nuzlocke {
		t.Error("Nuzlocke mode is per game")
	}
}

func TestPCCatches(t *testing.T) {
	g := &game{t: t, e: newEngine(t, openStore(t, filepath.Join(t.TempDir(), "test.db"))), now: time.Now()}
	full := []PartyMon{cyndaquil(9, 30)}
	g.obs = Observation{TrainerID: 1, PlayerName: "A", Section: 0x65, Area: "Route 1", Party: full, DexCaught: []int{155}}
	g.tick(time.Second)
	g.tick(time.Second)
	g.kinds = nil

	old := PartyMon{Personality: 7, Species: 1123, SpeciesName: "Wooloo", National: 831, Nickname: "Wooloo", Level: 4,
		MetSection: 0x59, MetArea: "Viridian City"}
	g.obs.Boxes = []PartyMon{old} // first PC read: already there, not news
	g.tick(time.Second)
	g.obs.Boxes = nil // the PC isn't read every poll
	g.tick(time.Second)
	g.expect()

	// A second Pidgey (species already caught, so no new dex flag) goes to the PC.
	again := pidgey
	again.Personality = 0x9999
	g.obs.DexCaught = []int{16, 155}
	g.obs.Boxes = []PartyMon{old, again}
	g.tick(10 * time.Second)
	g.tick(10 * time.Second)
	g.obs.Boxes = nil
	g.tick(5 * time.Minute) // the dex-flag guess must not add a second event
	g.expect(KindCaughtPC)

	v := g.e.View()
	if v.Events[0].Personality != 0x9999 || !strings.Contains(v.Events[0].Text, "Pidgey Lv 3 on Route 1") {
		t.Errorf("event = %+v", v.Events[0])
	}
	for _, a := range v.Areas {
		if a.Name == "Route 1" && strings.Join(a.Catches, ",") != "Pidgey" {
			t.Errorf("Route 1 catches = %v", a.Catches)
		}
	}
}

// The view must come out the same every time, even when Pokémon and areas
// were recorded at the same moment (map iteration order is random in Go).
func TestViewIsStable(t *testing.T) {
	g := &game{t: t, e: newEngine(t, openStore(t, filepath.Join(t.TempDir(), "test.db"))), now: time.Now()}
	var party []PartyMon
	for i := 0; i < 6; i++ {
		p := pidgey
		p.Personality = uint32(100 + i)
		p.MetSection, p.MetArea = 0x60+i, fmt.Sprintf("Route %d", i+1)
		party = append(party, p)
	}
	g.obs = Observation{TrainerID: 1, PlayerName: "A", Section: -1, Party: party}
	g.tick(time.Second)
	g.tick(time.Second) // all six recorded at the same instant

	first, _ := json.Marshal(g.e.View())
	for i := 0; i < 20; i++ {
		again, _ := json.Marshal(g.e.View())
		if string(again) != string(first) {
			t.Fatal("view order changed between calls")
		}
	}
	if v := g.e.View(); v.Mons[0].Personality != 100 || v.Mons[5].Personality != 105 {
		t.Errorf("party not in slot order: %v, %v", v.Mons[0].Personality, v.Mons[5].Personality)
	}
}
