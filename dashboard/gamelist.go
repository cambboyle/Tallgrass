package dashboard

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"tallgrass/gamelist"
)

// EnableGamelists lets the Library page name recognised games in
// EmulationStation's menus (dir: ~/.emulationstation/gamelists).
func (s *Server) EnableGamelists(dir string) {
	s.esGamelists = dir
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

// handleGamelist: GET shows what would be named; POST (JSON) writes it.
func (s *Server) handleGamelist(w http.ResponseWriter, r *http.Request) {
	if s.esGamelists == "" || s.saves == nil {
		http.Error(w, "no EmulationStation gamelists folder", http.StatusNotFound)
		return
	}
	games := s.gamelistGames()
	running := gamelist.Running("/proc")
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"games": games, "emulationStationRunning": running})
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	if running {
		http.Error(w, "EmulationStation is running, and it rewrites its game lists when it quits. "+
			"On the Pi, open the Start menu → Quit → Quit EmulationStation, then try again.", http.StatusConflict)
		return
	}
	bySystem := map[string][]gamelist.Game{}
	for _, g := range games {
		bySystem[g.System] = append(bySystem[g.System], gamelist.Game{Path: "./" + g.File, Name: g.Name,
			Desc: g.Name + ". Tallgrass follows this game live: your party, the Pokémon on each route, your Pokédex, your journey and your PC boxes."})
	}
	var results []gamelist.Result
	for sys, list := range bySystem {
		res, err := gamelist.Apply(filepath.Join(s.esGamelists, sys, "gamelist.xml"), list)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		res.File = strings.TrimPrefix(res.File, s.esGamelists+"/")
		results = append(results, res)
	}
	slices.SortFunc(results, func(a, b gamelist.Result) int { return strings.Compare(a.File, b.File) })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
