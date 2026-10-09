package autoprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tallgrass/gen3"
	"tallgrass/profile"
)

func generate(t *testing.T, path string) (*profile.Profile, gen3.ReadMemory) {
	t.Helper()
	rom, err := gen3.OpenROM(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rom.Close() })
	info, _ := os.Stat(path)
	p, err := ForFireRed(filepath.Base(path), "sha", rom.Read, int(info.Size()))
	if err != nil {
		t.Fatal(err)
	}
	return p, rom.Read
}

// Generated profiles must agree with the hand-verified ones.
func TestMatchesHandMadeProfiles(t *testing.T) {
	for env, name := range map[string]string{"TALLGRASS_ROM": "radical-red", "TALLGRASS_UNBOUND_ROM": "unbound"} {
		path := os.Getenv(env)
		if path == "" {
			t.Logf("skipping %s: set %s", name, env)
			continue
		}
		auto, _ := generate(t, path)
		hand, _ := profile.Load(name)
		check := func(what string, got, want any) {
			if got != want {
				t.Errorf("%s %s: auto %v, hand-made %v", name, what, got, want)
			}
		}
		check("encrypted", auto.EncryptedData, hand.EncryptedData)
		check("move names", auto.MoveNames, hand.MoveNames)
		check("exp row length", auto.Experience.RowLength, hand.Experience.RowLength)
		check("dex caught", auto.Pokedex.Caught, hand.Pokedex.Caught)
		check("dex seen", auto.Pokedex.Seen, hand.Pokedex.Seen)
		check("dex bytes", auto.Pokedex.FlagBytes, hand.Pokedex.FlagBytes)
		check("save dex caught", auto.SaveFile.DexCaught, hand.SaveFile.DexCaught)
		check("sector size", auto.SaveFile.SectorDataSize, hand.SaveFile.SectorDataSize)
		check("box format", auto.SaveFile.BoxMonSize, hand.SaveFile.BoxMonSize)
		check("boxes", auto.SaveFile.Boxes, hand.SaveFile.Boxes)
		check("wild headers", auto.WildEncounters.Headers, hand.WildEncounters.Headers)
		autoSD, handSD := *auto.SpeciesData, *hand.SpeciesData
		autoSD.EvolutionScheme, handSD.EvolutionScheme = "", "" // hand-made profiles name the hack
		check("species data", autoSD, handSD)
		check("trainers", *auto.Trainers, *hand.Trainers)
	}
}

// Every FireRed-engine ROM in a folder (run on the Pi): the generated
// profile must lead to sensible names, levels and encounters.
func TestFolder(t *testing.T) {
	dir := os.Getenv("TALLGRASS_GBA_DIR")
	if dir == "" {
		t.Skip("set TALLGRASS_GBA_DIR to a folder of GBA ROMs")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if !strings.HasSuffix(e.Name(), ".gba") {
			continue
		}
		rom, _ := gen3.OpenROM(path)
		code, _ := rom.Read(0x080000AC, 4)
		rom.Close()
		if string(code) != "BPRE" {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			p, read := generate(t, path)
			resolve := func(r profile.Ref) uint32 { a, _ := r.Resolve(read); return a }
			names := gen3.NewNames(read, gen3.NameTables{Moves: uint32(p.MoveNames)})
			mv, _ := names.Move(1)
			sp, _ := names.Species(25)
			lv, _ := gen3.NewLevels(read, resolve(p.Experience.BaseStats), resolve(p.Experience.Table), p.Experience.RowLength).Level(1, 1059860)
			n, _ := gen3.NewNational(read, resolve(p.Pokedex.SpeciesToNational)).Number(25)
			wild, err := gen3.ReadWildHeaders(read, resolve(p.WildEncounters.Headers))
			t.Logf("encrypted=%v moves@%v rows=%d dex %v/%v (%d bytes) boxes %d×%d | move 1 %q, #25 %q (national %d), Lv %d, %d wild maps",
				p.EncryptedData, p.MoveNames, p.Experience.RowLength, p.Pokedex.Seen, p.Pokedex.Caught, p.Pokedex.FlagBytes,
				p.SaveFile.Boxes, p.SaveFile.BoxMonSize, mv, sp, n, lv, len(wild))
			if mv != "Pound" || sp != "Pikachu" || n != 25 || lv != 100 || err != nil || len(wild) < 20 {
				t.Error("generated profile doesn't check out")
			}
		})
	}
}
