// Package savesync keeps a folder on a computer in step with a Tallgrass
// device's saves, so desktop save editors (OpenHome, PKHeX) can open them
// like any local file: the device's newest saves are pulled into the
// folder, and files edited there are uploaded back through the dashboard
// API, which backs up first and refuses while that game is running.
//
// Each save's state is two hashes (the same 12-character SHA-256 prefix
// as backup versions): the device version last pulled and the local file
// as last synced. Comparing both with what's there now tells which side
// changed; when both did, the local file is kept under a "conflict" name
// and the device's version is pulled.
//
// The folder is laid out for OpenHome's "suggested saves", which looks at
// files directly in a folder it's given, by extension: GB/GBA saves are
// "<name>.sav" (RetroArch's .srm renamed), DS saves "<name>.dsv". A name
// two saves would share keeps the emulator's own save (.srm) at the top and
// puts the other under "<system>/<original name>".
package savesync

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const hashLen = 12

// StateFile is kept in the synced folder.
const StateFile = ".tallgrass-sync.json"

// ErrPINNeeded means the dashboard has a PIN and there's no valid session.
var ErrPINNeeded = errors.New("the dashboard has a PIN: sign in first")

type fileState struct {
	Remote   string `json:"remote"`             // device version last pulled
	Local    string `json:"local"`              // local file as last synced
	Rejected string `json:"rejected,omitempty"` // local edit the device refused (not retried)
}

type state struct {
	Device  string               `json:"device"`
	Session string               `json:"session,omitempty"`
	Files   map[string]fileState `json:"files"`
}

// Syncer syncs one device with one folder.
type Syncer struct {
	Device string // dashboard URL, e.g. http://192.168.0.227:8080
	Dir    string
	Client *http.Client
	Logf   func(format string, args ...any)
	Now    func() time.Time

	st state
}

// New loads the folder's state (if any) for the given device.
func New(device, dir string) (*Syncer, error) {
	s := &Syncer{Device: strings.TrimRight(device, "/"), Dir: dir, Client: &http.Client{Timeout: 30 * time.Second}, Logf: log.Printf, Now: time.Now}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, StateFile))
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(b, &s.st); err != nil {
			return nil, fmt.Errorf("%s: %w", StateFile, err)
		}
	}
	if s.st.Device != "" && s.st.Device != s.Device {
		return nil, fmt.Errorf("%s is synced with %s; use another folder for %s", dir, s.st.Device, s.Device)
	}
	s.st.Device = s.Device
	if s.st.Files == nil {
		s.st.Files = map[string]fileState{}
	}
	return s, nil
}

func (s *Syncer) save() error {
	b, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(s.Dir, StateFile), b, 0o600)
}

// SignIn exchanges the dashboard PIN for a session, kept in the state file.
func (s *Syncer) SignIn(pin string) error {
	body, _ := json.Marshal(map[string]string{"pin": pin})
	resp, err := s.Client.Post(s.Device+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sign-in failed: %s", strings.TrimSpace(string(msg)))
	}
	for _, c := range resp.Cookies() {
		if c.Name == "tallgrass_session" {
			s.st.Session = c.Value
			return s.save()
		}
	}
	return errors.New("sign-in failed: no session returned")
}

func (s *Syncer) do(method, path, contentType string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequest(method, s.Device+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if s.st.Session != "" {
		req.AddCookie(&http.Cookie{Name: "tallgrass_session", Value: s.st.Session})
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, resp.StatusCode, ErrPINNeeded
	}
	return b, resp.StatusCode, err
}

type remoteFile struct {
	Path     string `json:"path"`
	InUse    bool   `json:"inUse"`
	Versions []struct {
		ID   string `json:"id"`
		Hash string `json:"hash"`
	} `json:"versions"`
}

// Once syncs every save once.
func (s *Syncer) Once() error {
	b, code, err := s.do("GET", "/api/saves", "", nil)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("listing saves: %d %s", code, strings.TrimSpace(string(b)))
	}
	var list struct{ Files []remoteFile }
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	files := slices.DeleteFunc(list.Files, func(f remoteFile) bool { return len(f.Versions) == 0 || !safePath(f.Path) })
	locals := localNames(files)
	for _, f := range files {
		if err := s.syncFile(f, filepath.Join(s.Dir, filepath.FromSlash(locals[f.Path]))); err != nil {
			if errors.Is(err, ErrPINNeeded) {
				return err
			}
			s.Logf("%s: %v", f.Path, err)
		}
	}
	return s.save()
}

