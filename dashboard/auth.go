package dashboard

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// A dashboard PIN. Until one is set the dashboard is open, as before. Once
// set, every page and API needs a session cookie, which signing in with the
// PIN gives (30 days). The PIN is kept only as a salted PBKDF2 hash, in a
// small file next to the binary; deleting that file removes the PIN.

const (
	sessionCookie = "tallgrass_session"
	sessionLife   = 30 * 24 * time.Hour
	pinIterations = 100_000
)

type authFile struct {
	PINHash  string           `json:"pinHash"` // hex PBKDF2-SHA256
	Salt     string           `json:"salt"`
	Sessions map[string]int64 `json:"sessions"` // SHA-256 of the token -> expiry (Unix)
}

type authState struct {
	path string
	mu   sync.Mutex
	file authFile
	// Failed sign-ins per client address, to slow down guessing.
	fails map[string]*failCount
}

type failCount struct {
	n     int
	until time.Time
}

// EnableAuth turns on PIN protection, kept in path.
func (s *Server) EnableAuth(path string) error {
	a := &authState{path: path, fails: map[string]*failCount{}}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(b, &a.file); err != nil {
			return err
		}
	}
	if a.file.Sessions == nil {
		a.file.Sessions = map[string]int64{}
	}
	s.auth = a
	return nil
}

func (a *authState) save() error {
	b, err := json.MarshalIndent(a.file, "", "  ")
	if err != nil {
		return err
	}
	tmp := a.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.path)
}

func (a *authState) pinSet() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.file.PINHash != ""
}

func hashPIN(pin, salt string) string {
	key, _ := pbkdf2.Key(sha256.New, pin, []byte(salt), pinIterations, 32)
	return hex.EncodeToString(key)
}

func (a *authState) checkPIN(pin string) bool {
	a.mu.Lock()
	hash, salt := a.file.PINHash, a.file.Salt
	a.mu.Unlock()
	return hash != "" && subtle.ConstantTimeCompare([]byte(hashPIN(pin, salt)), []byte(hash)) == 1
}

func randomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// signedIn reports whether the request carries a live session.
func (a *authState) signedIn(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.file.Sessions[tokenHash(c.Value)]
	return ok && time.Now().Unix() < exp
}

func (a *authState) newSession(w http.ResponseWriter) error {
	token := randomString(32)
	a.mu.Lock()
	now := time.Now()
	for k, exp := range a.file.Sessions {
		if exp < now.Unix() {
			delete(a.file.Sessions, k)
		}
	}
	a.file.Sessions[tokenHash(token)] = now.Add(sessionLife).Unix()
	err := a.save()
	a.mu.Unlock()
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Expires: now.Add(sessionLife)})
	return nil
}

// open lists what anyone may load even when a PIN is set: the sign-in page
// and what it needs.
func open(path string) bool {
	switch path {
	case "/login.html", "/style.css", "/favicon.ico", "/api/auth/status", "/api/auth/login":
		return true
	}
	return false
}

// protect wraps the dashboard: with a PIN set, requests need a session.
func (s *Server) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil || !s.auth.pinSet() || open(r.URL.Path) || s.auth.signedIn(r) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, "Sign in with the dashboard PIN first.", http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/login.html?next="+urlPath(r), http.StatusSeeOther)
	})
}

// urlPath is the request's path and query, for coming back after sign-in.
func urlPath(r *http.Request) string {
	p := r.URL.Path
	if r.URL.RawQuery != "" {
		p += "?" + r.URL.RawQuery
	}
	return url.QueryEscape(p)
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	v := map[string]bool{"enabled": s.auth != nil, "pinSet": false, "signedIn": true}
	if s.auth != nil {
		v["pinSet"] = s.auth.pinSet()
		v["signedIn"] = !v["pinSet"] || s.auth.signedIn(r)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// handleLogin: POST {"pin": "..."}. After 5 wrong PINs from one address,
// that address waits 30 s per further attempt.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	var req struct{ PIN string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a := s.auth
	addr := clientAddr(r)
	a.mu.Lock()
	f := a.fails[addr]
	if f != nil && time.Now().Before(f.until) {
		wait := time.Until(f.until).Round(time.Second)
		a.mu.Unlock()
		http.Error(w, "Too many wrong PINs. Try again in "+wait.String()+".", http.StatusTooManyRequests)
		return
	}
	a.mu.Unlock()

	if !a.checkPIN(req.PIN) {
		a.mu.Lock()
		if a.fails[addr] == nil {
			a.fails[addr] = &failCount{}
		}
		f := a.fails[addr]
		f.n++
		if f.n >= 5 {
			f.until = time.Now().Add(30 * time.Second)
		}
		a.mu.Unlock()
		http.Error(w, "That PIN isn't right.", http.StatusUnauthorized)
		return
	}
	a.mu.Lock()
	delete(a.fails, addr)
	a.mu.Unlock()
	if err := a.newSession(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func clientAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.auth != nil {
		if c, err := r.Cookie(sessionCookie); err == nil {
			s.auth.mu.Lock()
			delete(s.auth.file.Sessions, tokenHash(c.Value))
			s.auth.save()
			s.auth.mu.Unlock()
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// handleSetPIN sets, changes or (with an empty new PIN) removes the PIN.
// POST {"pin": "new", "current": "old"}; changing or removing needs the
// current PIN as well as a session. Signs out every other browser.
func (s *Server) handleSetPIN(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil || r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	var req struct{ PIN, Current string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a := s.auth
	if a.pinSet() && !a.checkPIN(req.Current) {
		http.Error(w, "Your current PIN isn't right.", http.StatusUnauthorized)
		return
	}
	if req.PIN != "" && !validPIN(req.PIN) {
		http.Error(w, "Use 4 to 12 digits.", http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	a.file.Sessions = map[string]int64{}
	if req.PIN == "" {
		a.file.PINHash, a.file.Salt = "", ""
	} else {
		a.file.Salt = randomString(16)
		a.file.PINHash = hashPIN(req.PIN, a.file.Salt)
	}
	err := a.save()
	a.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if req.PIN != "" {
		if err := a.newSession(w); err != nil { // stay signed in here
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func validPIN(pin string) bool {
	if len(pin) < 4 || len(pin) > 12 {
		return false
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
