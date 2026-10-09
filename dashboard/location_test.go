package dashboard

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"tallgrass/tracker"
	"testing"
	"time"

	"tallgrass/profile"
)

// Uses the real Radical Red ROM plus fake RAM: the player is on Route 1 at
// 22:00, has caught Pidgey (#16) and seen Rattata (#19).
func TestRadicalRedRoute1AtNight(t *testing.T) {
	path := os.Getenv("TALLGRASS_ROM")
	if path == "" {
		t.Skip("set TALLGRASS_ROM to a Radical Red ROM to run")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	prof, err := profile.Load("radical-red")
	if err != nil {
		t.Fatal(err)
	}

	seen, caught := make([]byte, 164), make([]byte, 164)
	caught[(16-1)/8] |= 1 << ((16 - 1) % 8)
	seen[(16-1)/8] |= 1 << ((16 - 1) % 8)
	seen[(19-1)/8] |= 1 << ((19 - 1) % 8)

	mem := map[uint32][]byte{
		0x08000000: rom,
		0x02024284: make([]byte, 600),                                 // empty party
		0x03005008: binary.LittleEndian.AppendUint32(nil, 0x02025000), // SaveBlock1 pointer
		0x02025004: {3, 19},                                           // map group, number: Route 1
		0x02036E10: {0x65},                                            // map section: Route 1
		0x03005EA6: {22},                                              // hour
		0x020258E0: caught,
		0x0202583C: seen,
	}
	read := func(addr uint32, n int) ([]byte, error) {
		for base, b := range mem {
			if addr >= base && int(addr-base)+n <= len(b) {
				return b[addr-base : int(addr-base)+n], nil
			}
		}
		return nil, fmt.Errorf("unmapped read %08X+%d", addr, n)
	}

	loc := New(prof, read).Snapshot().Location
	if loc == nil || loc.Name != "Route 1" || len(loc.Periods) != 2 {
		t.Fatalf("location = %+v", loc)
	}

	day, night := loc.Periods[0], loc.Periods[1]
	if day.Active || !night.Active {
		t.Errorf("at 22:00 night should be active: day %v, night %v", day.Active, night.Active)
	}

	found := map[string]EncounterView{}
	for _, e := range night.Methods[0].Encounters {
		found[e.Name] = e
		t.Logf("%-10s Lv %d-%d %3d%% seen=%v caught=%v sprite=%q", e.Name, e.MinLevel, e.MaxLevel, e.Chance, e.Seen, e.Caught, e.Sprite)
	}
	if p := found["Pidgey"]; !p.Caught || !p.Seen || p.Sprite == "" {
		t.Errorf("Pidgey = %+v", p)
	}
	if r := found["Rattata"]; r.Caught || !r.Seen {
		t.Errorf("Rattata = %+v", r)
	}
	if _, ok := found["Hoothoot"]; !ok {
		t.Error("Hoothoot missing at night")
	}
}

// The tracker reading the PC through the real decoders: Radical Red ROM,
// fake RAM with Wooloo's real 58-byte record.
func TestRadicalRedPCCatch(t *testing.T) {
	path := os.Getenv("TALLGRASS_ROM")
	if path == "" {
		t.Skip("set TALLGRASS_ROM to a Radical Red ROM to run")
	}
	rom, _ := os.ReadFile(path)
	prof, _ := profile.Load("radical-red")
	store, err := tracker.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	wooloo, _ := hex.DecodeString("858550b9fcfacc18d1e3e3e0e3e3ff0309000202bdd5e1d9e6e3e200" +
		"63040000400000000032032d84f006000000000000000059040284" + "5be619")
	storage := make([]byte, 4+19*30*58)
	copy(storage[4:], wooloo)
	party := make([]byte, 600)
	party[32] = 155 // a party, so the game counts as running
	party[84] = 9
	sb2 := []byte{0xBD, 0xD5, 0xE1, 0xD9, 0xE6, 0xE3, 0xE2, 0xFF, 0, 0, 0xFC, 0xFA, 0xCC, 0x18}
	ptr := func(a uint32) []byte { return binary.LittleEndian.AppendUint32(nil, a) }
	mem := map[uint32][]byte{
		0x08000000: rom, 0x02024284: party,
		0x03005008: ptr(0x02025000), 0x02025004: {3, 19}, 0x02036E10: {0x65}, 0x03005EA6: {12},
		0x0300500C: ptr(0x02024588), 0x02024588: sb2,
		0x03005010: ptr(0x02029314), 0x02029314: storage,
		0x020258E0: make([]byte, 164), 0x0202583C: make([]byte, 164),
	}
	read := func(addr uint32, n int) ([]byte, error) {
		for base, b := range mem {
			if addr >= base && int(addr-base)+n <= len(b) {
				return b[addr-base : int(addr-base)+n], nil
			}
		}
		return nil, fmt.Errorf("unmapped %08X+%d", addr, n)
	}

	s := New(prof, read)
	s.EnableTracking(store)
	poll := func() *tracker.View {
		s.lastPC = time.Time{} // don't wait 10 s between PC reads
		return s.Snapshot().Run
	}
	poll()
	if v := poll(); len(v.Events) == 0 || v.Events[0].Kind == tracker.KindCaughtPC {
		t.Fatalf("Wooloo was already in the PC; events = %+v", v.Events)
	}

	// Another Wooloo (new personality) turns up in slot 2.
	copy(storage[4+58:], wooloo)
	storage[4+58] ^= 1
	poll()
	v := poll()
	if ev := v.Events[0]; ev.Kind != tracker.KindCaughtPC || ev.Text != "Caught Wooloo Lv 4 on Viridian City, sent to the PC" {
		t.Errorf("newest event = %+v", ev)
	}
}
