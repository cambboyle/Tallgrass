package dashboard

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// The setup wizard prepares one game: the player picks it, and we check
// (and where we can, fix) everything that game needs, then confirm a live
// read. Only the three focus games are offered.

type setupGame struct {
	Slug     string   `json:"slug"`
	Name     string   `json:"name"`
	System   string   `json:"system"` // ROM folder: "gba" or "nds"
	Emulator string   `json:"emulator"`
	SHA1     []string `json:"-"`
	Version  string   `json:"version,omitempty"` // the release we support, when we know its name
}

var setupGames = []setupGame{
	{Slug: "radical-red", Name: "Pokémon Radical Red", System: "gba", Emulator: "retroarch",
		SHA1: []string{"964f951a0fdaf209e4ea1344883ef0d557bb3a80"}},
	{Slug: "unbound", Name: "Pokémon Unbound", System: "gba", Emulator: "retroarch",
		SHA1: []string{"b4776b82a4c7915d0fadeaa27e013523f99dfd94"}},
	{Slug: "heartgold-generations", Name: "HeartGold Generations", System: "nds", Emulator: "drastic",
		SHA1: []string{"d8fcc9de74ecc7e38caed8be883bd602653c3736"}, Version: "HeartGold Generations 2.0"},
}

// setupPaths are the RetroPie locations the wizard checks and edits.
type setupPaths struct {
	Configs string // /opt/retropie/configs
	DraStic string // /opt/retropie/emulators/drastic
	Proc    string // /proc
}

type setupState struct {
	paths setupPaths
	mu    sync.Mutex
	game  string // chosen game's slug; kept in the settings store too
}

// EnableSetup turns on the setup wizard.
func (s *Server) EnableSetup(configs, drastic string) {
	s.setup = &setupState{paths: setupPaths{Configs: configs, DraStic: drastic, Proc: "/proc"}}
	if s.store != nil {
		if g, err := s.store.Setting("", "setup.game"); err == nil {
			s.setup.game = g
		}
	}
}

// SetupCheck is one step of the wizard.
type SetupCheck struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"` // "ok", "todo", "wait" or "fail"
	Detail   string `json:"detail"`
	Fix      string `json:"fix,omitempty"` // action for POST /api/setup/fix
	FixLabel string `json:"fixLabel,omitempty"`
}

type SetupView struct {
	Games  []setupGame  `json:"games"`
	Chosen *setupGame   `json:"chosen,omitempty"`
	Checks []SetupCheck `json:"checks"`
	Done   bool         `json:"done"` // everything needed is in place and a live read worked
	ROMDir string       `json:"romDir,omitempty"`
}

