package dashboard

import (
	"encoding/binary"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"time"

	"tallgrass/emulator"
	"tallgrass/gen3"
	"tallgrass/gen4"
	"tallgrass/tracker"
)

// dsLive follows a DS game in DraStic by reading the emulated DS's RAM from
// DraStic's process. Only the poller touches it.
type dsLive struct {
	pid       int
	mem       *emulator.ProcMem
	data      *gen4.Data
	general   uint32    // address of the live general save block; 0 = not found yet
	lastScan  time.Time // scans read 4 MB, so they're spaced out
	lastCheck time.Time // last check that the block is still valid
	misses    int       // failed reads in a row; many means the block moved
	party     []*Member // last good read, shown when a read catches the game mid-update
	trainer   string
	trainID   uint32

	locOffset  int           // where the player's location record is in the general block; -1 unknown
	dex        *gen4.Pokedex // where the Pokédex flags are; nil until found
	seen       []byte
	caught     []byte
	location   *Location
	encounters map[int][]gen3.PeriodEncounters // by wild encounter file
}

func (l *dsLive) close() {
	if l != nil && l.mem != nil {
		l.mem.Close()
	}
}

// canFollowDS reports whether a DraStic game can be read live: a Gen 4
// game whose main RAM DraStic maps at the DS's own address.
func canFollowDS(proc string, r *emulator.Running) bool {
	return r.Emulator == "drastic" && dsFamily(r.ROM) != "" &&
		emulator.Mapped(proc, r.PID, gen4.MainRAM, gen4.MainRAM+gen4.MainRAMSize)
}

// dsSnapshot reads the live party of the DS game running in DraStic.
func (s *Server) dsSnapshot(r *emulator.Running) State {
	st := State{Game: strings.TrimSuffix(filepath.Base(r.ROM), filepath.Ext(r.ROM)), Running: s.info, Party: []*Member{}}
	l, err := s.followDS(r)
	if err != nil {
		log.Printf("reading %s live: %v", st.Game, err)
		st.Error = "Can't read this DS game's memory: " + err.Error()
		return st
	}

	if l.general == 0 && time.Since(l.lastScan) > 2*time.Second {
		l.lastScan = time.Now()
		if ram, err := l.mem.Read(gen4.MainRAM, gen4.MainRAMSize); err == nil {
			if addr, err := gen4.FindGeneral(ram, gen4.MainRAM, l.data.Layout); err == nil {
				l.general = addr
				log.Printf("found %s's save data at %08X in DS RAM", st.Game, addr)
			}
		}
	}
	// The block found at startup can be a copy the game later reuses (it
	// loads the save into a buffer for the title screen), so check now and
	// then that it's still a valid general block.
	if l.general != 0 && time.Since(l.lastCheck) > 10*time.Second {
		l.lastCheck = time.Now()
		layout := l.data.Layout
		if b, err := l.mem.Read(l.general, layout.Party+6*gen4.PartySize); err == nil && !gen4.CheckGeneral(b, layout) {
			l.misses++
			if l.misses > 2 {
				log.Printf("%s's save data moved; looking for it again", st.Game)
				l.general, l.party, l.locOffset, l.misses = 0, nil, -1, 0
				l.forgetDex()
			}
		}
	}
	if l.general != 0 {
		name, id, party, err := readLive(l)
		if err == nil && len(party) == 0 {
			err = errors.New("empty party") // there's always at least one: this isn't the live block
		}
		if err == nil {
			l.misses, l.trainer, l.trainID = 0, name, id
			l.party = l.party[:0]
			for _, p := range party {
				m := dsMember(l.data, p)
				m.Sprite = s.dsSpriteURL(l.data, p)
				l.party = append(l.party, m)
			}
		} else if l.misses++; l.misses > 30 {
			l.general, l.party, l.locOffset = 0, nil, -1 // the game was probably reset: find the block again
			l.forgetDex()
		}
	}
	s.mu.Lock()
	s.dsAt.pid, s.dsAt.general = l.pid, l.general
	s.mu.Unlock()
	s.dsReadDex(l)
	if l.party == nil {
		st.Error = "Looking for your party in the game's memory…"
		return st
	}

	s.setConnected(true)
	st.Connected = true
	if loc, err := s.dsLocation(l); err != nil {
		log.Printf("reading %s's location: %v", st.Game, err)
	} else if loc != nil {
		l.location = loc
	}
	st.Location = l.location
	st.Party = append(st.Party, l.party...)
	for len(st.Party) < 6 {
		st.Party = append(st.Party, nil)
	}
	if s.engine != nil {
		if err := s.engine.Observe(dsObservation(l)); err != nil {
			log.Printf("tracking: %v", err)
		}
		s.applySettings()
		st.Run = s.engine.View()
	}
	return st
}

