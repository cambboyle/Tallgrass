package dashboard

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"tallgrass/autoprofile"
	"tallgrass/gen3"
	"tallgrass/profile"
)

// SaveView is a parsed save file, as the box viewer shows it.
type SaveView struct {
	File    string `json:"file"`
	Version string `json:"version"` // backup ID, or "" for the file on disk
	ROM     string `json:"rom"`     // ROM id, for species pages
	Game    string `json:"game"`
	Counter uint32 `json:"counter"` // in-game saves so far
	Trainer struct {
		Name string `json:"name"`
		ID   uint32 `json:"id"`
	} `json:"trainer"`
	Party       []*SaveMon `json:"party"`
	Boxes       []SaveBox  `json:"boxes"`
	CurrentBox  int        `json:"currentBox"`
	HiddenBoxes int        `json:"hiddenBoxes"` // boxes the game has that we can't read yet
	Pokedex     *DexView   `json:"pokedex,omitempty"`
}

type SaveBox struct {
	Name string     `json:"name"`
	Mons []*SaveMon `json:"mons"` // 30 slots, null when empty
}

type SaveMon struct {
	*gen3.Pokemon
	SpeciesName string   `json:"speciesName"`
	ItemName    string   `json:"itemName"`
	MoveNames   []string `json:"moveNames"`
	MetArea     string   `json:"metArea"`
	AbilityName string   `json:"abilityName,omitempty"` // games that store an ability ID (Gen 4+)
	Sprite      string   `json:"sprite"`
	Boxed       bool     `json:"boxed"` // no stats, HP or PP stored
}

type DexView struct {
	Seen   int        `json:"seen"`
	Caught int        `json:"caught"`
	List   []DexEntry `json:"list"` // caught species, by national number
}

type DexEntry struct {
	National int    `json:"national"`
	Name     string `json:"name"`
	Sprite   string `json:"sprite"`
}

// romContext is ROM data read from a ROM file on disk, for viewing saves
// without a running game. Its lock covers the caches.
type romContext struct {
	mu       sync.Mutex
	id       string // SHA-1 prefix, used in sprite URLs
	prof     *profile.Profile
	rom      *gen3.ROMFile
	names    *gen3.Names
	national *gen3.National
	levels   *gen3.Levels
	mapNames uint32
	sprites  map[string][]byte
	species  *gen3.SpeciesTables // built on first species page
	// Whole-ROM indexes for species pages, built once.
	evolvesFrom map[int][]gen3.Evolution
	foundIn     map[int][]gen3.Found
	// Trainers, built on first use.
	trainers       []gen3.Trainer
	trainersPlaced bool
	areaOrder      map[string]int
	maps           map[string]*MapView // by "group.num"; built on first use
	maxSpecies     int                 // counted from names when the profile doesn't say
}

// romKey identifies one version of a ROM file. mod is Unix nanoseconds:
// time.Time values don't compare equal with == after a JSON round trip.
type romKey struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Mod  int64  `json:"mod"`
}

type romCache struct {
	hashMu sync.Mutex // one hash at a time: it's I/O and CPU heavy on a Pi
	file   string     // where hashes persist across restarts; "" for none

	mu     sync.Mutex
	hashes map[romKey]string           // ROM file version -> SHA-1
	auto   map[string]*profile.Profile // SHA-1 -> generated profile (nil: can't)
	byID   map[string]*romContext      // SHA-1 prefix -> context
	liveID string                      // the running game's ROM, once found
}

// romFor returns the ROM context for a watched save's ROM.
func (s *Server) romFor(save string) (*romContext, error) {
	path, err := s.saves.ROMFor(save)
	if err != nil {
		return nil, err
	}
	rc, sum, err := s.romAt(path)
	if err != nil {
		return nil, err
	}
	if rc == nil {
		return nil, fmt.Errorf("no game profile matches this ROM (SHA-1 %s); add it to a profile's romSha1", sum)
	}
	return rc, nil
}

