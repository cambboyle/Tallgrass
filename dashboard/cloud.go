package dashboard

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"tallgrass/cloud"
)

// cloudState links this device to a Tallgrass account.
type cloudState struct {
	client   *cloud.Client
	linkPath string // where the device key is kept
	siteURL  string // the website, for "enter this code at …"

	mu       sync.Mutex
	pairing  *cloud.Pairing // waiting for the code to be entered
	pairErr  string
	device   *cloud.Device // last answer from the server
	deviceAt time.Time

	sync *syncState // cloud backups; nil until EnableCloudSync
}

// EnableCloud turns on account linking. linkPath stores the device key.
func (s *Server) EnableCloud(c *cloud.Client, linkPath, siteURL string) {
	s.cloud = &cloudState{client: c, linkPath: linkPath, siteURL: siteURL}
}

// CloudView is the Account page's state.
type CloudView struct {
	Linked  bool          `json:"linked"`
	Device  *cloud.Device `json:"device,omitempty"`
	Pairing *struct {
		Code      string    `json:"code"`
		ExpiresAt time.Time `json:"expiresAt"`
	} `json:"pairing,omitempty"`
	Error   string `json:"error,omitempty"`
	SiteURL string `json:"siteUrl"`
}

func (s *Server) handleCloud(w http.ResponseWriter, r *http.Request) {
	if s.cloud == nil {
		http.Error(w, "accounts are off", http.StatusNotFound)
		return
	}
	c := s.cloud
	v := CloudView{SiteURL: c.siteURL}
	link, err := cloud.LoadLink(c.linkPath)
	if err != nil {
		v.Error = err.Error()
	}
	if link != nil {
		v.Linked = true
		c.mu.Lock()
		dev, at := c.device, c.deviceAt
		c.mu.Unlock()
		if dev == nil || time.Since(at) > time.Minute {
			switch d, err := c.client.Device(link.Token); {
			case errors.Is(err, cloud.ErrUnlinked):
				os.Remove(c.linkPath) // unlinked from the website
				v.Linked, dev = false, nil
			case err != nil:
				v.Error = "Couldn't reach Tallgrass: " + err.Error()
			default:
				dev = d
				c.mu.Lock()
				c.device, c.deviceAt = d, time.Now()
				c.mu.Unlock()
			}
		}
		v.Device = dev
	}
	c.mu.Lock()
	if p := c.pairing; p != nil && !v.Linked {
		v.Pairing = &struct {
			Code      string    `json:"code"`
			ExpiresAt time.Time `json:"expiresAt"`
		}{p.Code, p.ExpiresAt}
	}
	if c.pairErr != "" {
		v.Error = c.pairErr
	}
	c.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// handleCloudLink starts pairing: get a code, then poll in the background
// until it's entered on the website or expires.
func (s *Server) handleCloudLink(w http.ResponseWriter, r *http.Request) {
	if s.cloud == nil || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	c := s.cloud
	name, _ := os.Hostname()
	if name == "" {
		name = "Tallgrass device"
	}
	p, err := c.client.StartPairing(name)
	if err != nil {
		http.Error(w, "Couldn't reach Tallgrass: "+err.Error(), http.StatusBadGateway)
		return
	}
	c.mu.Lock()
	c.pairing, c.pairErr = p, ""
	c.mu.Unlock()
	go c.waitForLink(p)
	w.WriteHeader(http.StatusNoContent)
}

func (c *cloudState) waitForLink(p *cloud.Pairing) {
	for time.Now().Before(p.ExpiresAt.Add(30 * time.Second)) {
		time.Sleep(3 * time.Second)
		c.mu.Lock()
		current := c.pairing == p
		c.mu.Unlock()
		if !current {
			return // replaced by a newer code
		}
		status, token, err := c.client.Poll(p)
		if err != nil {
			continue // network blip: keep trying until the code expires
		}
		switch status {
		case "linked":
			err := cloud.SaveLink(c.linkPath, &cloud.Link{Token: token, LinkedAt: time.Now().UTC()})
			c.mu.Lock()
			c.pairing, c.device = nil, nil
			if err != nil {
				c.pairErr = "Linked, but couldn't save the device key: " + err.Error()
			}
			c.mu.Unlock()
			log.Printf("linked to a Tallgrass account")
			return
		case "expired":
			c.mu.Lock()
			c.pairing = nil
			c.mu.Unlock()
			return
		}
	}
	c.mu.Lock()
	if c.pairing == p {
		c.pairing = nil
	}
	c.mu.Unlock()
}

// handleCloudUnlink forgets the device key here. The website lists the
// device until it's removed there too.
func (s *Server) handleCloudUnlink(w http.ResponseWriter, r *http.Request) {
	if s.cloud == nil || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	if err := os.Remove(s.cloud.linkPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.cloud.mu.Lock()
	s.cloud.device = nil
	s.cloud.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// handleCloudUpload exports a ROM's game data and uploads it (maintainers).
// POST {"rom": "<id>"}
func (s *Server) handleCloudUpload(w http.ResponseWriter, r *http.Request) {
	if s.cloud == nil || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	var req struct{ ROM string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	link, err := cloud.LoadLink(s.cloud.linkPath)
	if err != nil || link == nil {
		http.Error(w, "Link this device to your account first (Account page).", http.StatusForbidden)
		return
	}
	gd, err := s.Export(req.ROM)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	body, err := json.Marshal(gd)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	game, err := s.cloud.client.UploadGameData(link.Token, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	log.Printf("uploaded game data for %s", game)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"game": game})
}
