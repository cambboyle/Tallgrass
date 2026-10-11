// Package dashboard serves the live party web UI. A single poller goroutine
// reads game RAM and every browser gets updates pushed over Server-Sent Events.
package dashboard

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"image/png"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tallgrass/emulator"
	"tallgrass/gen3"
	"tallgrass/profile"
	"tallgrass/saves"
	"tallgrass/tracker"
)

//go:embed static
var staticFiles embed.FS

// State is what the browser sees.
type State struct {
	Game      string        `json:"game"`              // profile name, or the ROM's file name if unsupported
	Running   *GameInfo     `json:"running,omitempty"` // what the emulator has open, when detection works
	Connected bool          `json:"connected"`
	Error     string        `json:"error,omitempty"`
	Party     []*Member     `json:"party"` // one entry per slot, null when empty
	Location  *Location     `json:"location,omitempty"`
	Run       *tracker.View `json:"run,omitempty"` // Nuzlocke log, if tracking is on
}

// Member is a decoded Pokémon plus names looked up from the ROM.
// Embedding *gen3.Pokemon puts its fields at the top level of the JSON.
type Member struct {
	*gen3.Pokemon
	SpeciesName string   `json:"speciesName"`
	ItemName    string   `json:"itemName"`
	MoveNames   []string `json:"moveNames"`
	AbilityName string   `json:"abilityName,omitempty"` // games that store an ability ID (Gen 4+)
	Sprite      string   `json:"sprite,omitempty"`      // URL of a PNG, empty if unavailable
}

// GameInfo describes the game open in an emulator.
type GameInfo struct {
	Emulator  string `json:"emulator"`
	File      string `json:"file"` // ROM file name
	SHA1      string `json:"sha1"`
	Supported bool   `json:"supported"` // a profile matches this ROM
	Live      bool   `json:"live"`      // we can read its memory
}

type Server struct {
	// prof is the current game's profile: fixed when detection is off,
	// switched by the poller when it's on (nil for an unsupported game).
	// The poller is its only writer; others read it under mu.
	prof  *profile.Profile
	read  gen3.ReadMemory
	names *gen3.Names

	detect  func() (*emulator.Running, error) // nil when we can't see processes
	running *emulator.Running                 // guarded by mu
	info    *GameInfo
	dsLive  *dsLive     // following a DS game in DraStic; poller only
	cloud   *cloudState // account linking; nil when off
	// Where the poller found the DS game's live save data, for handlers
	// (they open their own reads of DraStic's memory). Guarded by mu.
	dsAt struct {
		pid     int
		general uint32
	}

	mu      sync.Mutex
	state   []byte        // latest State as JSON
	changed chan struct{} // closed and replaced whenever state changes

	// Sprites rendered from ROM, keyed by the path after /api/sprite/. A nil
	// entry means rendering failed, so we don't retry every poll. The
	// generation goes in the URL so browsers never reuse another game's sprite.
	sprites    map[string][]byte
	generation int

	// Built on first use from ROM pointers, and dropped when the game
	// disconnects. Only the poller goroutine touches these.
	wild        *gen3.Wild
	species     *gen3.SpeciesTables // for ability names; nil until needed
	esGamelists string              // EmulationStation's gamelists folder; "" = off
	gamelistOn  bool                // game names asked for (also kept as a setting); under mu
	national    *gen3.National
	levels      *gen3.Levels
	mapNames    uint32
	lastPC      time.Time // when the tracker last read the PC

	engine  *tracker.Engine // nil unless EnableTracking was called
	store   *tracker.Store  // settings and the run log; nil unless EnableTracking was called
	setup   *setupState     // the setup wizard; nil when off
	auth    *authState      // dashboard PIN; nil when off
	updates *updateState    // self-update; nil when off
	// A Nuzlocke toggle from the web, applied by the poller (the engine's
	// only user); guarded by mu.
	pendingNuzlocke *bool
	saves           *saves.Watcher // nil unless EnableSaves was called

	// For viewing save files from ROM files on disk.
	profiles []*profile.Profile
	romCache romCache
	ds       dsCache

	connected      bool // whether the last poll reached a running game; guarded by mu
	libraryHashing bool // a background ROM hashing run is going; guarded by mu
}