// liveROM finds the ROM file of the game the dashboard is following, by
// hashing ROM files until one matches its profile.
func (s *Server) liveROM() (*romContext, error) {
	if s.detect != nil {
		running, _ := s.game()
		if running == nil {
			return nil, errNoGame
		}
		if !running.Live() {
			return nil, errNotLive
		}
		rc, sum, err := s.romAt(running.ROM)
		if err != nil {
			return nil, err
		}
		if rc == nil {
			return nil, fmt.Errorf("no game profile matches %s (SHA-1 %s) yet", filepath.Base(running.ROM), sum)
		}
		return rc, nil
	}

	if s.saves == nil {
		return nil, fmt.Errorf("no ROMs folder")
	}
	_, prof := s.game()
	s.romCache.mu.Lock()
	rc := s.romCache.byID[s.romCache.liveID]
	s.romCache.mu.Unlock()
	if rc != nil {
		return rc, nil
	}

	for _, rom := range s.saves.ROMs() {
		path := rom.Path
		rc, _, err := s.romAt(path)
		if err != nil {
			log.Printf("hashing %s: %v", path, err)
			continue
		}
		if rc != nil && prof != nil && rc.prof.Name == prof.Name {
			s.romCache.mu.Lock()
			s.romCache.liveID = rc.id
			s.romCache.mu.Unlock()
			return rc, nil
		}
	}
	return nil, fmt.Errorf("no ROM file in the ROMs folder matches the %s profile's romSha1", profileName(prof))
}

// latestSaveView shows the running game's save file as it is on disk, for
// games whose PC can't be read live.
func (s *Server) latestSaveView() (*SaveView, error) {
	running, _ := s.game()
	if running == nil || s.saves == nil {
		return nil, errNoGame
	}
	saves := s.saves.SaveFor(running.ROM)
	if len(saves) == 0 {
		return nil, errors.New("this game has no save yet")
	}
	file := saves[0]
	data, err := s.saves.ReadCurrent(file)
	if err != nil {
		return nil, err
	}
	var view *SaveView
	if isDS(file) {
		view, err = s.dsView(file, data)
	} else {
		return nil, errNotLive
	}
	if err != nil {
		return nil, err
	}
	view.File, view.Version = file, "latest-save"
	return view, nil
}

var (
	errNoGame  = errors.New("No game is running.")
	errNotLive = errors.New("Live boxes aren't available for this emulator yet (DS games run in DraStic). View the save from the Saves tab instead.")
)

// cachedHash returns a ROM's SHA-1 if it's already known, without hashing.
func (s *Server) cachedHash(path string) (romKey, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return romKey{}, "", err
	}
	key := romKey{path, info.Size(), info.ModTime().UnixNano()}
	s.romCache.mu.Lock()
	defer s.romCache.mu.Unlock()
	return key, s.romCache.hashes[key], nil
}

// hashROM returns a ROM file's SHA-1, hashing it once per file version.
func (s *Server) hashROM(path string) (string, error) {
	key, sum, err := s.cachedHash(path)
	if err != nil || sum != "" {
		return sum, err
	}

	s.romCache.hashMu.Lock()
	defer s.romCache.hashMu.Unlock()
	if _, sum, _ := s.cachedHash(path); sum != "" {
		return sum, nil // hashed while we waited
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	sum = hex.EncodeToString(h.Sum(nil))

	s.romCache.mu.Lock()
	if s.romCache.hashes == nil {
		s.romCache.hashes = map[romKey]string{}
	}
	s.romCache.hashes[key] = sum
	s.romCache.mu.Unlock()
	s.saveHashes()
	return sum, nil
}

type hashEntry struct {
	romKey
	SHA1 string `json:"sha1"`
}

// EnableHashCache keeps ROM hashes in a JSON file, so restarts don't
// re-read gigabytes of ROMs.
func (s *Server) EnableHashCache(path string) {
	s.romCache.file = path
	data, err := os.ReadFile(path)
	if err != nil {
		return // first run
	}
	var entries []hashEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("ignoring %s: %v", path, err)
		return
	}
	s.romCache.mu.Lock()
	defer s.romCache.mu.Unlock()
	if s.romCache.hashes == nil {
		s.romCache.hashes = map[romKey]string{}
	}
	for _, e := range entries {
		s.romCache.hashes[e.romKey] = e.SHA1
	}
}

