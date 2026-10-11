package dashboard

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"tallgrass/gen3"
)

// LibraryROM is one ROM on the device and what we know about it.
type LibraryROM struct {
	System   string        `json:"system"`
	File     string        `json:"file"`
	Size     int64         `json:"size"`
	SHA1     string        `json:"sha1"`           // "" while it's still being hashed
	Profile  string        `json:"profile"`        // "" if no profile matches
	Verified []string      `json:"verified"`       // what the profile has been checked against; "auto" if generated
	Note     string        `json:"note,omitempty"` // why an unsupported ROM isn't supported
	Viewable bool          `json:"viewable"`       // its saves can be opened in the box viewer
	Running  bool          `json:"running"`
	Saves    []LibrarySave `json:"saves"`
}

type LibrarySave struct {
	Path     string    `json:"path"`
	Modified time.Time `json:"modified"`
	Versions int       `json:"versions"`
}

// handleLibrary lists every ROM. ROMs not hashed yet are hashed in the
// background, one at a time; the page polls until "hashing" is false.
func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	if s.saves == nil {
		http.Error(w, "no ROMs folder", http.StatusNotFound)
		return
	}
	files, err := s.saves.Files()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	saveInfo := map[string]LibrarySave{}
	for _, f := range files {
		saveInfo[f.Path] = LibrarySave{Path: f.Path, Modified: f.Modified, Versions: len(f.Versions)}
	}
	running, _ := s.game()

	var roms []LibraryROM
	var unhashed []string
	for _, rom := range s.saves.ROMs() {
		key, sum, err := s.cachedHash(rom.Path)
		if err != nil {
			continue
		}
		lr := LibraryROM{System: rom.System, File: filepath.Base(rom.Path), Size: key.Size, SHA1: sum,
			Running: running != nil && running.ROM == rom.Path, Saves: []LibrarySave{}}
		if sum == "" {
			unhashed = append(unhashed, rom.Path)
		} else if p := s.profileFor(sum, rom.Path); p != nil {
			lr.Profile, lr.Viewable, lr.Verified = p.Name, p.SaveFile != nil, p.Verified
		} else if rom.System == "nds" {
			// DS games are read by game family, no profile needed.
			if family := dsFamily(rom.Path); family != "" {
				lr.Profile, lr.Viewable = family, true
			}
		}
		if sum != "" && lr.Profile == "" {
			lr.Note = unsupportedReason(rom.Path)
		}
		for _, rel := range s.saves.SaveFor(rom.Path) {
			lr.Saves = append(lr.Saves, saveInfo[rel])
		}
		roms = append(roms, lr)
	}
	if len(unhashed) > 0 {
		s.hashInBackground(unhashed)
	}
	// Still hashing until the background run, which also writes the hash
	// cache file, has finished.
	s.mu.Lock()
	hashing := len(unhashed) > 0 || s.libraryHashing
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"roms": roms, "hashing": hashing})
}

// hashInBackground hashes ROMs unless a background run is already going.
func (s *Server) hashInBackground(paths []string) {
	s.mu.Lock()
	if s.libraryHashing {
		s.mu.Unlock()
		return
	}
	s.libraryHashing = true
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.libraryHashing = false
			s.mu.Unlock()
		}()
		for _, p := range paths {
			if _, err := s.hashROM(p); err != nil {
				log.Printf("hashing %s: %v", p, err)
			}
		}
	}()
}

// unsupportedReason explains, roughly, why a ROM has no profile.
func unsupportedReason(path string) string {
	rom, err := gen3.OpenROM(path)
	if err != nil {
		return ""
	}
	defer rom.Close()
	code, err := rom.Read(0x080000AC, 4)
	if err != nil {
		return ""
	}
	switch string(code) {
	case "BPEE":
		// Hacks built from the pokeemerald decomp change the Pokémon data
		// layout and memory map from build to build.
		ptr, err := rom.Read(0x08000144, 4)
		if err == nil {
			names := le32(ptr)
			if b, err := rom.Read(names+11, 11); err == nil && gen3.DecodeText(b) == "BULBASAUR" {
				return "An Emerald hack without a profile yet."
			}
		}
		return "Built from the pokeemerald decomp, whose data layout changes between builds; not supported yet."
	case "BPRE":
		return "A FireRed hack we couldn't work out automatically."
	}
	return "Not a game we can read yet."
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