func New(prof *profile.Profile, read gen3.ReadMemory) *Server {
	s := &Server{
		prof:    prof,
		read:    read,
		names:   gen3.NewNames(read, nameTables(prof)),
		changed: make(chan struct{}),
		sprites: map[string][]byte{},
	}
	s.state, _ = json.Marshal(State{Game: prof.Name, Party: []*Member{}})
	return s
}

// Poll reads the party every interval until the process exits.
func (s *Server) Poll(interval time.Duration) {
	for {
		s.update(s.Snapshot())
		time.Sleep(interval)
	}
}

// Snapshot reads and decodes the party right now.
func (s *Server) Snapshot() State {
	if s.detect != nil {
		running, err := s.detect()
		if err != nil {
			log.Printf("detecting the running game: %v", err)
		}
		s.switchGame(running)
		if running != nil && s.info != nil && s.info.Live && running.Emulator == "drastic" {
			return s.dsSnapshot(running)
		}
		if running == nil || s.prof == nil || !running.Live() {
			s.setConnected(false)
			st := State{Party: []*Member{}, Running: s.info}
			if s.info != nil {
				st.Game = s.info.File
				if s.prof != nil {
					st.Game = s.prof.Name
				}
			}
			if s.engine != nil {
				s.applySettings()
				st.Run = s.engine.View()
			}
			return st
		}
	}

	st := State{Game: s.prof.Name, Running: s.info, Party: []*Member{}}

	size := s.prof.PokemonSize
	raw, err := s.read(uint32(s.prof.PartyAddress), s.prof.PartySlots*size)
	// Record this now: the rest of the first poll after a start can take
	// seconds (RetroArch answers one command per frame), and the Boxes page
	// shouldn't say "no game" meanwhile.
	s.setConnected(err == nil)
	if err != nil {
		// The game may have been closed or swapped, so cached ROM data can't be trusted.
		s.resetGameCaches()
		st.Error = fmt.Sprintf("can't read game memory (is a game running?): %v", err)
		return st
	}
	st.Connected = true

	ivs, haveIVs := s.skillIVs()
	scaled, scaling, err := s.scaledLevel()
	if err != nil {
		log.Printf("level scaling: %v", err)
	}
	for slot := 0; slot < s.prof.PartySlots; slot++ {
		mon, err := gen3.DecodeAs(s.prof.PokemonFormat, raw[slot*size:(slot+1)*size], s.prof.EncryptedData)
		if mon != nil && scaling {
			mon.Level = scaled // the game reports this level everywhere
		}
		if mon != nil && haveIVs {
			mon.IVs = ivs
		}
		if err == nil {
			err = gen3.ResolveForm(s.read, uint32(s.prof.FormSpecies), mon)
		}
		if err != nil {
			log.Printf("slot %d: %v", slot+1, err)
		}
		if mon == nil {
			st.Party = append(st.Party, nil)
			continue
		}
		st.Party = append(st.Party, s.member(mon))
	}

	seen, caught, err := s.dexFlags()
	if err != nil {
		log.Printf("reading pokédex flags: %v", err)
	}
	st.Location, err = s.location(seen, caught)
	if err != nil {
		log.Printf("reading location: %v", err)
	}
	if s.engine != nil {
		if err := s.track(st, caught); err != nil {
			log.Printf("tracking: %v", err)
		}
		s.applySettings()
		st.Run = s.engine.View()
	}
	return st
}

func (s *Server) setConnected(c bool) {
	s.mu.Lock()
	s.connected = c
	s.mu.Unlock()
}

// resetGameCaches drops everything read from the current game's ROM.
func (s *Server) resetGameCaches() {
	s.names.Reset()
	s.resetSprites()
	s.wild, s.national, s.levels, s.mapNames = nil, nil, nil, 0
	s.species = nil
	s.lastPC = time.Time{}
}

