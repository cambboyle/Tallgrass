package tracker

import (
	"path/filepath"
	"testing"
	"time"
)

func TestExport(t *testing.T) {
	g := &game{t: t, e: newEngine(t, openStore(t, filepath.Join(t.TempDir(), "test.db"))), now: time.Now()}
	g.obs = Observation{TrainerID: 1, PlayerName: "Cameron", Section: 0x65, Area: "Route 1",
		Party: []PartyMon{cyndaquil(6, 21)}, DexCaught: []int{155}}
	g.tick(time.Second)
	g.tick(time.Second)
	g.obs.Party = append(g.obs.Party, pidgey)
	g.obs.DexCaught = []int{16, 155}
	g.tick(time.Second)
	g.tick(time.Second)

	runs, err := g.e.store.Export()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("%d runs", len(runs))
	}
	r := runs[0]
	if r.Player != "Cameron" || len(r.Mons) != 2 || len(r.Areas) != 1 || len(r.Caught) != 2 || len(r.Events) == 0 {
		t.Errorf("export = %+v", r)
	}
	for i := 1; i < len(r.Events); i++ {
		if r.Events[i].ID < r.Events[i-1].ID {
			t.Error("events aren't oldest first")
		}
	}
}
