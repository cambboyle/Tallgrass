package dashboard

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"tallgrass/gen3"
	"tallgrass/tracker"
)

// EnableTracking turns on the Nuzlocke event log, stored in store. The
// profile needs a trainer section to tell save files apart.
func (s *Server) EnableTracking(store *tracker.Store) {
	s.engine = tracker.NewEngine(store, s.prof.Name, s.nationalName)
}

// track feeds the current state to the event engine.
func (s *Server) track(st State, caught []byte) error {
	if !st.Connected || s.prof.Trainer == nil {
		return nil
	}

	nameAddr, err := s.prof.Trainer.Name.Resolve(s.read)
	if err != nil {
		return err
	}
	name, err := s.read(nameAddr, 8)
	if err != nil {
		return err
	}
	idAddr, err := s.prof.Trainer.ID.Resolve(s.read)
	if err != nil {
		return err
	}
	id, err := s.read(idAddr, 4)
	if err != nil {
		return err
	}

	obs := tracker.Observation{
		At:         time.Now(),
		TrainerID:  binary.LittleEndian.Uint32(id),
		PlayerName: gen3.DecodeText(name),
		Section:    -1,
	}
	if st.Location != nil {
		obs.Section, obs.Area = st.Location.Section, st.Location.Name
	}

	for _, m := range st.Party {
		if m == nil {
			continue
		}
		p := tracker.PartyMon{
			Personality: m.Personality, Species: m.Species, SpeciesName: m.SpeciesName, Nickname: m.Nickname,
			Level: m.Level, CurrentHP: m.CurrentHP, IsEgg: m.IsEgg, MetSection: m.MetLocation,
		}
		p.MetArea, _ = s.mapName(m.MetLocation)
		if s.national != nil {
			p.National, _ = s.national.Number(m.Species)
		}
		obs.Party = append(obs.Party, p)
	}

	if time.Since(s.lastPC) >= pcReadEvery {
		s.lastPC = time.Now()
		boxes, err := s.pcMons()
		if err != nil {
			log.Printf("reading the PC: %v", err)
		}
		obs.Boxes = boxes
	}

	if caught != nil {
		obs.DexCaught = []int{}
		for n := 1; n <= len(caught)*8; n++ {
			if gen3.HasFlag(caught, n) {
				obs.DexCaught = append(obs.DexCaught, n)
			}
		}
	}
	return s.engine.Observe(obs)
}

// nationalName finds the species and name for a national dex number.
func (s *Server) nationalName(national int) (int, string) {
	if s.national == nil {
		return 0, ""
	}
	species, err := s.national.Species(national)
	if err != nil || species == 0 {
		return 0, ""
	}
	name, _ := s.names.Species(species)
	return species, name
}

// handleNuzlocke takes {"enabled": true|false} for the current game. JSON
// only, so another website can't flip it with a plain form post.
func (s *Server) handleNuzlocke(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		http.Error(w, "tracking is off", http.StatusNotFound)
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	var req struct{ Enabled bool }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.pendingNuzlocke = &req.Enabled
	s.mu.Unlock()
	w.WriteHeader(http.StatusAccepted) // applied on the next poll, then pushed
}

func (s *Server) applySettings() {
	s.mu.Lock()
	on := s.pendingNuzlocke
	s.pendingNuzlocke = nil
	s.mu.Unlock()
	if on == nil {
		return
	}
	if err := s.engine.SetNuzlocke(*on); err != nil {
		log.Printf("saving Nuzlocke setting: %v", err)
	}
}

// pcReadEvery spaces out PC reads: the PC is ~33 KB, several RetroArch
// commands, and it changes far less often than the party.
const pcReadEvery = 10 * time.Second

// pcMons reads every readable PC box from RAM; nil if the profile doesn't
// say how.
func (s *Server) pcMons() ([]tracker.PartyMon, error) {
	sf := s.prof.SaveFile
	if sf == nil || sf.RAM == nil || s.prof.Experience == nil || s.national == nil {
		return nil, nil
	}
	if s.levels == nil {
		exp := s.prof.Experience
		base, err := exp.BaseStats.Resolve(s.read)
		if err != nil {
			return nil, err
		}
		table, err := exp.Table.Resolve(s.read)
		if err != nil {
			return nil, err
		}
		s.levels = gen3.NewLevels(s.read, base, table, exp.RowLength)
	}

	addr, err := sf.RAM.Storage.Resolve(s.read)
	if err != nil {
		return nil, err
	}
	format := gen3.BoxFormat{Size: sf.BoxMonSize, Encrypted: s.prof.EncryptedData}
	storage, err := s.read(addr, 4+sf.Boxes*gen3.BoxSlots*format.Size)
	if err != nil {
		return nil, err
	}
	pc := &gen3.Save{Storage: storage}

	mons := []tracker.PartyMon{}
	for b := 0; b < sf.Boxes; b++ {
		box, err := pc.Box(b, format)
		if err != nil {
			return nil, err
		}
		for _, p := range box {
			if p == nil {
				continue
			}
			m := tracker.PartyMon{Personality: p.Personality, Species: p.Species, Nickname: p.Nickname,
				IsEgg: p.IsEgg, MetSection: p.MetLocation}
			m.SpeciesName, _ = s.names.Species(p.Species)
			m.National, _ = s.national.Number(p.Species)
			m.MetArea, _ = s.mapName(p.MetLocation)
			if m.Level, err = s.levels.Level(p.Species, p.Experience); err != nil {
				return nil, err
			}
			mons = append(mons, m)
		}
	}
	return mons, nil
}