// switchGame follows the emulator to a new game: it finds the ROM's profile
// by hash and rebuilds everything that depends on the game.
func (s *Server) switchGame(running *emulator.Running) {
	s.mu.Lock()
	same := (running == nil && s.running == nil) ||
		(running != nil && s.running != nil && running.ROM == s.running.ROM && running.PID == s.running.PID)
	info := s.info
	s.mu.Unlock()
	if same {
		// DraStic maps the DS's RAM a moment after it starts, so a game
		// seen at launch may become readable on a later poll.
		if info != nil && info.Supported && !info.Live && running.Emulator == "drastic" && canFollowDS("/proc", running) {
			live := *info
			live.Live = true
			s.mu.Lock()
			s.info = &live
			s.mu.Unlock()
			log.Printf("%s is now readable live", info.File)
		}
		return
	}

	var prof *profile.Profile
	info = nil
	if running != nil {
		info = &GameInfo{Emulator: running.Emulator, File: filepath.Base(running.ROM)}
		sum, err := s.hashROM(running.ROM)
		if err != nil {
			log.Printf("hashing %s: %v", running.ROM, err)
		}
		info.SHA1 = sum
		prof = s.profileFor(sum, running.ROM)
		info.Supported = prof != nil
		info.Live = prof != nil && running.Live()
		if running.Emulator == "drastic" && dsFamily(running.ROM) != "" {
			info.Supported = true
			info.Live = canFollowDS("/proc", running)
		}
		log.Printf("now playing %s in %s (profile: %v)", info.File, running.Emulator, profileName(prof))
	}

	s.mu.Lock()
	s.running, s.info, s.prof = running, info, prof
	s.mu.Unlock()

	if prof != nil {
		s.names = gen3.NewNames(s.read, nameTables(prof))
		if s.engine != nil {
			s.engine.SetGame(prof.Name)
		}
	} else if info != nil && info.Supported && s.engine != nil {
		s.engine.SetGame(strings.TrimSuffix(info.File, filepath.Ext(info.File))) // DS games have no profile
	}
	if running == nil || running.Emulator != "drastic" {
		s.dsLive.close()
		s.dsLive = nil
	}
	s.resetGameCaches()
}

func profileName(p *profile.Profile) string {
	if p == nil {
		return "none"
	}
	return p.Name
}

func nameTables(p *profile.Profile) gen3.NameTables {
	return gen3.NameTables{Species: uint32(p.SpeciesNames), Moves: uint32(p.MoveNames), Items: uint32(p.ItemNames),
		SpeciesLength: p.SpeciesNameLength, MoveLength: p.MoveNameLength, ItemSize: p.ItemSize}
}

// game returns the running game and its profile, for HTTP handlers.
func (s *Server) game() (*emulator.Running, *profile.Profile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running, s.prof
}

// member adds names and a sprite from the ROM, leaving any that fail blank
// so the UI can fall back to the ID.
func (s *Server) member(mon *gen3.Pokemon) *Member {
	m := &Member{Pokemon: mon, MoveNames: make([]string, len(mon.Moves))}
	logErr := func(what string, err error) {
		if err != nil {
			log.Printf("looking up %s name: %v", what, err)
		}
	}

	var err error
	m.SpeciesName, err = s.names.Species(mon.Species)
	logErr("species", err)
	if mon.HeldItem != 0 {
		m.ItemName, err = s.names.Item(mon.HeldItem)
		logErr("item", err)
	}
	for i, move := range mon.Moves {
		m.MoveNames[i], err = s.names.Move(move.ID)
		logErr("move", err)
	}
	if mon.Ability == 0 && !mon.IsEgg {
		m.AbilityName = s.abilityName(mon)
	}
	m.Sprite = s.sprite(mon)
	return m
}

// abilityName names a Gen 3 Pokémon's ability from its species and slot,
// for games whose profile has species data.
func (s *Server) abilityName(mon *gen3.Pokemon) string {
	if s.prof.SpeciesData == nil || s.prof.Experience == nil {
		return ""
	}
	if s.species == nil {
		t, err := speciesTables(s.prof, s.read)
		if err != nil {
			return ""
		}
		s.species = t
	}
	return s.species.Ability(s.read, mon.Species, mon.AbilitySlot)
}

// sprite returns the URL of a party Pokémon's sprite.
func (s *Server) sprite(mon *gen3.Pokemon) string {
	if mon.IsEgg {
		return s.spriteURL(gen3.EggSpecies, false)
	}
	return s.spriteURL(mon.Species, mon.IsShiny)
}

// spriteURL renders a sprite on first sight and returns its URL, or "" if
// it can't be rendered.
func (s *Server) spriteURL(species int, shiny bool) string {
	s.mu.Lock()
	key := fmt.Sprintf("%d/%d", s.generation, species)
	if shiny {
		key += "s"
	}
	png, seen := s.sprites[key]
	s.mu.Unlock()

	if !seen {
		png = s.renderSprite(species, shiny)
		s.mu.Lock()
		s.sprites[key] = png
		s.mu.Unlock()
	}
	if png == nil {
		return ""
	}
	return "/api/sprite/" + key
}

