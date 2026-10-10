package dashboard

import (
	"bytes"
	"fmt"
	"image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"tallgrass/gen3"
	"tallgrass/gen4"
)

// dsCache holds Gen 4 ROM data, loaded once per ROM file version. DS games
// need no profile: the game family comes from the ROM's game code.
type dsCache struct {
	mu      sync.Mutex
	indexes map[*gen4.Data]*dsIndexes // built on first species page
	data    map[romKey]*gen4.Data
	byID    map[string]*gen4.Data // ROM SHA-1 prefix -> data, for sprite URLs
	ids     map[*gen4.Data]string
	sprites map[string][]byte // rendered PNGs; nil entry = couldn't render
}

func isDS(save string) bool { return strings.HasPrefix(save, "nds/") }

func isDSROM(path string) bool { return strings.EqualFold(filepath.Ext(path), ".nds") }

func (s *Server) dsData(rom string) (*gen4.Data, error) {
	info, err := os.Stat(rom)
	if err != nil {
		return nil, err
	}
	key := romKey{rom, info.Size(), info.ModTime().UnixNano()}
	s.ds.mu.Lock()
	defer s.ds.mu.Unlock()
	if d, ok := s.ds.data[key]; ok {
		return d, nil
	}
	d, err := gen4.LoadData(rom)
	if err != nil {
		return nil, err
	}
	sum, err := s.hashROM(rom) // usually cached already (Library, game detection)
	if err != nil {
		return nil, err
	}
	if s.ds.data == nil {
		s.ds.data, s.ds.byID, s.ds.ids, s.ds.sprites = map[romKey]*gen4.Data{}, map[string]*gen4.Data{}, map[*gen4.Data]string{}, map[string][]byte{}
	}
	s.ds.data[key] = d
	s.ds.byID[sum[:12]] = d
	s.ds.ids[d] = sum[:12]
	return d, nil
}

// dsSpriteURL points at a Pokémon's sprite from its DS ROM.
func (s *Server) dsSpriteURL(d *gen4.Data, p *gen3.Pokemon) string {
	s.ds.mu.Lock()
	id := s.ds.ids[d]
	s.ds.mu.Unlock()
	if id == "" || p.IsEgg {
		return ""
	}
	return fmt.Sprintf("/api/ds/sprite?rom=%s&species=%d&female=%t&shiny=%t", id, p.Species, p.Female, p.IsShiny)
}

// handleDSSprite renders a DS sprite once and caches it; the ROM ID is a
// content hash, so browsers may cache it for good.
func (s *Server) handleDSSprite(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	species, err := strconv.Atoi(q.Get("species"))
	key := q.Get("rom") + "/" + q.Get("species") + "/" + q.Get("female") + "/" + q.Get("shiny")
	s.ds.mu.Lock()
	d := s.ds.byID[q.Get("rom")]
	img, done := s.ds.sprites[key]
	s.ds.mu.Unlock()
	if d == nil || err != nil {
		http.NotFound(w, r)
		return
	}
	if !done {
		if pic, err := d.Sprite(species, q.Get("female") == "true", q.Get("shiny") == "true"); err == nil {
			var buf bytes.Buffer
			png.Encode(&buf, pic)
			img = buf.Bytes()
		} else {
			log.Printf("DS sprite %d: %v", species, err)
		}
		s.ds.mu.Lock()
		s.ds.sprites[key] = img
		s.ds.mu.Unlock()
	}
	if img == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(img)
}

// dsFamily names a DS ROM's game family, or "" if it isn't one we read.
func dsFamily(rom string) string {
	code, err := gen4.ReadGameCode(rom)
	if err != nil {
		return ""
	}
	l, ok := gen4.LayoutFor(code)
	if !ok {
		return ""
	}
	return "Gen 4 · " + l.Family
}

// dsView builds the box viewer's data from a Gen 4 save.
func (s *Server) dsView(file string, data []byte) (*SaveView, error) {
	rom, err := s.saves.ROMFor(file)
	if err != nil {
		return nil, err
	}
	d, err := s.dsData(rom)
	if err != nil {
		return nil, err
	}
	save, err := gen4.ParseSave(data)
	if err != nil {
		return nil, err
	}
	return s.dsSaveView(rom, d, save)
}