// followDS opens (or reuses) DraStic's memory and the game's ROM data.
func (s *Server) followDS(r *emulator.Running) (*dsLive, error) {
	if s.dsLive != nil && s.dsLive.pid == r.PID {
		return s.dsLive, nil
	}
	s.dsLive.close()
	s.dsLive = nil
	data, err := s.dsData(r.ROM)
	if err != nil {
		return nil, err
	}
	mem, err := emulator.OpenProcMem("/proc", r.PID)
	if err != nil {
		return nil, err
	}
	s.dsLive = &dsLive{pid: r.PID, mem: mem, data: data, locOffset: -1, encounters: map[int][]gen3.PeriodEncounters{}}
	return s.dsLive, nil
}

// readLive tries a few times, a frame apart: a read can land while the game
// is part-way through decrypting or re-encrypting a Pokémon.
func readLive(l *dsLive) (string, uint32, []*gen3.Pokemon, error) {
	var err error
	for try := 0; try < 3; try++ {
		var name string
		var id uint32
		var party []*gen3.Pokemon
		if name, id, party, err = gen4.ReadLiveParty(l.mem.Read, l.general, l.data.Layout); err == nil {
			return name, id, party, nil
		}
		time.Sleep(17 * time.Millisecond)
	}
	return "", 0, nil, err
}

func dsMember(d *gen4.Data, p *gen3.Pokemon) *Member {
	m := &Member{Pokemon: p, MoveNames: make([]string, len(p.Moves)),
		SpeciesName: gen4.Name(d.Species, p.Species), AbilityName: gen4.Name(d.Abilities, p.Ability)}
	if p.HeldItem != 0 {
		m.ItemName = gen4.Name(d.Items, p.HeldItem)
	}
	for i, mv := range p.Moves {
		m.MoveNames[i] = gen4.Name(d.Moves, mv.ID)
	}
	return m
}

// dsReadDex reads the Pokédex's caught and seen flags (finding them first).
func (s *Server) dsReadDex(l *dsLive) {
	if l.general == 0 {
		return
	}
	if l.dex == nil {
		g, err := l.mem.Read(l.general, 0x10000)
		if err != nil {
			return
		}
		dex, ok := gen4.FindPokedex(g, l.data.SpeciesCount())
		if !ok {
			return
		}
		l.dex = &dex
	}
	if b, err := l.mem.Read(l.general+uint32(l.dex.Caught), l.dex.Size); err == nil {
		l.caught = b
	}
	if b, err := l.mem.Read(l.general+uint32(l.dex.Seen), l.dex.Size); err == nil {
		l.seen = b
	}
}

// forgetDex drops the Pokédex's location and flags, which belong to a block
// that's no longer trusted.
func (l *dsLive) forgetDex() {
	l.dex, l.caught, l.seen = nil, nil, nil
}

func dsObservation(l *dsLive) tracker.Observation {
	o := tracker.Observation{At: time.Now(), TrainerID: l.trainID, PlayerName: l.trainer, Section: -1}
	// Dex flags only count when this poll's read of the block succeeded: a
	// block that's moving or being rewritten gives garbage flags, which
	// would be logged as catches.
	if l.caught != nil && l.misses == 0 {
		o.DexCaught = []int{}
		for sp := 1; sp < len(l.caught)*8; sp++ {
			if gen4.Has(l.caught, sp) {
				o.DexCaught = append(o.DexCaught, sp) // Gen 4 species IDs are national numbers
			}
		}
	}
	if l.location != nil {
		o.Section, o.Area = l.location.Section, l.location.Name
	}
	for _, m := range l.party {
		o.Party = append(o.Party, tracker.PartyMon{
			Personality: m.Personality, Species: m.Species, SpeciesName: m.SpeciesName,
			National: m.Species, // Gen 4 species IDs are national numbers
			Nickname: m.Nickname, Level: m.Level, CurrentHP: m.CurrentHP, IsEgg: m.IsEgg,
			MetSection: m.MetLocation, MetArea: gen4.Name(l.data.Locations, m.MetLocation),
		})
	}
	return o
}