func (s *Server) renderSprite(species int, shiny bool) []byte {
	img, err := gen3.FrontSprite(s.read, species, shiny)
	if err != nil {
		log.Printf("species %d sprite: %v", species, err)
		return nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Printf("species %d sprite: %v", species, err)
		return nil
	}
	return buf.Bytes()
}

func (s *Server) resetSprites() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sprites) > 0 {
		s.sprites = map[string][]byte{}
		s.generation++
	}
}

// update stores the new state and wakes waiting clients, but only if it changed.
func (s *Server) update(st State) {
	b, err := json.Marshal(st)
	if err != nil {
		log.Printf("encoding state: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if string(b) == string(s.state) {
		return
	}
	s.state = b
	close(s.changed)
	s.changed = make(chan struct{})
}

func (s *Server) current() ([]byte, chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.changed
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFiles, "static")
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("GET /api/party", s.handleParty)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("GET /api/sprite/{gen}/{key}", s.handleSprite)
	mux.HandleFunc("GET /api/saves", s.handleSaves)
	mux.HandleFunc("GET /api/saves/download", s.handleSaveDownload)
	mux.HandleFunc("POST /api/saves/restore", s.handleSaveRestore)
	mux.HandleFunc("POST /api/saves/upload", s.handleSaveUpload)
	mux.HandleFunc("GET /api/saves/view", s.handleSaveView)
	mux.HandleFunc("GET /api/boxes", s.handleBoxes)
	mux.HandleFunc("POST /api/settings/nuzlocke", s.handleNuzlocke)
	mux.HandleFunc("GET /api/library", s.handleLibrary)
	mux.HandleFunc("POST /api/library/patch", s.handleLibraryPatch)
	mux.HandleFunc("GET /api/library/gamelist", s.handleGamelist)
	mux.HandleFunc("POST /api/library/gamelist", s.handleGamelist)
	mux.HandleFunc("GET /api/ds/sprite", s.handleDSSprite)
	mux.HandleFunc("GET /api/species", s.handleSpecies)
	mux.HandleFunc("GET /api/trainers", s.handleTrainers)
	mux.HandleFunc("GET /api/export", s.handleExport)
	mux.HandleFunc("GET /api/map", s.handleMap)
	mux.HandleFunc("GET /api/update", s.handleUpdate)
	mux.HandleFunc("POST /api/update/apply", s.handleUpdateApply)
	mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("POST /api/auth/pin", s.handleSetPIN)
	mux.HandleFunc("GET /api/setup", s.handleSetup)
	mux.HandleFunc("POST /api/setup/game", s.handleSetupGame)
	mux.HandleFunc("POST /api/setup/fix", s.handleSetupFix)
	mux.HandleFunc("GET /api/cloud", s.handleCloud)
	mux.HandleFunc("POST /api/cloud/link", s.handleCloudLink)
	mux.HandleFunc("POST /api/cloud/unlink", s.handleCloudUnlink)
	mux.HandleFunc("POST /api/cloud/upload", s.handleCloudUpload)
	mux.HandleFunc("GET /api/saves/sprite", s.handleROMSprite)
	return s.protect(mux)
}

func (s *Server) handleParty(w http.ResponseWriter, r *http.Request) {
	state, _ := s.current()
	w.Header().Set("Content-Type", "application/json")
	w.Write(state)
}

func (s *Server) handleSprite(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	img := s.sprites[r.PathValue("gen")+"/"+r.PathValue("key")]
	s.mu.Unlock()
	if img == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(img)
}

// handleEvents streams the state now and after every change.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)

	keepAlive := time.NewTicker(30 * time.Second)
	defer keepAlive.Stop()

	for {
		state, changed := s.current()
		if _, err := fmt.Fprintf(w, "data: %s\n\n", state); err != nil {
			return
		}
		rc.Flush()

	wait:
		for {
			select {
			case <-changed:
				break wait
			case <-keepAlive.C:
				fmt.Fprint(w, ": keep-alive\n\n")
				rc.Flush()
			case <-r.Context().Done():
				return
			}
		}
	}
}
