package emulator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeProc(t *testing.T, procs map[string][]string) string {
	t.Helper()
	dir := t.TempDir()
	for pid, args := range procs {
		os.MkdirAll(filepath.Join(dir, pid), 0o755)
		os.WriteFile(filepath.Join(dir, pid, "cmdline"), []byte(strings.Join(args, "\x00")+"\x00"), 0o644)
	}
	os.MkdirAll(filepath.Join(dir, "self"), 0o755)
	os.WriteFile(filepath.Join(dir, "self", "cmdline"), []byte("x"), 0o644)
	return dir
}

func TestDetect(t *testing.T) {
	// The real command lines on the Pi.
	proc := fakeProc(t, map[string][]string{
		"1":    {"/sbin/init"},
		"3515": {"/bin/bash", "/opt/retropie/emulators/drastic/drastic.sh", "/home/pi/RetroPie/roms/nds/HeartGold Generations (2.0).nds"},
		"3516": {"/opt/retropie/emulators/drastic/drastic", "/home/pi/RetroPie/roms/nds/HeartGold Generations (2.0).nds"},
	})
	r, err := Detect(proc)
	if err != nil || r == nil || r.Emulator != "drastic" || r.PID != 3516 || r.Live() ||
		r.ROM != "/home/pi/RetroPie/roms/nds/HeartGold Generations (2.0).nds" {
		t.Errorf("got %+v, %v", r, err)
	}

	proc = fakeProc(t, map[string][]string{
		"1246": {"/opt/retropie/emulators/retroarch/bin/retroarch", "-L", "/opt/retropie/libretrocores/lr-mgba/mgba_libretro.so",
			"--config", "/opt/retropie/configs/gba/retroarch.cfg", "/home/pi/RetroPie/roms/gba/Game.gba",
			"--appendconfig", "/dev/shm/retroarch.cfg"},
	})
	if r, _ := Detect(proc); r == nil || !r.Live() || r.ROM != "/home/pi/RetroPie/roms/gba/Game.gba" {
		t.Errorf("retroarch: got %+v", r)
	}

	if r, _ := Detect(fakeProc(t, map[string][]string{"9": {"/usr/bin/retroarch", "--menu"}})); r != nil {
		t.Errorf("RetroArch without a game: got %+v", r)
	}
	if !Supported(proc) || Supported(t.TempDir()) {
		t.Error("Supported is wrong")
	}
}
