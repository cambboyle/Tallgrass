package dashboard

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"tallgrass/profile"
	"tallgrass/saves"
)

func TestSetConfigValue(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "retroarch.cfg")
	orig := "video_smooth = \"false\"\nnetwork_cmd_enable = \"false\"\n# network_cmd_enable = \"true\"\naudio = \"on\"\nnetwork_cmd_enable = \"false\"\n"
	os.WriteFile(file, []byte(orig), 0o644)

	if err := setConfigValue(file, "network_cmd_enable", "true"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(file)
	want := "video_smooth = \"false\"\nnetwork_cmd_enable = \"true\"\n# network_cmd_enable = \"true\"\naudio = \"on\"\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if v, _ := readConfigValue(file, "network_cmd_enable"); v != "true" {
		t.Errorf("read back %q", v)
	}
	// The original is kept, once.
	if b, _ := os.ReadFile(file + ".bak-tallgrass"); string(b) != orig {
		t.Error("backup isn't the original")
	}
	setConfigValue(file, "autosave_interval", "60")
	if b, _ := os.ReadFile(file + ".bak-tallgrass"); string(b) != orig {
		t.Error("a second edit replaced the backup")
	}
	if got, _ := os.ReadFile(file); !strings.HasSuffix(string(got), "autosave_interval = \"60\"\n") {
		t.Errorf("new key not appended:\n%s", got)
	}
}

func TestBlankSave(t *testing.T) {
	if !blank([]byte{0xFF, 0xFF, 0}) || blank([]byte{0xFF, 1}) {
		t.Error("blank")
	}
}

// Every GBA game the wizard offers has a bundled profile for the same ROM,
// so the live read can work once the wizard finds it.
func TestSetupGamesHaveProfiles(t *testing.T) {
	all, err := profile.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range setupGames {
		if g.System != "gba" {
			continue // DS games need no profile
		}
		found := false
		for _, p := range all {
			for _, sum := range p.ROMSHA1 {
				if slices.Contains(g.SHA1, sum) {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%s: no bundled profile for its ROM", g.Slug)
		}
	}
}

// The catalogue and the wizard describe the same games: each wizard game is in
// the catalogue with the same supported-build SHA-1, and the same system.
func TestSetupGamesMatchCatalog(t *testing.T) {
	for _, g := range setupGames {
		h := knownHacks.Hack(g.Slug)
		if h == nil {
			t.Errorf("%s isn't in catalog/hacks.json", g.Slug)
			continue
		}
		if h.System != g.System {
			t.Errorf("%s: wizard says %s, catalogue says %s", g.Slug, g.System, h.System)
		}
		if !slices.Equal(h.Result.SHA1, g.SHA1) {
			t.Errorf("%s: wizard SHA-1 %v, catalogue %v", g.Slug, g.SHA1, h.Result.SHA1)
		}
	}
}

// With no ROM in the folder, the wizard says who made the game, what to patch,
// and links to the official page.
func TestSetupMissingROMUsesCatalog(t *testing.T) {
	dir := t.TempDir()
	w := saves.NewWatcher([]saves.Source{{System: "gba", SaveDir: dir, ROMDir: dir}}, saves.NewBackups(t.TempDir()))
	s := New(testProfile, func(uint32, int) ([]byte, error) { return nil, errors.New("no game") })
	s.EnableSaves(w, nil)

	_, c, _ := s.setupROM(findSetupGame("rowe"))
	if c.Status != "todo" {
		t.Fatalf("status = %q: %+v", c.Status, c)
	}
	for _, want := range []string{"Rioluwott", "Pokémon Emerald (USA, Europe)", "official patch", "ab2196855ac47598cb2ef7ebff475d6ed17b1992"} {
		if !strings.Contains(c.Detail, want) {
			t.Errorf("detail is missing %q: %s", want, c.Detail)
		}
	}
	if !strings.HasPrefix(c.Link, "https://www.pokecommunity.com/") || c.LinkText == "" {
		t.Errorf("link = %q %q", c.Link, c.LinkText)
	}
}
