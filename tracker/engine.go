package tracker

import (
	"fmt"
	"sort"
	"time"
)

// Event kinds.
const (
	KindStarted  = "tracking_started"
	KindObtained = "obtained"
	KindEgg      = "egg"
	KindHatched  = "hatched"
	KindCaughtPC = "caught_pc"
	KindFainted  = "fainted"
	KindLevelUp  = "level_up"
	KindEvolved  = "evolved"
	KindNewArea  = "new_area"
)

// pcCatchDelay is how long a newly set Pokédex caught flag waits for that
// species to join the party before we decide it went to the PC. The flag is
// set at "Gotcha!", but the Pokémon only joins after the Pokédex entry and
// nickname prompt.
const pcCatchDelay = 2 * time.Minute

// Observation is one poll of the game, already decoded and named.
type Observation struct {
	At         time.Time
	TrainerID  uint32
	PlayerName string
	Party      []PartyMon
	Section    int    // region map section, -1 if unknown
	Area       string // its name
	DexCaught  []int  // national numbers flagged caught; nil if unknown
	// Boxes is every Pokémon in the PC (HP ignored), when it was read this
	// time; nil otherwise. The PC is read less often than the party.
	Boxes []PartyMon
}

type PartyMon struct {
	Personality uint32
	Species     int
	SpeciesName string
	National    int
	Nickname    string
	Level       int
	CurrentHP   int
	IsEgg       bool
	MetSection  int
	MetArea     string
}

// Mon is a Pokémon this run has owned.
type Mon struct {
	Personality uint32     `json:"personality"`
	Species     int        `json:"species"`
	SpeciesName string     `json:"speciesName"`
	Nickname    string     `json:"nickname"`
	Level       int        `json:"level"`
	IsEgg       bool       `json:"isEgg"`
	MetSection  int        `json:"metSection"`
	MetArea     string     `json:"metArea"`
	ObtainedAt  time.Time  `json:"obtainedAt"`
	FaintedAt   *time.Time `json:"faintedAt,omitempty"` // first faint; dead in a Nuzlocke
}

type Area struct {
	Section    int       `json:"section"`
	Name       string    `json:"name"`
	FirstVisit time.Time `json:"firstVisit"`
}

type Event struct {
	ID          int64     `json:"id"`
	At          time.Time `json:"at"`
	Kind        string    `json:"kind"`
	Personality uint32    `json:"personality,omitempty"`
	Species     int       `json:"species,omitempty"`
	Section     int       `json:"section"`
	Area        string    `json:"area"`
	Subject     string    `json:"subject,omitempty"` // the Pokémon involved
	Text        string    `json:"text"`
}

type pendingFlag struct {
	at      time.Time
	section int
	area    string
}

// Engine compares each observation with the last and records what changed.
// Not safe for concurrent use.
type Engine struct {
	store *Store
	game  string
	// speciesName names a national dex number, for catches sent to the PC.
	speciesName func(national int) (species int, name string)

	runID     int64
	trainer   uint32
	player    string
	startedAt time.Time
	mons      map[uint32]*Mon
	areas     map[int]*Area
	dex       map[int]bool
	events    []Event
	section   int
	nuzlocke  bool // Nuzlocke mode for this game: faints are deaths

	// In memory only; rebuilt from the next observations after a restart.
	hp          map[uint32]int
	newMons     map[uint32]int // unknown personalities -> consecutive sightings
	newBoxMons  map[uint32]int // same, for the PC (counted per PC read)
	pendingDex  map[int]pendingFlag
	dexBaseline bool // first dex read of a new run: already-set flags aren't news
	boxBaseline bool // first PC read this session: what's there isn't news
	boxesSeen   bool // the PC can be read, so PC catches come from it, not dex flags
	inParty     map[uint32]bool
	partySlot   map[uint32]int // party order, for a stable view
}

func NewEngine(store *Store, game string, speciesName func(national int) (int, string)) *Engine {
	return &Engine{store: store, game: game, speciesName: speciesName}
}

