package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"

	"tallgrass/emulator"
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
