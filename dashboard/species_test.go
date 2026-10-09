package dashboard

import (
	"os"
	"testing"

	"tallgrass/gen3"
	"tallgrass/profile"
)

// Species pages from the real ROMs, through each profile's speciesData.
func TestSpeciesPages(t *testing.T) {
	for env, name := range map[string]string{
		"TALLGRASS_ROM": "radical-red", "TALLGRASS_UNBOUND_ROM": "unbound", "TALLGRASS_EMERALD_ROM": "emerald",
	} {
		path := os.Getenv(env)
		if path == "" {
			t.Logf("skipping %s: set %s", name, env)
			continue
		}
		rom, err := gen3.OpenROM(path)
		if err != nil {
			t.Fatal(err)
		}
		defer rom.Close()
		p, _ := profile.Load(name)
		tables, err := speciesTables(p, rom.Read)
		if err != nil {
			t.Fatal(err)
		}
		names := gen3.NewNames(rom.Read, nameTables(p))
		rc, err := newROMContext(path, "test", p)
		if err != nil {
			t.Fatal(err)
		}
		rc.species = tables
		rc.buildIndexes()
		// A species from the game's own first wild encounter table.
		mainTable, _ := p.WildEncounters.Headers.Resolve(rom.Read)
		headers, _ := gen3.ReadWildHeaders(rom.Read, mainTable)
		periods, _ := gen3.NewWild(rom.Read, mainTable, nil).ForMap(headers[0].Group, headers[0].Num)
		wildSpecies := periods[0].Methods[0].Encounters[0].Species
		for _, sp := range []int{2, wildSpecies} { // Ivysaur, something wild
			n, _ := names.Species(sp)
			t.Logf("%s %s: evolves from %+v; found in %d places, e.g. %+v", name, n, rc.evolvesFrom[sp], len(rc.foundIn[sp]), rc.foundIn[sp][:min(3, len(rc.foundIn[sp]))])
		}
		if len(rc.evolvesFrom[2]) != 1 || rc.evolvesFrom[2][0].Name != "Bulbasaur" || len(rc.foundIn[wildSpecies]) == 0 {
			t.Errorf("%s: indexes look wrong", name)
		}
		for _, species := range []int{1, 133} { // Bulbasaur, Eevee
			info, err := tables.Species(rom.Read, names, species)
			if err != nil {
				t.Fatalf("%s %d: %v", name, species, err)
			}
			t.Logf("%s: %s %v %+v abilities %v hidden %q; evolves %d ways (first: %+v); %d level-up moves, first %+v",
				name, info.Name, info.Types, info.BaseStats, info.Abilities, info.Hidden, len(info.EvolvesTo),
				info.EvolvesTo[0], len(info.LevelMoves), info.LevelMoves[0])
			if len(info.Types) == 0 || len(info.Abilities) == 0 || len(info.LevelMoves) < 5 || len(info.EvolvesTo) == 0 {
				t.Errorf("%s %s looks incomplete", name, info.Name)
			}
		}
	}
}