// SetNuzlocke turns Nuzlocke mode on or off for the current game. Events are
// recorded either way; the mode changes what the view emphasises.
func (e *Engine) SetNuzlocke(on bool) error {
	if err := e.store.SetSetting(e.game, "nuzlocke", fmt.Sprint(on)); err != nil {
		return err
	}
	e.nuzlocke = on
	return nil
}

// SetGame switches to another game; its run loads on the next observation.
func (e *Engine) SetGame(game string) {
	if game != e.game {
		e.game, e.runID = game, 0
	}
}

// Observe records events for anything that changed since the last call.
func (e *Engine) Observe(o Observation) error {
	if o.TrainerID == 0 || o.PlayerName == "" {
		return nil // title screen or no save loaded yet
	}
	if e.runID == 0 || o.TrainerID != e.trainer || o.PlayerName != e.player {
		if err := e.loadRun(o); err != nil {
			return err
		}
	}

	if err := e.observeArea(o); err != nil {
		return err
	}
	if err := e.observeParty(o); err != nil {
		return err
	}
	if err := e.observeBoxes(o); err != nil {
		return err
	}
	return e.observeDex(o)
}

func (e *Engine) loadRun(o Observation) error {
	id, created, err := e.store.run(e.game, o.TrainerID, o.PlayerName, o.At)
	if err != nil {
		return fmt.Errorf("loading run: %w", err)
	}
	mons, err := e.store.loadMons(id)
	if err != nil {
		return err
	}
	areas, err := e.store.loadAreas(id)
	if err != nil {
		return err
	}
	dex, err := e.store.loadDexCaught(id)
	if err != nil {
		return err
	}
	events, err := e.store.loadEvents(id)
	if err != nil {
		return err
	}

	setting, err := e.store.Setting(e.game, "nuzlocke")
	if err != nil {
		return err
	}

	*e = Engine{
		store: e.store, game: e.game, speciesName: e.speciesName, nuzlocke: setting == "true",
		runID: id, trainer: o.TrainerID, player: o.PlayerName, startedAt: o.At,
		mons: mons, areas: areas, dex: dex, events: events, section: -1,
		hp: map[uint32]int{}, newMons: map[uint32]int{}, newBoxMons: map[uint32]int{}, pendingDex: map[int]pendingFlag{},
		dexBaseline: created, boxBaseline: true, inParty: map[uint32]bool{},
	}
	if len(events) > 0 {
		e.startedAt = events[0].At
	}
	if created {
		return e.emit(Event{At: o.At, Kind: KindStarted, Section: o.Section, Area: o.Area,
			Text: fmt.Sprintf("Started tracking %s's run", o.PlayerName)})
	}
	return nil
}

func (e *Engine) emit(ev Event) error {
	if err := e.store.addEvent(e.runID, &ev); err != nil {
		return fmt.Errorf("saving event: %w", err)
	}
	e.events = append(e.events, ev)
	return nil
}

func (e *Engine) observeArea(o Observation) error {
	if o.Section < 0 || o.Section == e.section {
		return nil
	}
	e.section = o.Section
	if _, ok := e.areas[o.Section]; ok || o.Area == "" {
		return nil
	}
	a := &Area{Section: o.Section, Name: o.Area, FirstVisit: o.At}
	e.areas[o.Section] = a
	if err := e.store.saveArea(e.runID, a); err != nil {
		return err
	}
	return e.emit(Event{At: o.At, Kind: KindNewArea, Section: o.Section, Area: o.Area,
		Text: "Arrived at " + o.Area})
}

// plausible filters out half-written party data, e.g. while a save loads.
func plausible(p PartyMon) bool {
	return p.Species > 0 && p.Species < 4096 && p.Level >= 1 && p.Level <= 100
}

