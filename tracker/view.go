package tracker

import (
	"sort"
	"time"
)

// View is the run as the dashboard shows it.
type View struct {
	Player    string     `json:"player"`
	Nuzlocke  bool       `json:"nuzlocke"` // Nuzlocke mode is on for this game
	StartedAt time.Time  `json:"startedAt"`
	Alive     int        `json:"alive"`
	Dead      int        `json:"dead"`
	Mons      []MonView  `json:"mons"`   // party first, then the rest; dead last
	Areas     []AreaView `json:"areas"`  // in order of first visit
	Events    []Event    `json:"events"` // newest first
}

type MonView struct {
	Mon
	InParty bool `json:"inParty"`
}

type AreaView struct {
	Section int      `json:"section"`
	Name    string   `json:"name"`
	Catches []string `json:"catches"` // what was obtained here, oldest first
}

const viewEvents = 50

// View summarises the current run; nil before a save has been seen.
func (e *Engine) View() *View {
	if e.runID == 0 {
		return nil
	}
	v := &View{Player: e.player, Nuzlocke: e.nuzlocke, StartedAt: e.startedAt, Mons: []MonView{}, Areas: []AreaView{}, Events: []Event{}}

	for _, m := range e.mons {
		v.Mons = append(v.Mons, MonView{Mon: *m, InParty: e.inParty[m.Personality]})
		if m.IsEgg {
			continue
		}
		if m.FaintedAt != nil {
			v.Dead++
		} else {
			v.Alive++
		}
	}
	sort.Slice(v.Mons, func(i, j int) bool {
		a, b := v.Mons[i], v.Mons[j]
		if (a.FaintedAt == nil) != (b.FaintedAt == nil) {
			return a.FaintedAt == nil
		}
		if a.InParty != b.InParty {
			return a.InParty
		}
		if a.InParty {
			return e.partySlot[a.Personality] < e.partySlot[b.Personality]
		}
		// Pokémon recorded together (e.g. when tracking began) tie on time;
		// break ties so the order never changes between polls.
		if !a.ObtainedAt.Equal(b.ObtainedAt) {
			return a.ObtainedAt.Before(b.ObtainedAt)
		}
		return a.Personality < b.Personality
	})

	// Areas: every visited one, plus any where a Pokémon was met before
	// tracking began (e.g. the starter's lab).
	type area struct {
		AreaView
		first time.Time
	}
	areas := map[int]*area{}
	for _, a := range e.areas {
		areas[a.Section] = &area{AreaView{Section: a.Section, Name: a.Name, Catches: []string{}}, a.FirstVisit}
	}
	// Areas only known from a met location (never visited while tracking,
	// like the starter's lab) sort first, as they predate tracking.
	addCatch := func(section int, name string, what string) {
		a, ok := areas[section]
		if !ok {
			a = &area{AreaView{Section: section, Name: name, Catches: []string{}}, time.Time{}}
			areas[section] = a
		}
		a.Catches = append(a.Catches, what)
	}
	byTime := make([]*Mon, 0, len(e.mons))
	for _, m := range e.mons {
		byTime = append(byTime, m)
	}
	sort.Slice(byTime, func(i, j int) bool {
		a, b := byTime[i], byTime[j]
		if !a.ObtainedAt.Equal(b.ObtainedAt) {
			return a.ObtainedAt.Before(b.ObtainedAt)
		}
		return a.Personality < b.Personality
	})
	for _, m := range byTime {
		if !m.IsEgg && m.MetArea != "" {
			addCatch(m.MetSection, m.MetArea, label(m.Nickname, m.SpeciesName))
		}
	}
	for _, ev := range e.events {
		// PC catches seen in a box are already listed as Pokémon above; only
		// the dex-flag guesses (no personality) need adding.
		if ev.Kind == KindCaughtPC && ev.Area != "" && ev.Personality == 0 {
			addCatch(ev.Section, ev.Area, ev.Subject+" (PC)")
		}
	}
	sorted := make([]*area, 0, len(areas))
	for _, a := range areas {
		sorted = append(sorted, a)
	}
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if !a.first.Equal(b.first) {
			return a.first.Before(b.first)
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Section < b.Section
	})
	for _, a := range sorted {
		v.Areas = append(v.Areas, a.AreaView)
	}

	for i := len(e.events) - 1; i >= 0 && len(v.Events) < viewEvents; i-- {
		v.Events = append(v.Events, e.events[i])
	}
	return v
}
