package dashboard

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"tallgrass/cloud"
)

// Cloud sync (optional, off until switched on from the Account page): a
// linked device uploads every save backup version it hasn't sent yet and
// its Journey history to the account, every 10 minutes. Uploaded versions
// are remembered in a small file next to the binary.

const syncSetting = "cloud.sync"

type syncState struct {
	run      sync.Mutex // held for a whole sync; only syncOnce touches Uploaded and Journey
	mu       sync.Mutex // guards the fields the Account page reads (LastAt, LastErr, Sent, enabled)
	file     string
	Uploaded map[string]bool `json:"uploaded"`    // "gba/Game.srm|<version>"
	Journey  string          `json:"journeyHash"` // SHA-256 of the last Journey sent
	LastAt   time.Time       `json:"lastAt"`
	LastErr  string          `json:"lastError"`
	Sent     int             `json:"sent"` // versions uploaded in all
	enabled  bool
}

// SyncView is the Account page's cloud backup status.
type SyncView struct {
	Enabled bool      `json:"enabled"`
	LastAt  time.Time `json:"lastAt,omitzero"`
	Error   string    `json:"error,omitempty"`
	Sent    int       `json:"sent"`
}

// EnableCloudSync starts the sync loop; stateFile remembers what's been sent.
func (s *Server) EnableCloudSync(stateFile string) {
	if s.cloud == nil {
		return
	}
	st := &syncState{file: stateFile, Uploaded: map[string]bool{}}
	if b, err := os.ReadFile(stateFile); err == nil {
		json.Unmarshal(b, st)
		if st.Uploaded == nil {
			st.Uploaded = map[string]bool{}
		}
	}
	if s.store != nil {
		v, _ := s.store.Setting("", syncSetting)
		st.enabled = v == "on"
	}
	s.cloud.sync = st
	go func() {
		time.Sleep(time.Minute)
		for {
			s.syncOnce()
			time.Sleep(10 * time.Minute)
		}
	}()
}

func (st *syncState) save() {
	st.mu.Lock()
	b, err := json.Marshal(st)
	st.mu.Unlock()
	if err == nil {
		err = os.WriteFile(st.file, b, 0o600)
	}
	if err != nil {
		log.Printf("saving cloud sync state: %v", err)
	}
}

// syncOnce uploads whatever's new. Errors are kept for the Account page.
func (s *Server) syncOnce() {
	st := s.cloud.sync
	st.run.Lock()
	defer st.run.Unlock()
	st.mu.Lock()
	enabled := st.enabled
	st.mu.Unlock()
	if !enabled {
		return
	}
	link, err := cloud.LoadLink(s.cloud.linkPath)
	if err != nil || link == nil {
		return // not linked: nothing to do
	}
	fail := func(err error) {
		msg := err.Error()
		if errors.Is(err, cloud.ErrUnlinked) {
			msg = "This device was unlinked from your account."
		}
		st.mu.Lock()
		st.LastErr = msg
		st.mu.Unlock()
		st.save()
	}

	if s.saves != nil {
		files, err := s.saves.Files()
		if err != nil {
			fail(err)
			return
		}
		for _, f := range files {
			for _, v := range f.Versions {
				key := f.Path + "|" + v.ID
				if st.Uploaded[key] {
					continue
				}
				data, err := s.saves.Read(f.Path, v.ID)
				if err != nil {
					fail(err)
					return
				}
				if err := s.cloud.client.UploadSave(link.Token, f.Path, strings.TrimSuffix(v.ID, ".gz"), data); err != nil {
					fail(err)
					return
				}
				st.mu.Lock()
				st.Uploaded[key] = true
				st.Sent++
				st.mu.Unlock()
				st.save() // after each, so an interruption doesn't resend everything
			}
		}
	}

	if s.store != nil {
		runs, err := s.store.Export()
		if err != nil {
			fail(err)
			return
		}
		b, _ := json.Marshal(map[string]any{"schema": "tallgrass.journey", "version": 1, "runs": runs})
		sum := sha256.Sum256(b)
		if h := string(sum[:]); h != st.Journey {
			if err := s.cloud.client.UploadJourney(link.Token, b); err != nil {
				fail(err)
				return
			}
			st.Journey = h
		}
	}
	st.mu.Lock()
	st.LastAt, st.LastErr = time.Now(), ""
	st.mu.Unlock()
	st.save()
}

// handleCloudSync: GET the status; POST {"enabled": bool} switches it.
func (s *Server) handleCloudSync(w http.ResponseWriter, r *http.Request) {
	if s.cloud == nil || s.cloud.sync == nil {
		http.Error(w, "cloud sync is off", http.StatusNotFound)
		return
	}
	st := s.cloud.sync
	if r.Method == http.MethodPost {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
			return
		}
		var req struct{ Enabled bool }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		value := "off"
		if req.Enabled {
			value = "on"
		}
		if s.store != nil {
			if err := s.store.SetSetting("", syncSetting, value); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		st.mu.Lock()
		st.enabled = req.Enabled
		st.mu.Unlock()
		if req.Enabled {
			go s.syncOnce() // start now rather than in up to 10 minutes
		}
	}
	st.mu.Lock()
	v := SyncView{Enabled: st.enabled, LastAt: st.LastAt, Error: st.LastErr, Sent: st.Sent}
	st.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