// dsSaveView builds the box viewer's data from a Gen 4 save, read from a
// file or from the running game's RAM.
func (s *Server) dsSaveView(rom string, d *gen4.Data, save *gen4.Save) (*SaveView, error) {
	l := d.Layout

	v := &SaveView{Game: strings.TrimSuffix(filepath.Base(rom), filepath.Ext(rom)), Counter: save.Counter,
		Party: []*SaveMon{}, Boxes: []SaveBox{}}
	s.ds.mu.Lock()
	v.ROM = s.ds.ids[d]
	s.ds.mu.Unlock()
	v.Trainer.Name, v.Trainer.ID = save.Trainer(l)

	party, err := save.Party(l)
	if err != nil {
		return nil, err
	}
	for _, p := range party {
		if p != nil {
			m := dsMon(d, p, false)
			m.Sprite = s.dsSpriteURL(d, p)
			v.Party = append(v.Party, m)
		}
	}
	for b := 0; b < save.Boxes(l); b++ {
		mons, err := save.Box(l, b)
		if err != nil {
			return nil, err
		}
		box := SaveBox{Name: save.BoxName(l, b), Mons: make([]*SaveMon, len(mons))}
		for i, p := range mons {
			if p != nil {
				box.Mons[i] = dsMon(d, p, true)
				box.Mons[i].Sprite = s.dsSpriteURL(d, p)
			}
		}
		v.Boxes = append(v.Boxes, box)
	}
	return v, nil
}

func dsMon(d *gen4.Data, p *gen3.Pokemon, boxed bool) *SaveMon {
	m := &SaveMon{Pokemon: p, Boxed: boxed, MoveNames: make([]string, len(p.Moves)),
		SpeciesName: gen4.Name(d.Species, p.Species), ItemName: gen4.Name(d.Items, p.HeldItem),
		AbilityName: gen4.Name(d.Abilities, p.Ability), MetArea: gen4.Name(d.Locations, p.MetLocation)}
	if p.HeldItem == 0 {
		m.ItemName = ""
	}
	for i, mv := range p.Moves {
		m.MoveNames[i] = gen4.Name(d.Moves, mv.ID)
	}
	if boxed && !p.IsEgg {
		p.Level = d.Level(p.Species, p.Experience)
	}
	return m
}

// dsIndexes are whole-ROM lookups for species pages.
type dsIndexes struct {
	evolvesFrom map[int][]gen3.Evolution
	foundIn     map[int][]gen3.Found // nil if the game's maps can't be read
	trainers    []gen3.Trainer
	maps        map[string]*MapView // by "0.<map ID>"
	areaOrder   map[string]int
}

// placeDS names DS maps and puts trainers on them. DS maps have one
// number; they're "0.<ID>" here so they read like Gen 3's "group.num".
func (idx *dsIndexes) placeDS(d *gen4.Data, maps []gen4.MapInfo) {
	idx.maps, idx.areaOrder = map[string]*MapView{}, map[string]int{}
	key := func(id int) string { return fmt.Sprintf("0.%d", id) }
	area := func(id int) string {
		if id < len(maps) {
			return gen4.Name(d.Locations, maps[id].Section)
		}
		return ""
	}
	byTrainer := map[int][]*MapView{}
	for _, m := range maps {
		v := &MapView{ID: key(m.ID), Area: area(m.ID), Trainers: m.Trainers, section: m.Section,
			raw: gen3.MapData{Num: m.ID, Section: m.Section}}
		for _, w := range m.Warps {
			v.Warps = append(v.Warps, MapLink{Map: key(w), Area: area(w)})
		}
		for _, it := range m.Items {
			mi := MapItem{Item: it.Item, Hidden: it.Hidden, X: it.X, Y: it.Z}
			if it.Quantity > 1 {
				mi.Quantity = it.Quantity
			}
			v.Items = append(v.Items, mi)
		}
		badges := false
		for _, shop := range m.Shops {
			var names []string
			for _, it := range shop.Items {
				names = append(names, gen4.Name(d.Items, it))
			}
			v.Shops = append(v.Shops, names)
			v.ShopBadges = append(v.ShopBadges, shop.Badges)
			badges = badges || shop.Badges != nil
		}
		if !badges {
			v.ShopBadges = nil
		}
		idx.maps[v.ID] = v
		for _, t := range m.Trainers {
			byTrainer[t] = append(byTrainer[t], v)
		}
	}
	for i := range idx.trainers {
		t := &idx.trainers[i]
		for _, v := range byTrainer[t.ID] {
			t.Maps = append(t.Maps, v.ID)
			if !slices.Contains(t.Areas, v.Area) {
				t.Areas = append(t.Areas, v.Area)
			}
			if _, ok := idx.areaOrder[v.Area]; !ok {
				idx.areaOrder[v.Area] = v.section
			}
		}
	}
}
