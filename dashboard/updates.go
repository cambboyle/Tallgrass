package dashboard

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"tallgrass/update"
)

// Self-update: check GitHub for a newer release now and then, and install
// it when asked. After installing, the process exits and the service
// manager (systemd's Restart=always) starts the new version.
type updateState struct {
	version string // this build's version ("dev" for local builds)
	exe     string

	mu        sync.Mutex
	latest    *update.Release
	checkedAt time.Time
	err       string
	applying  bool
}

// EnableUpdates turns on update checks for this build.
func (s *Server) EnableUpdates(version string) {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("updates off: %v", err)
		return
	}
	u := &updateState{version: version, exe: exe}
	s.updates = u
	go func() {
		time.Sleep(time.Minute)
		for {
			u.check()
			time.Sleep(6 * time.Hour)
		}
	}()
}

func (u *updateState) check() {
	r, err := update.Latest()
	u.mu.Lock()
	defer u.mu.Unlock()
	u.checkedAt = time.Now()
	if err != nil {
		u.err = err.Error()
		return
	}
	u.latest, u.err = r, ""
}

type UpdateView struct {
	Current   string    `json:"current"`
	Latest    string    `json:"latest,omitempty"`
	Available bool      `json:"available"`
	Notes     string    `json:"notes,omitempty"`
	URL       string    `json:"url,omitempty"`
	CheckedAt time.Time `json:"checkedAt,omitempty"`
	Error     string    `json:"error,omitempty"`
	Applying  bool      `json:"applying"`
}

// handleUpdate: GET reports; ?check=1 checks GitHub first.
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	u := s.updates
	if u == nil {
		http.Error(w, "updates are off", http.StatusNotFound)
		return
	}
	if r.URL.Query().Get("check") == "1" {
		u.check()
	}
	u.mu.Lock()
	v := UpdateView{Current: u.version, CheckedAt: u.checkedAt, Error: u.err, Applying: u.applying}
	if u.latest != nil {
		v.Latest, v.Notes, v.URL = u.latest.Version, u.latest.Notes, u.latest.URL
		v.Available = update.Newer(u.latest.Version, u.version)
	}
	u.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// handleUpdateApply installs the latest release and restarts.
func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	u := s.updates
	if u == nil || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	u.mu.Lock()
	latest := u.latest
	if latest == nil || !update.Newer(latest.Version, u.version) || u.applying {
		u.mu.Unlock()
		http.Error(w, "no update to install", http.StatusConflict)
		return
	}
	u.applying = true
	u.mu.Unlock()

	if err := update.Apply(latest, u.exe); err != nil {
		u.mu.Lock()
		u.applying = false
		u.mu.Unlock()
		log.Printf("update to %s failed: %v", latest.Version, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	log.Printf("installed %s; restarting", latest.Version)
	w.WriteHeader(http.StatusNoContent)
	go func() {
		time.Sleep(time.Second) // let the response go out
		os.Exit(0)
	}()
}
