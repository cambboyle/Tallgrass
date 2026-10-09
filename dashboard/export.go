package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"tallgrass/gen3"
	"tallgrass/profile"
)

// GameData is everything we know about one ROM, as plain facts for the
// docs site: no sprites, no text beyond names. Bump ExportVersion when a
// field changes meaning or goes away; adding fields doesn't need it.
// docs/gamedata-export.md describes the format.
type GameData struct {
	Schema      string             `json:"schema"`  // always "tallgrass.gamedata"
	Version     int                `json:"version"` // ExportVersion
	GeneratedAt time.Time          `json:"generatedAt"`
	ROM         ExportROM          `json:"rom"`
	Species     []gen3.SpeciesInfo `json:"species"`
	Trainers    []gen3.Trainer     `json:"trainers"`
	Maps        []*MapView         `json:"maps"` // empty when this game's maps can't be read yet
}

type ExportROM struct {
	SHA1       string `json:"sha1"`
	File       string `json:"file"`
	Game       string `json:"game"` // profile name, or the DS game family
	GameCode   string `json:"gameCode,omitempty"`
	Generation int    `json:"generation"`
}

const ExportVersion = 1

// handleExport serves GET /api/export?rom=<id> as a download.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("rom")
	gd, err := s.Export(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	name := fmt.Sprintf("%s-%s.json", fileSafe(gd.ROM.Game), id)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	enc.Encode(gd)
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func fileSafe(s string) string {
	return strings.Trim(unsafeChars.ReplaceAllString(s, "-"), "-")
}

// Export gathers a ROM's game data by its ID (SHA-1 prefix).
func (s *Server) Export(id string) (*GameData, error) {
	if id == "" {
		return nil, errors.New("which ROM? pass rom=<id>")
	}
	s.openROM(id)
	gd := &GameData{Schema: "tallgrass.gamedata", Version: ExportVersion, GeneratedAt: time.Now().UTC(),
		Species: []gen3.SpeciesInfo{}, Trainers: []gen3.Trainer{}, Maps: []*MapView{}}

	s.ds.mu.Lock()
	d := s.ds.byID[id]
	s.ds.mu.Unlock()
	if d != nil {
		gd.ROM = ExportROM{Game: d.Layout.Family, GameCode: d.GameCode(), Generation: 4}
		for sp := 1; sp < d.SpeciesCount(); sp++ {
			info, err := s.speciesPage(id, sp)
			if err != nil || !realSpeciesName(info.Name) {
				continue
			}
			gd.Species = append(gd.Species, *info.SpeciesInfo)
		}
	} else {
		s.romCache.mu.Lock()
		rc := s.romCache.byID[id]
		s.romCache.mu.Unlock()
		if rc == nil {
			return nil, errors.New("unknown ROM: it must be in the Library or running")
		}
		rc.mu.Lock()
		count, prof := rc.speciesCount(), rc.prof
		rc.mu.Unlock()
		gd.ROM = ExportROM{Game: prof.Name, Generation: prof.Generation}
		for sp := 1; sp < count; sp++ {
			info, err := s.speciesPage(id, sp)
			if err != nil || !realSpeciesName(info.Name) {
				continue
			}
			gd.Species = append(gd.Species, *info.SpeciesInfo)
		}
	}
	if t, err := s.trainers(id); err == nil {
		gd.Trainers = t.Trainers
	}
	gd.Maps = s.allMaps(id)
	if path := s.romPath(id); path != "" {
		gd.ROM.File = filepath.Base(path)
		if _, sum, err := s.cachedHash(path); err == nil {
			gd.ROM.SHA1 = sum
		}
	}
	return gd, nil
}

// allMaps lists a ROM's maps in group and number order.
func (s *Server) allMaps(id string) []*MapView {
	out := []*MapView{}
	s.ds.mu.Lock()
	d := s.ds.byID[id]
	s.ds.mu.Unlock()
	if d != nil {
		for _, m := range s.dsIndexes(d).maps {
			out = append(out, m)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].raw.Num < out[j].raw.Num })
		return out
	}
	s.romCache.mu.Lock()
	rc := s.romCache.byID[id]
	s.romCache.mu.Unlock()
	if rc == nil {
		return out
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.buildMaps()
	for _, m := range rc.maps {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].raw, out[j].raw
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		return a.Num < b.Num
	})
	return out
}

// realSpeciesName rejects unused slots: blank, "-----", "??????" and the
// like.
func realSpeciesName(n string) bool {
	return strings.Trim(n, "-? ") != "" && !strings.Contains(n, "?")
}

// romPath finds a ROM's file by ID: the running game or a Library ROM.
func (s *Server) romPath(id string) string {
	s.mu.Lock()
	running, info := s.running, s.info
	s.mu.Unlock()
	if running != nil && info != nil && strings.HasPrefix(info.SHA1, id) {
		return running.ROM
	}
	if s.saves == nil {
		return ""
	}
	for _, rom := range s.saves.ROMs() {
		if _, sum, err := s.cachedHash(rom.Path); err == nil && strings.HasPrefix(sum, id) {
			return rom.Path
		}
	}
	return ""
}

// ExportFile exports a ROM file directly, for the -export flag.
func (s *Server) ExportFile(path string, profiles []*profile.Profile) (*GameData, error) {
	s.profiles = profiles
	sum, err := s.hashROM(path)
	if err != nil {
		return nil, err
	}
	if isDSROM(path) {
		if _, err := s.dsData(path); err != nil {
			return nil, err
		}
	} else if rc, _, err := s.romAt(path); err != nil {
		return nil, err
	} else if rc == nil {
		return nil, fmt.Errorf("%s isn't a game we can read yet", filepath.Base(path))
	}
	gd, err := s.Export(sum[:12])
	if err != nil {
		return nil, err
	}
	gd.ROM.File, gd.ROM.SHA1 = filepath.Base(path), sum
	return gd, nil
}
