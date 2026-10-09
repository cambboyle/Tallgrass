package dashboard

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tallgrass/emulator"
	"tallgrass/profile"
	"tallgrass/saves"
)

func TestFollowsRunningGame(t *testing.T) {
	roms, drastic := t.TempDir(), t.TempDir()
	write := func(path, data string) {
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(data), 0o644)
	}
	gbaROM := filepath.Join(roms, "gba", "Red.gba")
	ndsROM := filepath.Join(roms, "nds", "Gold.nds")
	write(gbaROM, "a gba rom")
	write(ndsROM, "a ds rom")
	write(filepath.Join(roms, "gba", "Red.srm"), "red save")
	write(filepath.Join(roms, "gba", "Other.srm"), "other save")
	write(filepath.Join(roms, "gba", "Other.gba"), "another rom")
	write(filepath.Join(drastic, "Gold.dsv"), "gold save")

	sum := sha1.Sum([]byte("a gba rom"))
	prof := *testProfile
	prof.Name = "Red Hack"
	prof.ROMSHA1 = []string{hex.EncodeToString(sum[:])}

	var running *emulator.Running
	s := New(testProfile, memory(6))
	s.EnableDetection(func() (*emulator.Running, error) { return running, nil }, []*profile.Profile{&prof})
	w := saves.NewWatcher(saves.RetroPieSources(roms, drastic), saves.NewBackups(t.TempDir()))
	w.Scan()
	s.EnableSaves(w, nil)

	st := s.Snapshot()
	if st.Game != "" || st.Running != nil || st.Connected {
		t.Errorf("no game: %+v", st)
	}

	running = &emulator.Running{Emulator: "drastic", PID: 2, ROM: ndsROM}
	st = s.Snapshot()
	if st.Game != "Gold.nds" || st.Running == nil || st.Running.Supported || st.Running.Live || st.Connected {
		t.Errorf("DS game: %+v %+v", st, st.Running)
	}
	if !s.saveInUse("nds/Gold.dsv") || s.saveInUse("gba/Red.srm") {
		t.Error("only the running DS game's save should be in use")
	}

	running = &emulator.Running{Emulator: "retroarch", PID: 3, ROM: gbaROM}
	time.Sleep(time.Millisecond)
	st = s.Snapshot()
	if st.Game != "Red Hack" || !st.Running.Supported || !st.Running.Live || !st.Connected || st.Party[0] == nil {
		t.Errorf("supported game: %+v %+v", st, st.Running)
	}
	if !s.saveInUse("gba/Red.srm") || s.saveInUse("gba/Other.srm") || s.saveInUse("nds/Gold.dsv") {
		t.Error("only Red's save should be in use")
	}

	running = nil
	if st = s.Snapshot(); st.Connected || st.Running != nil {
		t.Errorf("after closing: %+v", st)
	}
}
