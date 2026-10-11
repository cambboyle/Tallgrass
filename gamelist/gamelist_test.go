package gamelist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const before = `<?xml version="1.0"?>
<gameList>
	<game>
		<path>./1636 - Pokemon Fire Red (U)(Squirrels) (patched).gba</path>
		<name>1636 - Pokemon Fire Red (U)(Squirrels) (patched)</name>
		<playcount>2</playcount>
		<lastplayed>20261009T171652</lastplayed>
	</game>
	<game id="7">
		<path>./PokemonUnbound.gba</path>
		<name>My Unbound run</name>
		<image>./images/unbound.png</image>
	</game>
	<folder>
		<path>./hacks</path>
		<name>Hacks</name>
	</folder>
</gameList>
`

func TestApply(t *testing.T) {
	file := filepath.Join(t.TempDir(), "gba", "gamelist.xml")
	os.MkdirAll(filepath.Dir(file), 0o755)
	os.WriteFile(file, []byte(before), 0o644)

	res, err := Apply(file, []Game{
		{Path: "./1636 - Pokemon Fire Red (U)(Squirrels) (patched).gba", Name: "Pokémon Radical Red", Desc: "A FireRed romhack."},
		{Path: "./PokemonUnbound.gba", Name: "Pokémon Unbound", Desc: "A FireRed romhack."},
		{Path: "./ROWE.gba", Name: "Pokémon R.O.W.E.", Desc: "An Emerald romhack."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Renamed, ",") != "Pokémon Radical Red" || strings.Join(res.Added, ",") != "Pokémon R.O.W.E." {
		t.Errorf("result = %+v", res)
	}
	b, _ := os.ReadFile(file)
	got := string(b)
	for _, want := range []string{
		"<name>Pokémon Radical Red</name>", "<playcount>2</playcount>", "<lastplayed>20261009T171652</lastplayed>",
		"<name>My Unbound run</name>", // a name the player chose is kept
		`<game id="7">`, "<image>./images/unbound.png</image>", "<desc>A FireRed romhack.</desc>",
		"<path>./ROWE.gba</path>", "<folder>", "<name>Hacks</name>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if bak, _ := os.ReadFile(file + ".bak-tallgrass"); string(bak) != before {
		t.Error("the original wasn't kept")
	}

	// Running it again changes nothing.
	res, err = Apply(file, []Game{{Path: "./PokemonUnbound.gba", Name: "Pokémon Unbound", Desc: "x"}})
	if err != nil || len(res.Renamed)+len(res.Added) != 0 {
		t.Errorf("second run: %+v %v", res, err)
	}
	if b2, _ := os.ReadFile(file); string(b2) != got {
		t.Error("a run with nothing to do rewrote the file")
	}
}

func TestApplyCreates(t *testing.T) {
	file := filepath.Join(t.TempDir(), "nds", "gamelist.xml")
	res, err := Apply(file, []Game{{Path: "./HGG.nds", Name: "HeartGold Generations", Desc: "d"}})
	if err != nil || len(res.Added) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := os.Stat(file + ".bak-tallgrass"); err == nil {
		t.Error("no original to back up, but a backup appeared")
	}
}