func findSetupGame(slug string) *setupGame {
	for i := range setupGames {
		if setupGames[i].Slug == slug {
			return &setupGames[i]
		}
	}
	return nil
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if s.setup == nil {
		http.Error(w, "setup is off", http.StatusNotFound)
		return
	}
	s.setup.mu.Lock()
	slug := s.setup.game
	s.setup.mu.Unlock()
	v := SetupView{Games: setupGames, Checks: []SetupCheck{}}
	if g := findSetupGame(slug); g != nil {
		v.Chosen = g
		v.Checks, v.Done, v.ROMDir = s.setupChecks(g)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// handleSetupGame picks the game to set up. POST {"game": "<slug>"}
func (s *Server) handleSetupGame(w http.ResponseWriter, r *http.Request) {
	if s.setup == nil || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	var req struct{ Game string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Game != "" && findSetupGame(req.Game) == nil) {
		http.Error(w, "unknown game", http.StatusBadRequest)
		return
	}
	s.setup.mu.Lock()
	s.setup.game = req.Game
	s.setup.mu.Unlock()
	if s.store != nil {
		if err := s.store.SetSetting("", "setup.game", req.Game); err != nil {
			log.Printf("saving setup choice: %v", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// setupChecks works out each step for a game. The ROM step may report
// "wait" while ROMs are still being identified in the background.
func (s *Server) setupChecks(g *setupGame) (checks []SetupCheck, done bool, romDir string) {
	rom, romCheck, romDir := s.setupROM(g)
	checks = append(checks, romCheck)
	if g.Emulator == "retroarch" {
		checks = append(checks, s.setupRetroArch()...)
	} else {
		checks = append(checks, s.setupDraStic()...)
	}
	checks = append(checks, s.setupSave(g, rom))
	live := s.setupLive(g, rom)
	checks = append(checks, live)

	done = live.Status == "ok"
	for _, c := range checks {
		if c.ID != "save" && c.Status != "ok" {
			done = false
		}
	}
	return checks, done, romDir
}

// setupROM finds the game's ROM by SHA-1 in its system's ROM folder.
func (s *Server) setupROM(g *setupGame) (string, SetupCheck, string) {
	c := SetupCheck{ID: "rom", Title: "Find your copy of " + g.Name}
	if s.saves == nil {
		c.Status, c.Detail = "fail", "Tallgrass can't see a ROMs folder. Start it with -roms pointing at your ROMs."
		return "", c, ""
	}
	var dir string
	var others []string
	var unhashed []string
	for _, rom := range s.saves.ROMs() {
		if rom.System != g.System {
			continue
		}
		dir = filepath.Dir(rom.Path)
		_, sum, err := s.cachedHash(rom.Path)
		if err != nil {
			continue
		}
		if sum == "" {
			unhashed = append(unhashed, rom.Path)
			continue
		}
		if slices.Contains(g.SHA1, sum) {
			c.Status, c.Detail = "ok", "Found "+filepath.Base(rom.Path)+"."
			return rom.Path, c, dir
		}
		others = append(others, filepath.Base(rom.Path))
	}
	if dir == "" {
		dir = filepath.Join(s.romsRoot(), g.System)
	}
	if len(unhashed) > 0 {
		s.hashInBackground(unhashed)
		c.Status, c.Detail = "wait", fmt.Sprintf("Checking %d ROM files…", len(unhashed))
		return "", c, dir
	}
	c.Status = "todo"
	version := g.Version
	if version == "" {
		version = "the supported version"
	}
	c.Detail = fmt.Sprintf("Copy your %s ROM (%s) into %s. Tallgrass recognises the exact file by its SHA-1, %s.", g.Name, version, dir, g.SHA1[0])
	if len(others) > 0 {
		c.Detail += " Files there now that aren't it: " + strings.Join(others, ", ") + "."
	}
	return "", c, dir
}

// romsRoot is the ROMs folder the save watcher was given.
func (s *Server) romsRoot() string {
	for _, rom := range s.saves.ROMs() {
		return filepath.Dir(filepath.Dir(rom.Path))
	}
	return "your ROMs folder"
}

func (s *Server) retroarchConfigs() []string {
	// The GBA config overrides the shared one, so check it first.
	return []string{
		filepath.Join(s.setup.paths.Configs, "gba", "retroarch.cfg"),
		filepath.Join(s.setup.paths.Configs, "all", "retroarch.cfg"),
	}
}

func (s *Server) setupRetroArch() []SetupCheck {
	var checks []SetupCheck

	core := SetupCheck{ID: "core", Title: "GBA emulator: lr-mgba"}
	emus := filepath.Join(s.setup.paths.Configs, "gba", "emulators.cfg")
	switch def, err := readConfigValue(emus, "default"); {
	case errors.Is(err, os.ErrNotExist):
		core.Status, core.Detail = "fail", "Couldn't find "+emus+". Is this RetroPie?"
	case err != nil:
		core.Status, core.Detail = "fail", err.Error()
	case def == "lr-mgba":
		core.Status, core.Detail = "ok", "GBA games run in mGBA, which lets Tallgrass read the game's memory."
	default:
		core.Status = "todo"
		core.Detail = fmt.Sprintf("GBA games run in %s. Tallgrass needs lr-mgba to read the game's memory (install it from RetroPie-Setup if it's missing).", def)
		core.Fix, core.FixLabel = "gba-core", "Use lr-mgba for GBA"
	}
	checks = append(checks, core)

	net := SetupCheck{ID: "network", Title: "RetroArch network commands"}
	if v, file := s.retroarchValue("network_cmd_enable"); v == "true" {
		net.Status, net.Detail = "ok", "On ("+filepath.Base(filepath.Dir(file))+"/retroarch.cfg). Tallgrass reads the game through them."
	} else {
		net.Status, net.Detail = "todo", "Off. Tallgrass reads the running game through RetroArch's network commands (on this device only)."
		net.Fix, net.FixLabel = "retroarch-network", "Turn on"
	}
	checks = append(checks, net)

	auto := SetupCheck{ID: "autosave", Title: "Save to disk every minute"}
	if v, _ := s.retroarchValue("autosave_interval"); v != "" && v != "0" {
		auto.Status, auto.Detail = "ok", fmt.Sprintf("RetroArch writes your save every %s seconds, so backups stay current.", v)
	} else {
		auto.Status, auto.Detail = "todo", "RetroArch only writes your save when the game closes, so backups lag behind."
		auto.Fix, auto.FixLabel = "retroarch-autosave", "Save every minute"
	}
	checks = append(checks, auto)
	return checks
}

// retroarchValue is a setting's effective value: the GBA config wins over
// the shared one. It returns the file it came from.
func (s *Server) retroarchValue(key string) (string, string) {
	for _, file := range s.retroarchConfigs() {
		if v, err := readConfigValue(file, key); err == nil && v != "" {
			return v, file
		}
	}
	return "", ""
}

func (s *Server) setupDraStic() []SetupCheck {
	var checks []SetupCheck
	ds := SetupCheck{ID: "drastic", Title: "DS emulator: DraStic"}
	if _, err := os.Stat(filepath.Join(s.setup.paths.DraStic, "drastic")); err == nil {
		ds.Status, ds.Detail = "ok", "Installed."
	} else {
		ds.Status, ds.Detail = "todo", "Install DraStic from RetroPie-Setup (Manage packages → experimental → drastic)."
	}
	checks = append(checks, ds)

	mem := SetupCheck{ID: "memory", Title: "Permission to read DraStic's memory"}
	b, err := os.ReadFile(filepath.Join(s.setup.paths.Proc, "sys", "kernel", "yama", "ptrace_scope"))
	switch {
	case errors.Is(err, os.ErrNotExist) || (err == nil && strings.TrimSpace(string(b)) == "0"):
		mem.Status, mem.Detail = "ok", "Tallgrass can follow DS games live (it runs as the same user as DraStic)."
	case err != nil:
		mem.Status, mem.Detail = "fail", err.Error()
	default:
		mem.Status = "todo"
		mem.Detail = "This system stops programs reading each other's memory (ptrace_scope " + strings.TrimSpace(string(b)) +
			"), so DS games can't be followed live. Run: echo 0 | sudo tee /proc/sys/kernel/yama/ptrace_scope"
	}
	checks = append(checks, mem)
	return checks
}

// setupSave finds the game's save; for GBA it offers to turn a .sav from
// another emulator into the .srm RetroArch uses.
func (s *Server) setupSave(g *setupGame, rom string) SetupCheck {
	c := SetupCheck{ID: "save", Title: "Your save"}
	if rom == "" {
		c.Status, c.Detail = "wait", "Once the ROM is found."
		return c
	}
	saves := s.saves.SaveFor(rom)
	var srm, sav string
	for _, rel := range saves {
		switch strings.ToLower(filepath.Ext(rel)) {
		case ".srm", ".dsv":
			srm = rel
		case ".sav":
			sav = rel
		}
	}
	if srm != "" {
		data, err := s.saves.ReadCurrent(srm)
		if err == nil && !blank(data) {
			c.Status, c.Detail = "ok", "Found "+filepath.Base(srm)+". Every change is backed up."
			return c
		}
	}
	if g.System == "gba" && sav != "" {
		c.Status = "todo"
		c.Detail = filepath.Base(sav) + " looks like a save from another emulator. RetroArch reads " +
			strings.TrimSuffix(filepath.Base(sav), filepath.Ext(sav)) + ".srm instead."
		c.Fix, c.FixLabel = "copy-sav", "Use this save in RetroArch"
		return c
	}
	c.Status, c.Detail = "todo", "No save yet. Start a new game or load yours; Tallgrass picks it up after your first in-game save."
	return c
}

// blank reports whether a save file has never been written (all 0xFF or 0).
func blank(b []byte) bool {
	for _, c := range b {
		if c != 0xFF && c != 0 {
			return false
		}
	}
	return true
}

// setupLive is done when this game is running and its party has been read.
func (s *Server) setupLive(g *setupGame, rom string) SetupCheck {
	c := SetupCheck{ID: "live", Title: "Read the game live"}
	s.mu.Lock()
	running, connected := s.running, s.connected
	s.mu.Unlock()
	switch {
	case rom == "":
		c.Status, c.Detail = "wait", "Once the ROM is found."
	case running == nil || running.ROM != rom:
		c.Status, c.Detail = "todo", "Start "+g.Name+" from EmulationStation and load your save. This page notices by itself."
	case !connected:
		c.Status, c.Detail = "wait", "Game started. Waiting to read your party: load your save if you're on the title screen."
	default:
		c.Status, c.Detail = "ok", "Reading your party live. You're all set."
	}
	return c
}

// handleSetupFix applies one of the wizard's fixes. POST {"action": "..."}
func (s *Server) handleSetupFix(w http.ResponseWriter, r *http.Request) {
	if s.setup == nil || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	var req struct{ Action string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()
	if running != nil && strings.HasPrefix(req.Action, "retroarch") || running != nil && req.Action == "copy-sav" {
		http.Error(w, "Close the game first: RetroArch rewrites its files when it exits.", http.StatusConflict)
		return
	}
	var err error
	switch req.Action {
	case "retroarch-network":
		err = s.setRetroArch("network_cmd_enable", "true")
	case "retroarch-autosave":
		err = s.setRetroArch("autosave_interval", "60")
	case "gba-core":
		err = setConfigValue(filepath.Join(s.setup.paths.Configs, "gba", "emulators.cfg"), "default", "lr-mgba")
	case "copy-sav":
		err = s.copySav()
	default:
		http.Error(w, "unknown fix", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("setup: applied %s", req.Action)
	w.WriteHeader(http.StatusNoContent)
}

// setRetroArch sets a RetroArch option where it takes effect: in the GBA
// config if that file sets it already, otherwise in the shared config.
func (s *Server) setRetroArch(key, value string) error {
	files := s.retroarchConfigs()
	target := files[1]
	if v, err := readConfigValue(files[0], key); err == nil && v != "" {
		target = files[0]
	}
	return setConfigValue(target, key, value)
}

// copySav copies the chosen game's .sav over (or into) its .srm, backing up
// whatever .srm there was first.
func (s *Server) copySav() error {
	s.setup.mu.Lock()
	g := findSetupGame(s.setup.game)
	s.setup.mu.Unlock()
	if g == nil {
		return errors.New("pick a game first")
	}
	rom, check, _ := s.setupROM(g)
	if check.Status != "ok" {
		return errors.New("the ROM isn't found yet")
	}
	var sav string
	for _, rel := range s.saves.SaveFor(rom) {
		if strings.EqualFold(filepath.Ext(rel), ".sav") {
			sav = rel
		}
	}
	if sav == "" {
		return errors.New("no .sav file next to the ROM")
	}
	data, err := s.saves.ReadCurrent(sav)
	if err != nil {
		return err
	}
	// Some emulators append a real-time clock after the 128 KB save.
	if len(data) > 0x20000 {
		data = data[:0x20000]
	}
	srm := strings.TrimSuffix(rom, filepath.Ext(rom)) + ".srm"
	if old, err := os.ReadFile(srm); err == nil {
		backup := fmt.Sprintf("%s.before-tallgrass-%s", srm, time.Now().Format("20060102-150405"))
		if err := os.WriteFile(backup, old, 0o644); err != nil {
			return fmt.Errorf("backing up the current .srm: %w", err)
		}
	}
	return os.WriteFile(srm, data, 0o644)
}

var configLine = regexp.MustCompile(`^\s*([A-Za-z0-9_]+)\s*=\s*"?([^"]*)"?\s*$`)

// readConfigValue reads `key = "value"` from a RetroArch/RetroPie config
// file; "" if it isn't set.
func readConfigValue(file, key string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	value := ""
	for sc.Scan() {
		if m := configLine.FindStringSubmatch(sc.Text()); m != nil && m[1] == key {
			value = m[2] // the last one wins
		}
	}
	return value, sc.Err()
}

// setConfigValue sets `key = "value"` in a config file, replacing every
// existing line for the key (or appending one). The first time it touches a
// file it keeps the original as <file>.bak-tallgrass.
func setConfigValue(file, key, value string) error {
	old, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	backup := file + ".bak-tallgrass"
	if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(backup, old, 0o644); err != nil {
			return fmt.Errorf("backing up %s: %w", file, err)
		}
	}
	line := fmt.Sprintf("%s = %q", key, value)
	var out bytes.Buffer
	found := false
	for _, l := range strings.SplitAfter(string(old), "\n") {
		if m := configLine.FindStringSubmatch(strings.TrimRight(l, "\n")); m != nil && m[1] == key {
			if !found {
				out.WriteString(line + "\n")
				found = true
			}
			continue
		}
		out.WriteString(l)
	}
	if !found {
		if out.Len() > 0 && !bytes.HasSuffix(out.Bytes(), []byte("\n")) {
			out.WriteString("\n")
		}
		out.WriteString(line + "\n")
	}
	tmp := file + ".tmp-tallgrass"
	if err := os.WriteFile(tmp, out.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}