func (s *Server) saveHashes() {
	if s.romCache.file == "" {
		return
	}
	s.romCache.mu.Lock()
	entries := make([]hashEntry, 0, len(s.romCache.hashes))
	for k, v := range s.romCache.hashes {
		entries = append(entries, hashEntry{k, v})
	}
	s.romCache.mu.Unlock()
	data, _ := json.MarshalIndent(entries, "", "  ")
	tmp := s.romCache.file + ".tmp"
	err := os.WriteFile(tmp, data, 0o644)
	if err == nil {
		err = os.Rename(tmp, s.romCache.file)
	}
	if err != nil {
		log.Printf("saving ROM hashes: %v", err)
	}
}

// profileFor finds the profile for a ROM: a bundled one listing its hash,
// or one generated from the ROM itself for FireRed-engine hacks; nil if
// neither works.
func (s *Server) profileFor(sum, path string) *profile.Profile {
	for _, p := range s.profiles {
		for _, want := range p.ROMSHA1 {
			if want == sum {
				return p
			}
		}
	}
	if sum == "" || !strings.EqualFold(filepath.Ext(path), ".gba") {
		return nil
	}

	s.romCache.mu.Lock()
	p, tried := s.romCache.auto[sum]
	s.romCache.mu.Unlock()
	if tried {
		return p
	}
	p = generateProfile(sum, path)
	s.romCache.mu.Lock()
	if s.romCache.auto == nil {
		s.romCache.auto = map[string]*profile.Profile{}
	}
	s.romCache.auto[sum] = p
	s.romCache.mu.Unlock()
	return p
}

// generateProfile builds an automatic profile for a FireRed-engine ROM
// (game code BPRE); nil for anything else.
func generateProfile(sum, path string) *profile.Profile {
	rom, err := gen3.OpenROM(path)
	if err != nil {
		return nil
	}
	defer rom.Close()
	code, err := rom.Read(0x080000AC, 4)
	if err != nil || string(code) != "BPRE" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	p, err := autoprofile.ForFireRed(name, sum, rom.Read, int(info.Size()))
	if err != nil {
		log.Printf("no automatic profile for %s: %v", filepath.Base(path), err)
		return nil
	}
	log.Printf("generated a profile for %s", filepath.Base(path))
	return p
}

// romAt returns the ROM context for a ROM file, or nil and the hash if no
// profile matches.
func (s *Server) romAt(path string) (*romContext, string, error) {
	sum, err := s.hashROM(path)
	if err != nil {
		return nil, "", err
	}

	s.romCache.mu.Lock()
	defer s.romCache.mu.Unlock()
	if s.romCache.byID == nil {
		s.romCache.byID = map[string]*romContext{}
	}
	if rc, ok := s.romCache.byID[sum[:12]]; ok {
		return rc, sum, nil
	}

	prof := s.profileFor(sum, path)
	if prof == nil {
		return nil, sum, nil
	}
	if prof.SaveFile == nil {
		return nil, sum, fmt.Errorf("profile %q doesn't describe its save file layout", prof.Name)
	}
	rc, err := newROMContext(path, sum[:12], prof)
	if err != nil {
		return nil, sum, err
	}
	s.romCache.byID[rc.id] = rc
	return rc, sum, nil
}

func newROMContext(path, id string, prof *profile.Profile) (*romContext, error) {
	rom, err := gen3.OpenROM(path)
	if err != nil {
		return nil, err
	}
	read := rom.Read
	rc := &romContext{id: id, prof: prof, rom: rom, sprites: map[string][]byte{},
		names: gen3.NewNames(read, nameTables(prof))}

	if dex := prof.Pokedex; dex != nil {
		table, err := dex.SpeciesToNational.Resolve(read)
		if err != nil {
			return nil, err
		}
		rc.national = gen3.NewNational(read, table)
	}
	if exp := prof.Experience; exp != nil {
		if rc.levels, err = newLevels(read, exp); err != nil {
			return nil, err
		}
	}
	if loc := prof.Location; loc != nil {
		if rc.mapNames, err = loc.MapNames.Resolve(read); err != nil {
			return nil, err
		}
	}
	return rc, nil
}

