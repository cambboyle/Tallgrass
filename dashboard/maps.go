package dashboard

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"

	"tallgrass/gen3"
)

// MapView is one map as the Route section and the export show it.
type MapView struct {
	ID          string     `json:"id"` // "group.num"
	Area        string     `json:"area"`
	Connections []MapLink  `json:"connections,omitempty"` // walking off an edge
	Warps       []MapLink  `json:"warps,omitempty"`       // doors, stairs, cave entrances
	Items       []MapItem  `json:"items,omitempty"`
	Shops       [][]string `json:"shops,omitempty"` // item names, in the clerk's order
	// Per shop, per item: badges needed before the clerk sells it (DS
	// standard Poké Marts); nil for shops whose stock never changes.
	ShopBadges [][]int      `json:"shopBadges,omitempty"`
	Trainers   []int        `json:"trainers,omitempty"`
	section    int          // region map section, -1 if unknown
	raw        gen3.MapData // IDs, for the export
}

type MapLink struct {
	Direction string `json:"direction,omitempty"`
	Map       string `json:"map"`
	Area      string `json:"area"`
}

type MapItem struct {
	Item     string `json:"item"`
	Quantity int    `json:"quantity,omitempty"` // when more than 1
	Hidden   bool   `json:"hidden,omitempty"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
}

// buildMaps reads and names every map once. Call with rc.mu held.
func (rc *romContext) buildMaps() {
	if rc.maps != nil {
		return
	}
	rc.maps = map[string]*MapView{}
	area := rc.areaNamer()
	if area == nil {
		return
	}
	groups, err := rc.prof.Location.MapGroups.Resolve(rc.rom.Read)
	if err != nil {
		return
	}
	list, err := gen3.ReadMaps(rc.rom.Read, groups, rc.prof.Location.MapGroupsXor.Addr)
	if err != nil {
		log.Printf("%s: maps: %v", rc.prof.Name, err)
		return
	}
	id := func(g, n int) string { return fmt.Sprintf("%d.%d", g, n) }
	name := func(g, n int) string { a, _ := area(uint8(g), uint8(n)); return a }
	for _, m := range list {
		a, sec := area(uint8(m.Group), uint8(m.Num))
		v := &MapView{ID: id(m.Group, m.Num), Area: a, Trainers: m.Trainers, section: sec, raw: m}
		for _, c := range m.Connections {
			v.Connections = append(v.Connections, MapLink{Direction: c.Direction, Map: id(c.Group, c.Num), Area: name(c.Group, c.Num)})
		}
		for _, w := range m.Warps {
			v.Warps = append(v.Warps, MapLink{Map: id(w.Group, w.Num), Area: name(w.Group, w.Num)})
		}
		for _, it := range m.Items {
			n, _ := rc.names.Item(it.Item)
			mi := MapItem{Item: n, Hidden: it.Hidden, X: it.X, Y: it.Y}
			if it.Quantity > 1 {
				mi.Quantity = it.Quantity
			}
			v.Items = append(v.Items, mi)
		}
		for _, shop := range m.Shops {
			var names []string
			for _, item := range shop {
				if n, _ := rc.names.Item(item); n != "" && !slices.Contains(names, n) {
					names = append(names, n)
				}
			}
			if len(names) > 0 {
				v.Shops = append(v.Shops, names)
			}
		}
		rc.maps[v.ID] = v
	}
}

// MapPage is the Route section's extra detail for one map.
type MapPage struct {
	*MapView
	TrainerList []gen3.Trainer `json:"trainerList"`
	Sprites     map[int]string `json:"sprites"`
}

// handleMap serves GET /api/map?rom=<id>&map=<group>.<num>: the map's
// items, shops, exits and trainers (Gen 3 games whose maps we can read).
func (s *Server) handleMap(w http.ResponseWriter, r *http.Request) {
	id, mapID := r.URL.Query().Get("rom"), r.URL.Query().Get("map")
	s.openROM(id)
	s.ds.mu.Lock()
	d := s.ds.byID[id]
	s.ds.mu.Unlock()
	if d != nil {
		idx := s.dsIndexes(d)
		m := idx.maps[mapID]
		if m == nil {
			http.Error(w, "unknown map", http.StatusNotFound)
			return
		}
		page := &MapPage{MapView: m, TrainerList: []gen3.Trainer{}, Sprites: map[int]string{}}
		for _, t := range idx.trainers {
			if slices.Contains(t.Maps, mapID) {
				page.TrainerList = append(page.TrainerList, t)
				for _, p := range t.Party {
					page.Sprites[p.Species] = s.dsSpriteURL(d, &gen3.Pokemon{Species: p.Species})
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(page)
		return
	}
	s.romCache.mu.Lock()
	rc := s.romCache.byID[id]
	s.romCache.mu.Unlock()
	if rc == nil {
		http.Error(w, "no map data for this game", http.StatusNotFound)
		return
	}
	rc.mu.Lock()
	if rc.trainers == nil {
		if err := rc.buildTrainers(); err != nil {
			log.Printf("%s: trainers: %v", rc.prof.Name, err)
		}
	}
	rc.buildMaps()
	m := rc.maps[mapID]
	page := &MapPage{TrainerList: []gen3.Trainer{}, Sprites: map[int]string{}}
	if m != nil {
		page.MapView = m
		for _, t := range rc.trainers {
			if slices.Contains(t.Maps, mapID) {
				page.TrainerList = append(page.TrainerList, t)
				for _, p := range t.Party {
					page.Sprites[p.Species] = rc.spriteURL(p.Species, false)
				}
			}
		}
	}
	rc.mu.Unlock()
	if m == nil {
		http.Error(w, "unknown map", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}