func (e *Engine) observeParty(o Observation) error {
	inParty := map[uint32]bool{}
	partySlot := map[uint32]int{}
	for slot, p := range o.Party {
		if !plausible(p) {
			continue
		}
		inParty[p.Personality] = true
		partySlot[p.Personality] = slot

		m, known := e.mons[p.Personality]
		if !known {
			// Log a newcomer only once it's been seen twice in a row.
			if e.newMons[p.Personality]++; e.newMons[p.Personality] < 2 {
				continue
			}
			delete(e.newMons, p.Personality)
			if err := e.obtained(o, p); err != nil {
				return err
			}
			e.hp[p.Personality] = p.CurrentHP
			continue
		}

		if err := e.update(o, p, m); err != nil {
			return err
		}
	}

	for pers := range e.newMons {
		if !inParty[pers] {
			delete(e.newMons, pers)
		}
	}
	e.inParty, e.partySlot = inParty, partySlot
	return nil
}

func (e *Engine) obtained(o Observation, p PartyMon) error {
	m := &Mon{
		Personality: p.Personality, Species: p.Species, SpeciesName: p.SpeciesName, Nickname: p.Nickname,
		Level: p.Level, IsEgg: p.IsEgg, MetSection: p.MetSection, MetArea: p.MetArea, ObtainedAt: o.At,
	}
	e.mons[p.Personality] = m
	if err := e.store.saveMon(e.runID, m); err != nil {
		return err
	}

	ev := Event{At: o.At, Personality: p.Personality, Species: p.Species, Section: p.MetSection, Area: p.MetArea,
		Subject: label(p.Nickname, p.SpeciesName)}
	if p.IsEgg {
		ev.Kind, ev.Text = KindEgg, "Received an Egg"
	} else {
		ev.Kind = KindObtained
		ev.Text = fmt.Sprintf("Got %s Lv %d", ev.Subject, p.Level)
		if p.MetArea != "" {
			ev.Text += " from " + p.MetArea
		}
	}
	return e.emit(ev)
}

func (e *Engine) update(o Observation, p PartyMon, m *Mon) error {
	changed := false
	base := Event{At: o.At, Personality: p.Personality, Species: p.Species, Section: o.Section, Area: o.Area,
		Subject: label(p.Nickname, p.SpeciesName)}

	if m.IsEgg && !p.IsEgg {
		m.IsEgg, m.Species, m.SpeciesName, m.Level = false, p.Species, p.SpeciesName, p.Level
		changed = true
		ev := base
		ev.Kind, ev.Text = KindHatched, fmt.Sprintf("Egg hatched into %s", p.SpeciesName)
		if err := e.emit(ev); err != nil {
			return err
		}
	}
	if !p.IsEgg && p.Species != m.Species {
		ev := base
		ev.Kind = KindEvolved
		ev.Text = fmt.Sprintf("%s evolved from %s into %s", label(p.Nickname, m.SpeciesName), m.SpeciesName, p.SpeciesName)
		m.Species, m.SpeciesName = p.Species, p.SpeciesName
		changed = true
		if err := e.emit(ev); err != nil {
			return err
		}
	}
	if !p.IsEgg && p.Level > m.Level {
		ev := base
		ev.Kind = KindLevelUp
		ev.Text = fmt.Sprintf("%s grew to Lv %d", label(p.Nickname, p.SpeciesName), p.Level)
		m.Level = p.Level
		changed = true
		if err := e.emit(ev); err != nil {
			return err
		}
	}
	if p.Nickname != m.Nickname {
		m.Nickname = p.Nickname
		changed = true
	}

	// A faint is HP reaching 0. After a restart we have no previous HP, so
	// only count it if this Pokémon hasn't already been recorded as fainted.
	prev, seenHP := e.hp[p.Personality]
	e.hp[p.Personality] = p.CurrentHP
	if !p.IsEgg && p.CurrentHP == 0 && ((seenHP && prev > 0) || (!seenHP && m.FaintedAt == nil)) {
		ev := base
		ev.Kind = KindFainted
		ev.Text = fmt.Sprintf("%s fainted at Lv %d", label(p.Nickname, p.SpeciesName), p.Level)
		if o.Area != "" {
			ev.Text += " on " + o.Area
		}
		if m.FaintedAt == nil {
			at := o.At
			m.FaintedAt = &at
			changed = true
		}
		if err := e.emit(ev); err != nil {
			return err
		}
	}

	if changed {
		return e.store.saveMon(e.runID, m)
	}
	return nil
}