// localNames picks each save's path in the folder (see the package doc).
func localNames(files []remoteFile) map[string]string {
	want := map[string][]string{} // flat name → saves wanting it
	for _, f := range files {
		system, name, _ := strings.Cut(f.Path, "/")
		ext := filepath.Ext(name)
		flat := strings.TrimSuffix(name, ext) + ".sav"
		if system == "nds" {
			flat = strings.TrimSuffix(name, ext) + ".dsv"
		}
		want[flat] = append(want[flat], f.Path)
	}
	out := map[string]string{}
	for flat, paths := range want {
		// The emulator's own save first (.srm, .dsv), then by path.
		slices.SortFunc(paths, func(a, b string) int {
			ea, eb := emulatorSave(a), emulatorSave(b)
			if ea != eb {
				if ea {
					return -1
				}
				return 1
			}
			return cmp.Compare(a, b)
		})
		out[paths[0]] = flat
		for _, p := range paths[1:] {
			out[p] = p
		}
	}
	return out
}

func emulatorSave(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".srm" || ext == ".dsv"
}

// safePath accepts "<system>/<file>" with no way out of the folder.
func safePath(p string) bool {
	parts := strings.Split(p, "/")
	if len(parts) != 2 {
		return false
	}
	for _, x := range parts {
		if x == "" || x == "." || x == ".." || strings.ContainsAny(x, `\:`) {
			return false
		}
	}
	return true
}

func (s *Syncer) syncFile(f remoteFile, local string) error {
	newest := f.Versions[0]
	st, known := s.st.Files[f.Path]
	data, err := os.ReadFile(local)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	localHash := ""
	if exists {
		localHash = hashOf(data)
	}

	switch {
	case !exists:
		return s.pull(f.Path, local, newest.ID, newest.Hash)

	case !known && localHash != newest.Hash:
		// A file that was here before syncing started: keep it aside.
		if err := s.setAside(local); err != nil {
			return err
		}
		return s.pull(f.Path, local, newest.ID, newest.Hash)

	case localHash == newest.Hash: // both sides agree
		s.st.Files[f.Path] = fileState{Remote: newest.Hash, Local: localHash}
		return nil

	case localHash != st.Local && localHash == st.Rejected:
		return nil // already refused; wait for another edit or a device change

	case localHash != st.Local && newest.Hash == st.Remote:
		return s.push(f, local, data, localHash)

	case localHash != st.Local: // both changed
		s.Logf("%s changed here and on the device; keeping this copy aside and pulling the device's", f.Path)
		if err := s.setAside(local); err != nil {
			return err
		}
		return s.pull(f.Path, local, newest.ID, newest.Hash)

	case newest.Hash != st.Remote:
		return s.pull(f.Path, local, newest.ID, newest.Hash)
	}
	return nil
}

func (s *Syncer) pull(rel, local, version, hash string) error {
	q := url.Values{"file": {rel}, "version": {version}}
	b, code, err := s.do("GET", "/api/saves/download?"+q.Encode(), "", nil)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("download: %d %s", code, strings.TrimSpace(string(b)))
	}
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		return err
	}
	if err := writeFile(local, b, 0o644); err != nil {
		return err
	}
	s.st.Files[rel] = fileState{Remote: hash, Local: hashOf(b)}
	s.Logf("pulled %s", rel)
	return nil
}

func (s *Syncer) push(f remoteFile, local string, data []byte, hash string) error {
	if f.InUse {
		return nil // the device would refuse; try again once the game is closed
	}
	b, code, err := s.do("POST", "/api/saves/upload?"+url.Values{"file": {f.Path}}.Encode(), "application/octet-stream", data)
	if err != nil {
		return err
	}
	st := s.st.Files[f.Path]
	switch {
	case code >= 200 && code < 300: // the dashboard answers 204
		st.Local = hash
		st.Rejected = ""
		s.Logf("uploaded %s", f.Path)
	case code == http.StatusConflict:
		return nil // its game started meanwhile
	default:
		st.Rejected = hash
		s.Logf("the device refused %s: %s", f.Path, strings.TrimSpace(string(b)))
	}
	s.st.Files[f.Path] = st
	return nil
}

// setAside moves a local file to "Conflicts/<name> <time><ext>", out of
// the way of OpenHome's suggested saves.
func (s *Syncer) setAside(local string) error {
	base := filepath.Base(local)
	ext := filepath.Ext(base)
	aside := filepath.Join(s.Dir, "Conflicts", fmt.Sprintf("%s %s%s", strings.TrimSuffix(base, ext), s.Now().UTC().Format("20060102T150405Z"), ext))
	if err := os.MkdirAll(filepath.Dir(aside), 0o755); err != nil {
		return err
	}
	if err := os.Rename(local, aside); err != nil {
		return err
	}
	s.Logf("kept %s", filepath.Base(aside))
	return nil
}

// Run syncs every interval until the process is stopped (or a PIN is needed).
func (s *Syncer) Run(interval time.Duration) error {
	for {
		if err := s.Once(); err != nil {
			if errors.Is(err, ErrPINNeeded) {
				return err
			}
			s.Logf("%v", err)
		}
		time.Sleep(interval)
	}
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:hashLen]
}

// writeFile replaces a file in one step, so editors never see half of it.
func writeFile(name string, b []byte, perm os.FileMode) error {
	tmp := name + ".tmp"
	if err := os.WriteFile(tmp, b, perm); err != nil {
		return err
	}
	return os.Rename(tmp, name)
}
