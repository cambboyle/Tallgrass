package dashboard

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"tallgrass/gamelist"
)

// EnableGamelists lets the Library page name recognised games in
// EmulationStation's menus (dir: ~/.emulationstation/gamelists).
// EmulationStation rewrites its lists when it quits, so names are only
// written while it isn't running: once asked, Tallgrass keeps checking
// (every 10 s) and applies them at the first chance, such as during a
// restart, and again whenever they've been undone.
func (s *Server) EnableGamelists(dir string) {
	s.esGamelists = dir
	if dir == "" {
		return
	}
	go func() {
		// Check straight away too: at boot Tallgrass starts a few seconds
		// before EmulationStation (measured on a Pi 4: 18 s vs 25 s), the
		// only time names written then show without another restart.
		tick := time.Tick(10 * time.Second)
		for first := true; ; first = false {
			if !first {
				<-tick
			}
			if s.gamelistWanted() && !gamelist.Running("/proc") {
				if results, err := s.applyGamelists(); err != nil {
					log.Printf("updating EmulationStation game names: %v", err)
				} else {
					for _, r := range results {
						if n := len(r.Renamed) + len(r.Added); n > 0 {
							log.Printf("named %d games in %s", n, r.File)
						}
					}
				}
			}
		}
	}()
}

// gamelistWanted reports whether the player asked for game names (kept as
// a setting, so it survives restarts).
func (s *Server) gamelistWanted() bool {
	s.mu.Lock()
	wanted, store := s.gamelistOn, s.store
	s.mu.Unlock()
	if wanted || store == nil {
		return wanted
	}
	v, _ := store.Setting("", "gamelist.names")
	return v == "on"
}

func (s *Server) wantGamelists() {
	s.mu.Lock()
	s.gamelistOn = true
	store := s.store
	s.mu.Unlock()
	if store != nil {
		if err := store.SetSetting("", "gamelist.names", "on"); err != nil {
			log.Printf("saving the game names setting: %v", err)
		}
	}
}

// GamelistGame is one game the Library page offers to name.
type GamelistGame struct {
	System string `json:"system"`
	File   string `json:"file"`
	Name   string `json:"name"`
}

// gamelistGames lists the ROMs Tallgrass recognises, with the name to give
// each: a focus game's name (with its version), or a hand-made profile's.
// Auto-detected profiles and unknown ROMs aren't renamed.
func (s *Server) gamelistGames() []GamelistGame {
	var out []GamelistGame
	if s.saves == nil {
		return out
	}
	for _, rom := range s.saves.ROMs() {
		_, sum, err := s.cachedHash(rom.Path)
		if err != nil || sum == "" {
			continue
		}
		name := ""
		for _, g := range setupGames {
			if slices.Contains(g.SHA1, sum) {
				name = g.Name
				if g.Version != "" {
					name = g.Version
				}
			}
		}
		if name == "" {
			if p := s.profileFor(sum, rom.Path); p != nil && !slices.Contains(p.Verified, "auto") {
				name = p.Name
			}
		}
		if name != "" {
			out = append(out, GamelistGame{System: rom.System, File: filepath.Base(rom.Path), Name: name})
		}
	}
	return out
}

// handleGamelist: GET shows what would be named; POST (JSON) asks for it:
// written now if EmulationStation isn't running, otherwise as soon as it
// isn't.
func (s *Server) handleGamelist(w http.ResponseWriter, r *http.Request) {
	if s.esGamelists == "" || s.saves == nil {
		http.Error(w, "no EmulationStation gamelists folder", http.StatusNotFound)
		return
	}
	running := gamelist.Running("/proc")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(map[string]any{"games": s.gamelistGames(), "emulationStationRunning": running,
			"on": s.gamelistWanted()})
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	s.wantGamelists()
	if running {
		json.NewEncoder(w).Encode(map[string]any{"pending": true})
		return
	}
	results, err := s.applyGamelists()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"results": results})
}

// applyGamelists writes the names into each system's gamelist.xml.
func (s *Server) applyGamelists() ([]gamelist.Result, error) {
	bySystem := map[string][]gamelist.Game{}
	for _, g := range s.gamelistGames() {
		bySystem[g.System] = append(bySystem[g.System], gamelist.Game{Path: "./" + g.File, Name: g.Name,
			Desc: g.Name + ". Tallgrass follows this game live: your party, the Pokémon on each route, your Pokédex, your journey and your PC boxes."})
	}
	results := []gamelist.Result{}
	for sys, list := range bySystem {
		res, err := gamelist.Apply(filepath.Join(s.esGamelists, sys, "gamelist.xml"), list)
		if err != nil {
			return nil, err
		}
		res.File = strings.TrimPrefix(res.File, s.esGamelists+"/")
		results = append(results, res)
	}
	slices.SortFunc(results, func(a, b gamelist.Result) int { return strings.Compare(a.File, b.File) })
	return results, nil
}