func (e *Engine) observeDex(o Observation) error {
	if o.DexCaught == nil {
		return nil
	}

	var fresh []int
	for _, n := range o.DexCaught {
		if !e.dex[n] {
			e.dex[n] = true
			fresh = append(fresh, n)
		}
	}
	if len(fresh) > 0 {
		if err := e.store.saveDexCaught(e.runID, fresh); err != nil {
			return err
		}
	}
	if e.dexBaseline || e.boxesSeen {
		// Already-set flags at the start aren't news, and once the PC can be
		// read, catches sent there are seen directly (observeBoxes).
		e.dexBaseline = false
		return nil
	}
	for _, n := range fresh {
		e.pendingDex[n] = pendingFlag{at: o.At, section: o.Section, area: o.Area}
	}

	// Resolve pending flags: the species joining the party means an ordinary
	// catch (logged by observeParty); otherwise, after a while, the PC.
	inParty := map[int]bool{}
	for _, p := range o.Party {
		inParty[p.National] = true
	}
	var due []int
	for n, f := range e.pendingDex {
		if inParty[n] {
			delete(e.pendingDex, n)
		} else if o.At.Sub(f.at) >= pcCatchDelay {
			due = append(due, n)
		}
	}
	sort.Ints(due)
	for _, n := range due {
		f := e.pendingDex[n]
		delete(e.pendingDex, n)
		species, name := e.speciesName(n)
		if name == "" {
			name = fmt.Sprintf("#%d", n)
		}
		text := fmt.Sprintf("Caught %s, sent to the PC", name)
		if f.area != "" {
			text = fmt.Sprintf("Caught %s on %s, sent to the PC", name, f.area)
		}
		if err := e.emit(Event{At: f.at, Kind: KindCaughtPC, Species: species, Section: f.section, Area: f.area,
			Subject: name, Text: text}); err != nil {
			return err
		}
	}
	return nil
}

// observeBoxes logs Pokémon that appear in the PC without having been in the
// party: catches sent straight to the PC, including species already owned.
func (e *Engine) observeBoxes(o Observation) error {
	if o.Boxes == nil {
		return nil
	}
	e.boxesSeen = true
	inBoxes := map[uint32]bool{}
	for _, p := range o.Boxes {
		if !plausible(p) || e.mons[p.Personality] != nil {
			continue
		}
		inBoxes[p.Personality] = true

		if e.boxBaseline {
			// Already in the PC when this session started: record it quietly.
			if err := e.addMon(p, o.At); err != nil {
				return err
			}
			continue
		}
		if e.newBoxMons[p.Personality]++; e.newBoxMons[p.Personality] < 2 {
			continue
		}
		delete(e.newBoxMons, p.Personality)
		if err := e.addMon(p, o.At); err != nil {
			return err
		}
		ev := Event{At: o.At, Kind: KindCaughtPC, Personality: p.Personality, Species: p.Species,
			Section: p.MetSection, Area: p.MetArea, Subject: label(p.Nickname, p.SpeciesName)}
		ev.Text = fmt.Sprintf("Caught %s Lv %d", ev.Subject, p.Level)
		if p.MetArea != "" {
			ev.Text += " on " + p.MetArea
		}
		ev.Text += ", sent to the PC"
		if err := e.emit(ev); err != nil {
			return err
		}
	}
	for pers := range e.newBoxMons {
		if !inBoxes[pers] {
			delete(e.newBoxMons, pers)
		}
	}
	e.boxBaseline = false
	return nil
}

func (e *Engine) addMon(p PartyMon, at time.Time) error {
	m := &Mon{
		Personality: p.Personality, Species: p.Species, SpeciesName: p.SpeciesName, Nickname: p.Nickname,
		Level: p.Level, IsEgg: p.IsEgg, MetSection: p.MetSection, MetArea: p.MetArea, ObtainedAt: at,
	}
	e.mons[p.Personality] = m
	return e.store.saveMon(e.runID, m)
}

func label(nickname, species string) string {
	if nickname == "" || nickname == species {
		return species
	}
	return fmt.Sprintf("%s (%s)", nickname, species)
}
