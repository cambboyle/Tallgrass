package dashboard

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestPIN(t *testing.T) {
	s := &Server{}
	if err := s.EnableAuth(filepath.Join(t.TempDir(), "auth.json")); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/party", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("party")) })
	mux.HandleFunc("GET /box.html", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("boxes")) })
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/pin", s.handleSetPIN)
	h := s.protect(mux)

	var cookie *http.Cookie
	do := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "192.168.0.50:1234"
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		for _, c := range w.Result().Cookies() {
			if c.Name == sessionCookie {
				cookie = c
			}
		}
		return w
	}

	if w := do("GET", "/api/party", ""); w.Code != 200 {
		t.Fatalf("no PIN yet: want open, got %d", w.Code)
	}
	if w := do("POST", "/api/auth/pin", `{"pin":"12a4"}`); w.Code != 400 {
		t.Errorf("letters in a PIN: got %d", w.Code)
	}
	if w := do("POST", "/api/auth/pin", `{"pin":"482913"}`); w.Code != 204 {
		t.Fatalf("set PIN: %d %s", w.Code, w.Body)
	}
	// The browser that set it stays signed in.
	if w := do("GET", "/api/party", ""); w.Code != 200 {
		t.Errorf("signed in after setting: %d", w.Code)
	}

	cookie = nil
	if w := do("GET", "/api/party", ""); w.Code != 401 {
		t.Errorf("API without a session: %d", w.Code)
	}
	if w := do("GET", "/box.html", ""); w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/login.html?next=") {
		t.Errorf("page without a session: %d %s", w.Code, w.Header().Get("Location"))
	}
	for i := 0; i < 5; i++ {
		if w := do("POST", "/api/auth/login", `{"pin":"000000"}`); w.Code != 401 {
			t.Fatalf("wrong PIN %d: %d", i, w.Code)
		}
	}
	if w := do("POST", "/api/auth/login", `{"pin":"482913"}`); w.Code != 429 {
		t.Errorf("after 5 wrong PINs: %d", w.Code)
	}
	s.auth.fails = map[string]*failCount{} // skip the 30 s wait
	if w := do("POST", "/api/auth/login", `{"pin":"482913"}`); w.Code != 204 || cookie == nil {
		t.Fatalf("right PIN: %d", w.Code)
	}
	if w := do("GET", "/api/party", ""); w.Code != 200 {
		t.Errorf("after signing in: %d", w.Code)
	}

	if w := do("POST", "/api/auth/pin", `{"pin":"1111","current":"999999"}`); w.Code != 401 {
		t.Errorf("change with the wrong current PIN: %d", w.Code)
	}
	if w := do("POST", "/api/auth/pin", `{"pin":"","current":"482913"}`); w.Code != 204 {
		t.Errorf("remove PIN: %d", w.Code)
	}
	cookie = nil
	if w := do("GET", "/api/party", ""); w.Code != 200 {
		t.Errorf("after removing the PIN: %d", w.Code)
	}
}
