package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"tallgrass/gen3"
	"tallgrass/gen4"
)

// SpeciesPage is a species' data plus its sprite.
type SpeciesPage struct {
	*gen3.SpeciesInfo
	Sprite string `json:"sprite,omitempty"`
}

// handleSpecies serves ?rom=<ROM id>&species=N, for any ROM the dashboard
// has opened (or the one being played).
func (s *Server) handleSpecies(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("rom")
	species, err := strconv.Atoi(r.URL.Query().Get("species"))
	if err != nil || species <= 0 {
		http.Error(w, "bad species", http.StatusBadRequest)
		return
	}
	page, err := s.speciesPage(id, species)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}

func (s *Server) speciesPage(id string, species int) (*SpeciesPage, error) {
	s.openRunningROM(id)

	s.ds.mu.Lock()
	d := s.ds.byID[id]
	s.ds.mu.Unlock()
	if d != nil {
		info, err := d.SpeciesInfo(species)
		if err != nil {
			return nil, err
		}
		idx := s.dsIndexes(d)
		info.EvolvesFrom = idx.evolvesFrom[species]
		if info.EvolvesFrom == nil {
			info.EvolvesFrom = []gen3.Evolution{}
		}
		if idx.foundIn != nil { // nil: this game's maps can't be read yet (Platinum)
			info.FoundIn = idx.foundIn[species]
			if info.FoundIn == nil {
				info.FoundIn = []gen3.Found{}
			}
		}
		return &SpeciesPage{SpeciesInfo: info, Sprite: s.dsSpriteURL(d, &gen3.Pokemon{Species: species})}, nil
	}

	s.romCache.mu.Lock()
	rc := s.romCache.byID[id]
	s.romCache.mu.Unlock()
	if rc == nil {
		return nil, errors.New("unknown ROM")
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.species == nil {
		t, err := speciesTables(rc.prof, rc.rom.Read)
		if err != nil {
			return nil, err
		}
		t.AreaName = rc.sectionName
		rc.species = t
	}
	info, err := rc.species.Species(rc.rom.Read, rc.names, species)
	if err != nil {
		return nil, err
	}
	rc.buildIndexes()
	info.EvolvesFrom, info.FoundIn = rc.evolvesFrom[species], rc.foundIn[species]
	if info.EvolvesFrom == nil {
		info.EvolvesFrom = []gen3.Evolution{}
	}
	if info.FoundIn == nil {
		info.FoundIn = []gen3.Found{}
	}
	return &SpeciesPage{SpeciesInfo: info, Sprite: rc.spriteURL(species, false)}, nil
}

// dsIndexes builds a DS ROM's whole-ROM lookups once. Failures are logged
// and leave that part empty (nil).
func (s *Server) dsIndexes(d *gen4.Data) *dsIndexes {
	s.ds.mu.Lock()
	idx, ok := s.ds.indexes[d]
	s.ds.mu.Unlock()
	if ok {
		return idx
	}
	idx = &dsIndexes{}
	var err error
	if idx.evolvesFrom, err = d.EvolvesFrom(); err != nil {
		log.Printf("DS evolves-from index: %v", err)
	}
	if d.Layout.Encounters != "" && d.Layout.MapHeaders != 0 {
		if idx.foundIn, err = d.FoundIn(); err != nil {
			log.Printf("DS where-to-find index: %v", err)
		}
	}
	if idx.trainers, err = d.Trainers(); err != nil {
		log.Printf("DS trainers: %v", err)
	}
	if maps, err := d.Maps(); err != nil {
		log.Printf("DS maps: %v", err)
	} else {
		idx.placeDS(d, maps)
	}
	s.ds.mu.Lock()
	defer s.ds.mu.Unlock()
	if s.ds.indexes == nil {
		s.ds.indexes = map[*gen4.Data]*dsIndexes{}
	}
	if existing, ok := s.ds.indexes[d]; ok {
		return existing // built by another request meanwhile
	}
	s.ds.indexes[d] = idx
	return idx
}

// buildIndexes builds the evolves-from and where-to-find indexes once per
// ROM. Failures are logged and leave those sections empty.
func (rc *romContext) buildIndexes() {
	if rc.evolvesFrom != nil {
		return
	}
	rc.evolvesFrom, rc.foundIn = map[int][]gen3.Evolution{}, map[int][]gen3.Found{}
	read := rc.rom.Read

	maxSpecies := rc.speciesCount()
	if idx, err := rc.species.EvolvesFrom(read, rc.names, maxSpecies); err == nil {
		rc.evolvesFrom = idx
	} else {
		log.Printf("%s: evolves-from index: %v", rc.prof.Name, err)
	}

	p := rc.prof
	if p.WildEncounters == nil || p.Location == nil || p.Location.MapGroups.IsZero() {
		return
	}
	wild, err := newWild(read, p.WildEncounters)
	if err != nil {
		return
	}
	area := rc.areaNamer()
	if area == nil {
		return
	}
	name := func(g, n uint8) string { a, _ := area(g, n); return a }
	if idx, err := wild.FoundIn(name); err == nil {
		rc.foundIn = idx
	} else {
		log.Printf("%s: where-to-find index: %v", p.Name, err)
	}
}

// speciesCount is one past the last species ID: from the profile, or
// where names run out (or turn to junk). Call with rc.mu held.
func (rc *romContext) speciesCount() int {
	if n := rc.prof.SpeciesData.SpeciesCount; n != 0 {
		return n
	}
	if rc.maxSpecies == 0 {
		rc.maxSpecies = 1
		for sp := 1; sp < 2048; sp++ {
			if n, err := rc.names.Species(sp); err == nil && n != "" && !strings.Contains(n, "?") {
				rc.maxSpecies = sp + 1
			}
		}
	}
	return rc.maxSpecies
}

// sectionName names a region map section ("Route 1"), or "". Call with
// rc.mu held.
func (rc *romContext) sectionName(sec int) string {
	l := rc.prof.Location
	if l == nil || sec < l.FirstMapSection {
		return ""
	}
	stride, offset := l.MapNameTable()
	name, _ := rc.names.MapName(rc.mapNames, stride, offset, l.FirstMapSection, sec)
	return name
}

// areaNamer names maps by their region map section ("Route 1"), also
// returning the section number (the game's own map order, -1 if unknown),
// or is nil if the profile can't. Call with rc.mu held.
func (rc *romContext) areaNamer() func(g, n uint8) (string, int) {
	p, read := rc.prof, rc.rom.Read
	if p.Location == nil || p.Location.MapGroups.IsZero() {
		return nil
	}
	groups, err := p.Location.MapGroups.Resolve(read)
	if err != nil {
		return nil
	}
	section := gen3.MapSections(read, groups)
	stride, offset := p.Location.MapNameTable()
	return func(g, n uint8) (string, int) {
		sec, err := section(g, n)
		if err != nil || sec < p.Location.FirstMapSection {
			return fmt.Sprintf("Map %d.%d", g, n), -1
		}
		name, err := rc.names.MapName(rc.mapNames, stride, offset, p.Location.FirstMapSection, sec)
		if err != nil || name == "" {
			return fmt.Sprintf("Map %d.%d", g, n), -1
		}
		return name, sec
	}
}

// openRunningROM makes sure the game being played has its ROM data open,
// so species pages work from the live party before any save is viewed.
func (s *Server) openRunningROM(id string) {
	s.mu.Lock()
	running, info := s.running, s.info
	s.mu.Unlock()
	if running == nil || info == nil || len(info.SHA1) < 12 || info.SHA1[:12] != id {
		return
	}
	if isDSROM(running.ROM) {
		s.dsData(running.ROM)
	} else {
		s.romAt(running.ROM)
	}
}
