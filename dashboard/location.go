package dashboard

import (
	"fmt"

	"tallgrass/gen3"
	"tallgrass/profile"
)

// Location is the current map and what can be caught there.
type Location struct {
	MapGroup int          `json:"mapGroup"`
	MapNum   int          `json:"mapNum"`
	Section  int          `json:"section"`
	Name     string       `json:"name"`
	Hour     *int         `json:"hour,omitempty"` // in-game clock, if the game has one
	Periods  []PeriodView `json:"periods"`        // empty when there are no wild Pokémon
}

type PeriodView struct {
	Name     string       `json:"name"` // "" when the game has no time of day
	FromHour int          `json:"fromHour"`
	ToHour   int          `json:"toHour"`
	Active   bool         `json:"active"`
	Swapped  bool         `json:"swapped,omitempty"` // the game's option swapping day and night is on
	Methods  []MethodView `json:"methods"`
}

type MethodView struct {
	Method     string          `json:"method"`
	Encounters []EncounterView `json:"encounters"`
}

type EncounterView struct {
	gen3.Encounter
	Name   string `json:"name"`
	Sprite string `json:"sprite,omitempty"`
	Seen   bool   `json:"seen"`
	Caught bool   `json:"caught"`
}

// location reads the current map and, if the profile supports it, its wild
// encounters marked with Pokédex seen/caught flags.
func (s *Server) location(seen, caught []byte) (*Location, error) {
	cfg := s.prof.Location
	if cfg == nil {
		return nil, nil
	}

	idAddr, err := cfg.MapID.Resolve(s.read)
	if err != nil {
		return nil, err
	}
	id, err := s.read(idAddr, 2)
	if err != nil {
		return nil, err
	}
	loc := &Location{MapGroup: int(id[0]), MapNum: int(id[1]), Periods: []PeriodView{}}

	secAddr, err := cfg.MapSection.Resolve(s.read)
	if err != nil {
		return nil, err
	}
	sec, err := s.read(secAddr, 1)
	if err != nil {
		return nil, err
	}
	loc.Section = int(sec[0])
	if loc.Name, err = s.mapName(loc.Section); err != nil {
		return nil, fmt.Errorf("map name: %w", err)
	}

	if s.prof.WildEncounters == nil {
		return loc, nil
	}
	periods, err := s.encounters(id[0], id[1])
	if err != nil || len(periods) == 0 {
		return loc, err
	}

	if hourRef := s.prof.WildEncounters.ClockHour; !hourRef.IsZero() {
		addr, err := hourRef.Resolve(s.read)
		if err != nil {
			return nil, err
		}
		b, err := s.read(addr, 1)
		if err != nil {
			return nil, err
		}
		hour := int(b[0])
		loc.Hour = &hour
	}

	swapped := false
	if f := s.prof.WildEncounters.SwapFlag; f != 0 && len(periods) == 2 {
		swapped, err = s.flag(int(f))
		if err != nil {
			return nil, err
		}
	}
	for _, p := range periods {
		pv := PeriodView{
			Name:     p.Name,
			FromHour: p.FromHour,
			ToHour:   p.ToHour,
			Active:   len(periods) == 1 || (loc.Hour != nil && p.Contains(*loc.Hour) != swapped),
			Swapped:  swapped,
		}
		for _, m := range p.Methods {
			mv := MethodView{Method: m.Method}
			for _, e := range m.Encounters {
				mv.Encounters = append(mv.Encounters, s.encounterView(e, seen, caught))
			}
			pv.Methods = append(pv.Methods, mv)
		}
		loc.Periods = append(loc.Periods, pv)
	}
	return loc, nil
}

// flag reads one of the game's flags; false if the profile has no flags.
func (s *Server) flag(n int) (bool, error) {
	if s.prof.Flags.IsZero() {
		return false, nil
	}
	addr, err := s.prof.Flags.Resolve(s.read)
	if err != nil {
		return false, err
	}
	b, err := s.read(addr+uint32(n/8), 1)
	if err != nil {
		return false, err
	}
	return b[0]>>(n%8)&1 != 0, nil
}

// mapName names a region map section; "" if the profile has no location.
func (s *Server) mapName(section int) (string, error) {
	cfg := s.prof.Location
	if cfg == nil {
		return "", nil
	}
	if s.mapNames == 0 {
		var err error
		if s.mapNames, err = cfg.MapNames.Resolve(s.read); err != nil {
			return "", err
		}
	}
	if section < cfg.FirstMapSection {
		return "", nil // no map yet (title screen, loading)
	}
	stride, offset := cfg.MapNameTable()
	return s.names.MapName(s.mapNames, stride, offset, cfg.FirstMapSection, section)
}

func (s *Server) encounters(group, num uint8) ([]gen3.PeriodEncounters, error) {
	if s.wild == nil {
		w, err := newWild(s.read, s.prof.WildEncounters)
		if err != nil {
			return nil, err
		}
		s.wild = w
	}
	return s.wild.ForMap(group, num)
}

// newWild builds the encounter reader a profile describes.
func newWild(read gen3.ReadMemory, cfg *profile.WildEncounters) (*gen3.Wild, error) {
	main, err := cfg.Headers.Resolve(read)
	if err != nil {
		return nil, err
	}
	var periods []gen3.Period
	for _, p := range cfg.Periods {
		addr, err := p.Headers.Resolve(read)
		if err != nil {
			return nil, err
		}
		land := -1
		if p.LandSlot != nil {
			land = *p.LandSlot
		}
		periods = append(periods, gen3.Period{Name: p.Name, Headers: addr, FromHour: p.FromHour, ToHour: p.ToHour, LandSlot: land})
	}
	w := gen3.NewWild(read, main, periods)
	if cfg.HeaderSize > 0 && (len(cfg.Slots) == 4 || len(cfg.Slots) == 5) {
		l := gen3.WildLayout{HeaderSize: cfg.HeaderSize, Slots: [5]int{-1, -1, -1, -1, -1}}
		copy(l.Slots[:], cfg.Slots)
		w.WithLayout(l)
	}
	return w, nil
}

// dexFlags reads the seen and caught flag arrays; nil if not configured.
func (s *Server) dexFlags() (seen, caught []byte, err error) {
	cfg := s.prof.Pokedex
	if cfg == nil {
		return nil, nil, nil
	}
	if s.national == nil {
		table, err := cfg.SpeciesToNational.Resolve(s.read)
		if err != nil {
			return nil, nil, err
		}
		s.national = gen3.NewNational(s.read, table)
	}
	read := func(ref profile.Ref) ([]byte, error) {
		addr, err := ref.Resolve(s.read)
		if err != nil {
			return nil, err
		}
		return s.read(addr, cfg.FlagBytes)
	}
	if seen, err = read(cfg.Seen); err != nil {
		return nil, nil, err
	}
	if caught, err = read(cfg.Caught); err != nil {
		return nil, nil, err
	}
	return seen, caught, nil
}

func (s *Server) encounterView(e gen3.Encounter, seen, caught []byte) EncounterView {
	v := EncounterView{Encounter: e, Sprite: s.spriteURL(e.Species, false)}
	v.Name, _ = s.names.Species(e.Species)
	if s.national != nil {
		if n, err := s.national.Number(e.Species); err == nil {
			v.Seen = gen3.HasFlag(seen, n)
			v.Caught = gen3.HasFlag(caught, n)
		}
	}
	return v
}
