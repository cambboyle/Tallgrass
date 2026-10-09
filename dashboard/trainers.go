package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"tallgrass/gen3"
)

// TrainersView is every trainer battle in a game, for the Trainers page and
// the Route section.
type TrainersView struct {
	ROM    string `json:"rom"`
	Game   string `json:"game"`
	Placed bool   `json:"placed"` // trainers have areas (Gen 3 games whose maps we can read)
	// AreaOrder ranks areas in the game's own map order (towns, then
	// routes, then dungeons), for grouping.
	AreaOrder map[string]int `json:"areaOrder,omitempty"`
	Trainers  []gen3.Trainer `json:"trainers"`
	Sprites   map[int]string `json:"sprites"` // species -> sprite URL
}

// handleTrainers serves GET /api/trainers?rom=<id>[&map=<group>.<num>].
// Without rom it uses the running game; map keeps only that map's trainers.
func (s *Server) handleTrainers(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("rom")
	if id == "" {
		s.mu.Lock()
		if s.info != nil && len(s.info.SHA1) >= 12 {
			id = s.info.SHA1[:12]
		}
		s.mu.Unlock()
	}
	if id == "" {
		http.Error(w, "No game is running. Pick one from the Library.", http.StatusNotFound)
		return
	}
	v, err := s.trainers(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if m := r.URL.Query().Get("map"); m != "" {
		filtered := *v
		filtered.Trainers = []gen3.Trainer{}
		for _, t := range v.Trainers {
			if slices.Contains(t.Maps, m) {
				filtered.Trainers = append(filtered.Trainers, t)
			}
		}
		v = &filtered
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// trainers reads (once) and returns a ROM's trainers.
func (s *Server) trainers(id string) (*TrainersView, error) {
	s.openROM(id)

	s.ds.mu.Lock()
	d := s.ds.byID[id]
	s.ds.mu.Unlock()
	if d != nil {
		idx := s.dsIndexes(d)
		if idx.trainers == nil {
			return nil, errors.New("couldn't read this game's trainers")
		}
		v := &TrainersView{ROM: id, Game: s.romName(id), Trainers: idx.trainers, Sprites: map[int]string{},
			Placed: len(idx.areaOrder) > 0, AreaOrder: idx.areaOrder}
		for _, t := range idx.trainers {
			for _, m := range t.Party {
				if _, ok := v.Sprites[m.Species]; !ok {
					v.Sprites[m.Species] = s.dsSpriteURL(d, &gen3.Pokemon{Species: m.Species})
				}
			}
		}
		return v, nil
	}

	s.romCache.mu.Lock()
	rc := s.romCache.byID[id]
	s.romCache.mu.Unlock()
	if rc == nil {
		return nil, errors.New("unknown ROM: open it from the Library first")
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.trainers == nil {
		if err := rc.buildTrainers(); err != nil {
			return nil, err
		}
	}
	v := &TrainersView{ROM: id, Game: rc.prof.Name, Placed: rc.trainersPlaced, AreaOrder: rc.areaOrder,
		Trainers: rc.trainers, Sprites: map[int]string{}}
	for _, t := range rc.trainers {
		for _, m := range t.Party {
			if _, ok := v.Sprites[m.Species]; !ok {
				v.Sprites[m.Species] = rc.spriteURL(m.Species, false)
			}
		}
	}
	return v, nil
}

// buildTrainers reads every trainer and where they stand. Call with rc.mu
// held.
func (rc *romContext) buildTrainers() error {
	td := rc.prof.Trainers
	if td == nil {
		return errors.New("this game's profile has no trainer data")
	}
	read := rc.rom.Read
	table, err := td.Table.Resolve(read)
	if err != nil {
		return err
	}
	classes, err := td.ClassNames.Resolve(read)
	if err != nil {
		return err
	}
	list, err := gen3.TrainerTables{Trainers: table, ClassNames: classes, ClassNameLen: td.ClassNameLength}.All(read, rc.names)
	if err != nil {
		return err
	}

	rc.areaOrder = map[string]int{}
	rc.buildMaps()
	placed := map[int][]*MapView{}
	for _, m := range rc.maps {
		for _, id := range m.Trainers {
			placed[id] = append(placed[id], m)
		}
	}
	for i := range list {
		for _, m := range placed[list[i].ID] {
			list[i].Maps = append(list[i].Maps, m.ID)
			if !slices.Contains(list[i].Areas, m.Area) {
				list[i].Areas = append(list[i].Areas, m.Area)
			}
			if _, ok := rc.areaOrder[m.Area]; !ok && m.section >= 0 {
				rc.areaOrder[m.Area] = m.section
			}
		}
	}
	rc.trainersPlaced = len(placed) > 0
	rc.trainers = list
	return nil
}

// openROM makes a ROM's data available by its ID: the running game's, or
// any ROM in the Library.
func (s *Server) openROM(id string) {
	s.openRunningROM(id)
	if s.saves == nil || s.romOpen(id) {
		return
	}
	for _, rom := range s.saves.ROMs() {
		if _, sum, err := s.cachedHash(rom.Path); err != nil || len(sum) < 12 || sum[:12] != id {
			continue
		}
		if isDSROM(rom.Path) {
			s.dsData(rom.Path)
		} else {
			s.romAt(rom.Path)
		}
		return
	}
}

func (s *Server) romOpen(id string) bool {
	s.ds.mu.Lock()
	_, ds := s.ds.byID[id]
	s.ds.mu.Unlock()
	s.romCache.mu.Lock()
	_, gba := s.romCache.byID[id]
	s.romCache.mu.Unlock()
	return ds || gba
}

// romName is a ROM's file name without extension, for page titles.
func (s *Server) romName(id string) string {
	if s.saves == nil {
		return ""
	}
	for _, rom := range s.saves.ROMs() {
		if _, sum, err := s.cachedHash(rom.Path); err == nil && len(sum) >= 12 && sum[:12] == id {
			return strings.TrimSuffix(filepath.Base(rom.Path), filepath.Ext(rom.Path))
		}
	}
	return ""
}