// handleSaveView parses ?file=gba/Game.srm&version=<backup ID, or empty for
// the file on disk now>.
func (s *Server) handleSaveView(w http.ResponseWriter, r *http.Request) {
	if s.saves == nil {
		http.NotFound(w, r)
		return
	}
	file, version := r.URL.Query().Get("file"), r.URL.Query().Get("version")
	var data []byte
	var err error
	if version == "" {
		data, err = s.saves.ReadCurrent(file)
	} else {
		data, err = s.saves.Read(file, version)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if isDS(file) {
		view, err := s.dsView(file, data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		view.File, view.Version = file, version
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(view)
		return
	}

	rc, err := s.romFor(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	save, err := gen3.ParseSave(data, rc.layout())
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	rc.mu.Lock()
	view, err := rc.view(save)
	rc.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	view.File, view.Version = file, version
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(view)
}

// handleBoxes reads the PC live from game RAM, so it's current even before
// RetroArch writes the save file. Names and sprites come from the ROM file.
func (s *Server) handleBoxes(w http.ResponseWriter, r *http.Request) {
	if !s.gameRunning() && s.detect == nil {
		http.Error(w, errNoGame.Error(), http.StatusServiceUnavailable)
		return
	}
	rc, err := s.liveROM()
	if errors.Is(err, errNotLive) {
		// DS games: read the PC from DraStic's memory, or failing that show
		// the latest save.
		if view, liveErr := s.liveDSView(); liveErr == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(view)
			return
		} else if !errors.Is(liveErr, errNotLive) {
			log.Printf("live DS boxes: %v", liveErr)
		}
		if view, saveErr := s.latestSaveView(); saveErr == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(view)
			return
		}
	}
	if errors.Is(err, errNoGame) || errors.Is(err, errNotLive) {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	sf := rc.prof.SaveFile
	if sf.RAM == nil {
		http.Error(w, fmt.Sprintf("profile %q doesn't say where its save blocks are in RAM", rc.prof.Name), http.StatusUnprocessableEntity)
		return
	}

	// The RAM blocks are laid out exactly like the save file's, so read as
	// many bytes as the save's sectors hold and reuse the save parser.
	size := int(sf.SectorDataSize)
	read := func(ref profile.Ref, n int) []byte {
		if err != nil {
			return nil
		}
		var addr uint32
		if addr, err = ref.Resolve(s.read); err != nil {
			return nil
		}
		var b []byte
		b, err = s.read(addr, n)
		return b
	}
	save := &gen3.Save{
		PartyOffset: int(sf.PartyOffset),
		PartyFormat: rc.prof.PokemonFormat,
		Block2:      read(sf.RAM.SaveBlock2, size),
		Block1:      read(sf.RAM.SaveBlock1, 4*size),
		Storage:     read(sf.RAM.Storage, 9*size),
	}
	if len(sf.ExtraSectors) > 0 && sf.RAM.Extra.Addr != 0 {
		save.Extra = read(sf.RAM.Extra, len(sf.ExtraSectors)*size)
	}
	if err != nil {
		http.Error(w, "reading game memory: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	// The save block's party copy is only updated when the game saves, so
	// use the live party (as the Party page does).
	prof := rc.prof
	partyRaw, partyErr := s.read(uint32(prof.PartyAddress), prof.PartySlots*prof.PokemonSize)

	rc.mu.Lock()
	view, err := rc.view(save)
	if err == nil && partyErr == nil {
		view.Party = []*SaveMon{}
		for slot := 0; slot < prof.PartySlots; slot++ {
			p, err := gen3.DecodeAs(prof.PokemonFormat, partyRaw[slot*prof.PokemonSize:(slot+1)*prof.PokemonSize], prof.EncryptedData)
			if err == nil && p != nil {
				view.Party = append(view.Party, rc.mon(p, false))
			}
		}
	}
	rc.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	view.Version = "live"
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(view)
}

func (rc *romContext) layout() gen3.SaveLayout {
	sf := rc.prof.SaveFile
	return gen3.SaveLayout{
		Signature:      uint32(sf.Signature),
		PartyOffset:    int(sf.PartyOffset),
		SectorDataSize: int(sf.SectorDataSize), VerifyChecksums: sf.VerifyChecksums,
		Block2Size: int(sf.Block2Size), Block1Size: int(sf.Block1Size), StorageSize: int(sf.StorageSize),
		ExtraSectors: sf.ExtraSectors, PartyFormat: rc.prof.PokemonFormat,
	}
}

// newLevels builds the level-from-experience calculator a profile describes.
func newLevels(read gen3.ReadMemory, exp *profile.Experience) (*gen3.Levels, error) {
	base, err := exp.BaseStats.Resolve(read)
	if err != nil {
		return nil, err
	}
	table, err := exp.Table.Resolve(read)
	if err != nil {
		return nil, err
	}
	l := gen3.NewLevels(read, base, table, exp.RowLength).WithRecord(exp.BaseStatsSize, exp.GrowthOffset)
	if exp.SharedGrowthSpecies != 0 {
		l.SameGrowthAs(exp.SharedGrowthSpecies)
	}
	return l, nil
}

// readBox reads box n (from 0): PC storage for the first sf.Boxes, then any
// extra groups the profile describes. ok is false for a box we can't place.
func readBox(save *gen3.Save, sf *profile.SaveFile, n int, format gen3.BoxFormat) (mons []*gen3.Pokemon, ok bool, err error) {
	if n < sf.Boxes {
		mons, err = save.Box(n, format)
		return mons, true, err
	}
	block, offset, ok := sf.Place(n + 1)
	if !ok {
		return nil, false, nil
	}
	mons, err = save.BoxAt(block, offset, format)
	return mons, true, err
}

// view builds the viewer's data from a save, whether parsed from a file or
// assembled from game RAM.
func (rc *romContext) view(save *gen3.Save) (*SaveView, error) {
	sf := rc.prof.SaveFile
	encrypted := rc.prof.EncryptedData

	v := &SaveView{Game: rc.prof.Name, ROM: rc.id, Counter: save.Counter, Party: []*SaveMon{}, Boxes: []SaveBox{},
		CurrentBox: save.CurrentBox()}
	v.Trainer.Name, v.Trainer.ID = save.Trainer()

	party, err := save.Party(encrypted)
	if err != nil {
		return nil, err
	}
	for _, p := range party {
		if p != nil {
			v.Party = append(v.Party, rc.mon(p, false))
		}
	}

	for b := 0; b < max(sf.Boxes, sf.TotalBoxes); b++ {
		mons, ok, err := readBox(save, sf, b, gen3.BoxFormat{Size: sf.BoxMonSize, Encrypted: encrypted})
		if err != nil {
			return nil, err
		}
		if !ok {
			v.HiddenBoxes++ // a box the game has that we can't find yet
			continue
		}
		name := save.BoxName(b, int(sf.BoxNames))
		if sf.BoxNamesInOrder {
			name = save.BoxNameInOrder(b, int(sf.BoxNames))
		}
		box := SaveBox{Name: name, Mons: make([]*SaveMon, len(mons))}
		for i, p := range mons {
			if p != nil {
				box.Mons[i] = rc.mon(p, true)
			}
		}
		v.Boxes = append(v.Boxes, box)
	}

	if dex := rc.prof.Pokedex; dex != nil && rc.national != nil {
		v.Pokedex = rc.dex(save, dex.FlagBytes)
	}
	return v, nil
}

// mon adds names, a sprite and (for boxed Pokémon) the level.
func (rc *romContext) mon(p *gen3.Pokemon, boxed bool) *SaveMon {
	if err := gen3.ResolveForm(rc.rom.Read, uint32(rc.prof.FormSpecies), p); err != nil {
		log.Printf("form of species %d: %v", p.Species, err)
	}
	m := &SaveMon{Pokemon: p, MoveNames: make([]string, len(p.Moves)), Boxed: boxed}
	m.SpeciesName, _ = rc.names.Species(p.Species)
	if p.HeldItem != 0 {
		m.ItemName, _ = rc.names.Item(p.HeldItem)
	}
	for i, mv := range p.Moves {
		m.MoveNames[i], _ = rc.names.Move(mv.ID)
	}
	if rc.mapNames != 0 {
		stride, offset := rc.prof.Location.MapNameTable()
		m.MetArea, _ = rc.names.MapName(rc.mapNames, stride, offset, rc.prof.Location.FirstMapSection, p.MetLocation)
	}
	if boxed && rc.levels != nil && !p.IsEgg {
		if lv, err := rc.levels.Level(p.Species, p.Experience); err == nil {
			p.Level = lv
		} else {
			log.Printf("level for species %d: %v", p.Species, err)
		}
	}
	species, shiny := p.Species, p.IsShiny
	if p.IsEgg {
		species, shiny = gen3.EggSpecies, false
	}
	m.Sprite = rc.spriteURL(species, shiny)
	return m
}

func (rc *romContext) dex(save *gen3.Save, size int) *DexView {
	sf := rc.prof.SaveFile
	flags := func(at profile.BlockOffset) []byte {
		block := save.Block(at.Block)
		off := int(at.Offset)
		if off+size > len(block) {
			return nil
		}
		return block[off : off+size]
	}
	seen, caught := flags(sf.DexSeen), flags(sf.DexCaught)

	d := &DexView{List: []DexEntry{}}
	for n := 1; n <= size*8; n++ {
		if gen3.HasFlag(seen, n) {
			d.Seen++
		}
		if !gen3.HasFlag(caught, n) {
			continue
		}
		d.Caught++
		e := DexEntry{National: n}
		if species, err := rc.national.Species(n); err == nil && species != 0 {
			e.Name, _ = rc.names.Species(species)
			e.Sprite = rc.spriteURL(species, false)
		}
		d.List = append(d.List, e)
	}
	return d
}

func (rc *romContext) spriteURL(species int, shiny bool) string {
	return fmt.Sprintf("/api/saves/sprite?rom=%s&species=%d&shiny=%t", rc.id, species, shiny)
}

// handleROMSprite renders a sprite from a ROM file, cached per ROM. The ROM
// ID is a content hash, so browsers may cache the image for good.
func (s *Server) handleROMSprite(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s.romCache.mu.Lock()
	rc := s.romCache.byID[q.Get("rom")]
	s.romCache.mu.Unlock()
	species, err := strconv.Atoi(q.Get("species"))
	if rc == nil || err != nil || species <= 0 {
		http.NotFound(w, r)
		return
	}
	shiny := q.Get("shiny") == "true"
	key := fmt.Sprintf("%d-%t", species, shiny)

	rc.mu.Lock()
	img, ok := rc.sprites[key]
	if !ok {
		if pic, err := gen3.FrontSprite(rc.rom.Read, species, shiny); err == nil {
			var buf bytes.Buffer
			png.Encode(&buf, pic)
			img = buf.Bytes()
		} else {
			log.Printf("species %d sprite from ROM: %v", species, err)
		}
		rc.sprites[key] = img
	}
	rc.mu.Unlock()

	if img == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(img)
}

// speciesTables resolves a profile's species data against a ROM.
func speciesTables(p *profile.Profile, read gen3.ReadMemory) (*gen3.SpeciesTables, error) {
	sd := p.SpeciesData
	if sd == nil || p.Experience == nil {
		return nil, errors.New("this game's profile has no species data")
	}
	t := &gen3.SpeciesTables{AbilityNameLen: sd.AbilityNameLength, HiddenAbility: sd.HiddenAbility,
		CFRULearnsets: sd.CFRULearnsets, EvosPerSpecies: sd.EvolutionsPerSpecies,
		TMLearnsetSize: sd.TMLearnsetSize, TutorLearnsetSize: sd.TutorLearnsetSize, EvoScheme: sd.EvolutionScheme,
		Format: sd.Format, TypeNameLen: sd.TypeNameLength, LearnsetEntry: sd.LearnsetEntry, TMByteLists: sd.TMByteLists, TMCount: sd.TMCount}
	for _, r := range []struct {
		ref profile.Ref
		out *uint32
	}{{p.Experience.BaseStats, &t.BaseStats}, {sd.TypeNames, &t.TypeNames}, {sd.AbilityNames, &t.AbilityNames},
		{sd.Learnsets, &t.Learnsets}, {sd.Evolutions, &t.Evolutions}} {
		addr, err := r.ref.Resolve(read)
		if err != nil {
			return nil, err
		}
		*r.out = addr
	}
	for _, r := range []struct {
		ref profile.Ref
		out *uint32
	}{{sd.TMMoves, &t.TMMoves}, {sd.TMLearnsets, &t.TMLearnsets}, {sd.TutorMoves, &t.TutorMoves},
		{sd.TutorLearnsets, &t.TutorLearnsets}, {sd.EggMoves, &t.EggMoves}} {
		if r.ref.Addr == 0 {
			continue // optional
		}
		addr, err := r.ref.Resolve(read)
		if err != nil {
			return nil, err
		}
		*r.out = addr
	}
	return t, nil
}
