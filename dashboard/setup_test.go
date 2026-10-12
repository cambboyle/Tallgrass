package dashboard

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"tallgrass/profile"
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