// dsLocation reads the player's map and builds the Route section; nil (no
// error) when the game doesn't support it yet.
func (s *Server) dsLocation(l *dsLive) (*Location, error) {
	layout := l.data.Layout
	if layout.MapHeaders == 0 || l.general == 0 {
		return nil, nil
	}
	if l.locOffset < 0 {
		g, err := l.mem.Read(l.general, 0x4000)
		if err != nil {
			return nil, err
		}
		if l.locOffset = gen4.FindLocation(g); l.locOffset < 0 {
			return nil, nil
		}
	}
	rec, err := l.mem.Read(l.general+uint32(l.locOffset), 20)
	if err != nil {
		return nil, err
	}
	mapID := int(int32(binary.LittleEndian.Uint32(rec)))
	if mapID <= 0 || mapID >= 2000 {
		l.locOffset = -1 // look again next time
		return nil, nil
	}
	hb, err := l.mem.Read(layout.MapHeaders+uint32(mapID*24), 24)
	if err != nil {
		return nil, err
	}
	h := layout.MapHeader(hb)
	loc := &Location{MapNum: mapID, Section: h.Section, Name: gen4.Name(l.data.Locations, h.Section), Periods: []PeriodView{}}
	hour := time.Now().Hour() // DraStic's clock is the system clock
	loc.Hour = &hour
	if h.WildBank < 0 {
		return loc, nil
	}

	periods, ok := l.encounters[h.WildBank]
	if !ok {
		if periods, err = l.data.Encounters(h.WildBank); err != nil {
			return nil, err
		}
		l.encounters[h.WildBank] = periods
	}
	for _, p := range periods {
		pv := PeriodView{Name: p.Name, FromHour: p.FromHour, ToHour: p.ToHour, Active: p.Contains(hour)}
		for _, m := range p.Methods {
			mv := MethodView{Method: m.Method}
			for _, e := range m.Encounters {
				mv.Encounters = append(mv.Encounters, EncounterView{Encounter: e,
					Name:   gen4.Name(l.data.Species, e.Species),
					Sprite: s.dsSpriteURL(l.data, &gen3.Pokemon{Species: e.Species}),
					Seen:   gen4.Has(l.seen, e.Species),
					Caught: gen4.Has(l.caught, e.Species)})
			}
			pv.Methods = append(pv.Methods, mv)
		}
		loc.Periods = append(loc.Periods, pv)
	}
	return loc, nil
}

// liveDSView reads the running DS game's PC straight from its RAM: the game
// keeps the whole save there with the save file's layout, so the latest
// save file says how big the blocks are and how far the PC is from the
// live general block. It refuses (so the caller can fall back to the save
// file) unless the RAM still looks like that save's data.
func (s *Server) liveDSView() (*SaveView, error) {
	running, _ := s.game()
	if running == nil || running.Emulator != "drastic" || s.saves == nil {
		return nil, errNotLive
	}
	s.mu.Lock()
	pid, general := s.dsAt.pid, s.dsAt.general
	s.mu.Unlock()
	if general == 0 || pid != running.PID {
		return nil, errors.New("still looking for the game's data in memory")
	}
	files := s.saves.SaveFor(running.ROM)
	if len(files) == 0 {
		return nil, errors.New("this game has no save yet")
	}
	data, err := s.saves.ReadCurrent(files[0])
	if err != nil {
		return nil, err
	}
	saved, err := gen4.ParseSave(data)
	if err != nil {
		return nil, err
	}
	d, err := s.dsData(running.ROM)
	if err != nil {
		return nil, err
	}
	mem, err := emulator.OpenProcMem("/proc", pid)
	if err != nil {
		return nil, err
	}
	defer mem.Close()
	g, err := mem.Read(general, len(saved.General))
	if err != nil {
		return nil, err
	}
	st, err := mem.Read(general+uint32(saved.StorageOffset), len(saved.Storage))
	if err != nil {
		return nil, err
	}
	live := &gen4.Save{General: g, Storage: st, Counter: saved.Counter, StorageOffset: saved.StorageOffset}
	if !gen4.CheckGeneral(g, d.Layout) || live.BoxName(d.Layout, 0) == "" || live.BoxName(d.Layout, 0) != saved.BoxName(d.Layout, 0) {
		return nil, errors.New("the PC in memory doesn't match the save's layout")
	}
	view, err := s.dsSaveView(running.ROM, d, live)
	if err != nil {
		return nil, err
	}
	view.File, view.Version = files[0], "live"
	return view, nil
}
