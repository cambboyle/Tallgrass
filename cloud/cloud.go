// Package cloud talks to Tallgrass's Supabase backend: linking this device
// to an account with a pairing code, and uploading game data.
package cloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Public project details: the URL and anon key are designed to be shipped
// in clients; what a caller may do is decided by row level security and the
// Edge Functions.
const (
	DefaultURL     = "https://ifjlueqhuyaafjuzltoi.supabase.co"
	DefaultAnonKey = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6Imlmamx1ZXFodXlhYWZqdXpsdG9pIiwicm9sZSI6ImFub24iLCJpYXQiOjE3OTE1NzY5ODksImV4cCI6MjEwNzE1Mjk4OX0.6Njg2t9WD5F9ttBNU1aQye6smZN2nwufkSFsYyfVUUQ"
)

// ErrUnlinked means the server no longer knows this device's key (it was
// unlinked from the website).
var ErrUnlinked = errors.New("this device isn't linked to an account")

type Client struct {
	URL, AnonKey string
	HTTP         *http.Client
}

func New(url, anonKey string) *Client {
	return &Client{URL: url, AnonKey: anonKey, HTTP: &http.Client{Timeout: 60 * time.Second}}
}

// call posts to an Edge Function. token, if set, is the device key.
func (c *Client) call(method, fn, token string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequest(method, c.URL+"/functions/v1/"+fn, body)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", c.AnonKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusUnauthorized && token != "" {
		return ErrUnlinked
	}
	if res.StatusCode >= 300 {
		var e struct{ Error string }
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("%s: HTTP %d", fn, res.StatusCode)
	}
	return json.Unmarshal(data, out)
}

func (c *Client) postJSON(fn, token string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return c.call(http.MethodPost, fn, token, bytes.NewReader(b), "application/json", out)
}

// Pairing is a code for the user to enter on the website.
type Pairing struct {
	Code       string    `json:"code"`
	PollSecret string    `json:"pollSecret"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

func (c *Client) StartPairing(deviceName string) (*Pairing, error) {
	var p Pairing
	if err := c.postJSON("pair-start", "", map[string]string{"deviceName": deviceName}, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Poll reports "pending", "expired" or "linked" (with the device key).
func (c *Client) Poll(p *Pairing) (status, token string, err error) {
	var r struct {
		Status, Token string
	}
	err = c.postJSON("pair-poll", "", map[string]string{"code": p.Code, "pollSecret": p.PollSecret}, &r)
	return r.Status, r.Token, err
}

// Device is this device as the server sees it.
type Device struct {
	DeviceID string `json:"deviceId"`
	Name     string `json:"name"`
	Account  struct {
		DisplayName  string `json:"displayName"`
		IsMaintainer bool   `json:"isMaintainer"`
	} `json:"account"`
}

func (c *Client) Device(token string) (*Device, error) {
	var d Device
	if err := c.call(http.MethodGet, "device", token, nil, "", &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// UploadGameData sends a game data export (maintainers only).
func (c *Client) UploadGameData(token string, export []byte) (game string, err error) {
	var r struct{ Game string }
	err = c.call(http.MethodPost, "gamedata-upload", token, bytes.NewReader(export), "application/json", &r)
	return r.Game, err
}

// Link is what this device keeps after pairing.
type Link struct {
	Token    string    `json:"token"`
	LinkedAt time.Time `json:"linkedAt"`
}

func LoadLink(path string) (*Link, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var l Link
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// SaveLink writes the device key, readable only by this user.
func SaveLink(path string, l *Link) error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// GameProfile is one profile in the registry.
type GameProfile struct {
	Slug    string          `json:"slug"`
	Version int             `json:"version"`
	Profile json.RawMessage `json:"profile"`
}

// GameProfiles reads the profile registry (public, no account needed).
func (c *Client) GameProfiles() ([]GameProfile, error) {
	req, err := http.NewRequest(http.MethodGet, c.URL+"/rest/v1/game_profiles?select=slug,version,profile&order=slug", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", c.AnonKey)
	req.Header.Set("Authorization", "Bearer "+c.AnonKey)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("profile registry: HTTP %d", res.StatusCode)
	}
	var out []GameProfile
	if err := json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// UploadSave sends one save backup version (the plain save, not gzipped)
// to the account's cloud backups. file is "gba/Game.srm".
func (c *Client) UploadSave(token, file, version string, data []byte) error {
	// Percent-encoded: headers can't carry UTF-8 ("Pokémon …") safely.
	parts := strings.Split(file, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return c.upload(token, map[string]string{"X-Kind": "save", "X-File": strings.Join(parts, "/"), "X-Version": version}, data, "application/octet-stream")
}

// UploadJourney sends the device's whole Journey history (JSON).
func (c *Client) UploadJourney(token string, data []byte) error {
	return c.upload(token, map[string]string{"X-Kind": "journey"}, data, "application/json")
}

func (c *Client) upload(token string, headers map[string]string, data []byte, contentType string) error {
	req, err := http.NewRequest(http.MethodPost, c.URL+"/functions/v1/backup-upload", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("apikey", c.AnonKey)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	if res.StatusCode == http.StatusUnauthorized {
		return ErrUnlinked
	}
	if res.StatusCode >= 300 {
		var e struct{ Error string }
		if json.Unmarshal(body, &e) == nil && e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("backup-upload: HTTP %d", res.StatusCode)
	}
	return nil
}
