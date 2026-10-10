package dashboard

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"tallgrass/emulator"
	"tallgrass/gen4"
	"tallgrass/profile"
	"tallgrass/saves"
)

// EnableSaves adds the save backup API, backed by a running watcher.
// profiles are matched against ROM files to view saves.
func (s *Server) EnableSaves(w *saves.Watcher, profiles []*profile.Profile) {
	s.saves = w
	if profiles != nil {
		s.profiles = profiles
	}
}

// EnableDetection makes the server follow whichever game the emulator has
// open, switching to its profile by ROM hash. profiles are all known ones.
func (s *Server) EnableDetection(detect func() (*emulator.Running, error), profiles []*profile.Profile) {
	s.detect, s.profiles = detect, profiles
}

func (s *Server) gameRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connected
}

// saveInUse reports whether a save's game is open, so its emulator would
// overwrite the file on exit. Without detection, any running game counts.
func (s *Server) saveInUse(rel string) bool {
	if s.detect == nil {
		return s.gameRunning()
	}
	running, _ := s.game()
	if running == nil {
		return false
	}
	rom, err := s.saves.ROMFor(rel)
	return err != nil || rom == running.ROM // unsure: be safe
}

func (s *Server) handleSaves(w http.ResponseWriter, r *http.Request) {
	if s.saves == nil {
		http.Error(w, "save backups are off (no ROMs folder)", http.StatusNotFound)
		return
	}
	files, err := s.saves.Files()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type file struct {
		saves.SaveFile
		InUse bool `json:"inUse"` // its game is open, so restoring is refused
	}
	out := make([]file, len(files))
	for i, f := range files {
		out[i] = file{f, s.saveInUse(f.Path)}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"files": out})
}

// handleSaveDownload serves one version: ?file=gba/Game.srm&version=<id>.
func (s *Server) handleSaveDownload(w http.ResponseWriter, r *http.Request) {
	if s.saves == nil {
		http.NotFound(w, r)
		return
	}
	file, version := r.URL.Query().Get("file"), r.URL.Query().Get("version")
	data, err := s.saves.Read(file, version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	// e.g. "Game 20261009T142530Z.srm", so downloads don't overwrite each other.
	ext := path.Ext(file)
	name := fmt.Sprintf("%s %s%s", path.Base(file)[:len(path.Base(file))-len(ext)], version[:16], ext)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Write(data)
}

// handleSaveRestore takes {"file": ..., "version": ...}. Requiring a JSON
// body means another website can't trigger it with a plain form post.
func (s *Server) handleSaveRestore(w http.ResponseWriter, r *http.Request) {
	if s.saves == nil {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	var req struct{ File, Version string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err := s.saves.Restore(req.File, req.Version, s.saveInUse(req.File))
	switch {
	case errors.Is(err, saves.ErrGameRunning):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleSaveUpload replaces a save with an uploaded one: POST
// /api/saves/upload?file=<path>, the save's bytes as
// application/octet-stream (which a plain form on another site can't send).
// The upload must be a save of the same kind; the current file is backed
// up first, and nothing is written while that game is running.
func (s *Server) handleSaveUpload(w http.ResponseWriter, r *http.Request) {
	if s.saves == nil {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Content-Type") != "application/octet-stream" {
		http.Error(w, "expected application/octet-stream", http.StatusUnsupportedMediaType)
		return
	}
	rel := r.URL.Query().Get("file")
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		http.Error(w, "the file is too big to be a save", http.StatusRequestEntityTooLarge)
		return
	}
	current, err := s.saves.ReadCurrent(rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err = checkUpload(rel, current, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = s.saves.Replace(rel, data, s.saveInUse(rel))
	switch {
	case errors.Is(err, saves.ErrGameRunning):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// checkUpload makes sure an upload is a save the game will load in place of
// current, and returns it ready to write. DraStic saves (.dsv) end with a
// footer that save editors drop; a bare 512 KB save gets current's back.
func checkUpload(rel string, current, data []byte) ([]byte, error) {
	if strings.EqualFold(filepath.Ext(rel), ".dsv") {
		const raw = 512 << 10
		if len(data) == raw && len(current) > raw {
			data = append(append([]byte{}, data...), current[raw:]...)
		}
		if _, err := gen4.ParseSave(data); err != nil {
			return nil, fmt.Errorf("that isn't a DS Pokémon save: %w", err)
		}
		if len(data) != len(current) {
			return nil, fmt.Errorf("that save is %d bytes; this one is %d", len(data), len(current))
		}
		return data, nil
	}
	if gen3Sectors(data) < 14 {
		return nil, errors.New("that isn't a Game Boy Advance Pokémon save")
	}
	if len(data) < len(current) {
		// RetroArch's .srm is the full 128 KB; editors may write less.
		data = append(append([]byte{}, data...), make([]byte, len(current)-len(data))...)
	}
	return data, nil
}

// gen3Sectors counts the 4 KB sectors that end with a Gen 3 save footer
// signature (vanilla 0x08012025, Unbound 0x01121999).
func gen3Sectors(data []byte) int {
	n := 0
	for o := 0; o+0x1000 <= len(data) && o < 32*0x1000; o += 0x1000 {
		if sig := binary.LittleEndian.Uint32(data[o+0xFF8:]); sig == 0x08012025 || sig == 0x01121999 {
			n++
		}
	}
	return n
}
