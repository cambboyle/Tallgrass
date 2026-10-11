package dashboard

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"tallgrass/patch"
)

// PatchResult is what POST /api/library/patch made.
type PatchResult struct {
	File    string `json:"file"`
	Base    string `json:"base"`
	Format  string `json:"format"`
	SHA1    string `json:"sha1"`
	Game    string `json:"game,omitempty"` // a game Tallgrass supports, if the result is one
	Checked bool   `json:"checked"`        // the patch verified its base and result (UPS/BPS)
}

// handleLibraryPatch applies a romhack patch to a ROM in the library:
// POST /api/library/patch?name=<new file name>[&base=<SHA-1 prefix>], the
// patch's bytes as application/octet-stream. UPS and BPS patches name the
// ROM they need, so the base is found by its size and CRC32; IPS and
// xdelta patches need base. The result goes next to the base ROM; nothing is overwritten.
func (s *Server) handleLibraryPatch(w http.ResponseWriter, r *http.Request) {
	if s.saves == nil {
		http.Error(w, "no ROMs folder", http.StatusNotFound)
		return
	}
	if r.Header.Get("Content-Type") != "application/octet-stream" {
		http.Error(w, "expected application/octet-stream", http.StatusUnsupportedMediaType)
		return
	}
	p, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<20))
	if err != nil {
		http.Error(w, "the patch is too big", http.StatusRequestEntityTooLarge)
		return
	}
	info, err := patch.Read(p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	basePath, base, err := s.patchBase(info, r.URL.Query().Get("base"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out, err := patch.Apply(p, base)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(filepath.Base(r.URL.Query().Get("name")))
	if name == "" || name == "." || strings.HasPrefix(name, ".") {
		http.Error(w, "give the patched ROM a name", http.StatusBadRequest)
		return
	}
	if ext := filepath.Ext(basePath); !strings.EqualFold(filepath.Ext(name), ext) {
		name += ext
	}
	dest := filepath.Join(filepath.Dir(basePath), name)
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		http.Error(w, name+" already exists; choose another name", http.StatusConflict)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = f.Write(out)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dest)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sum := sha1.Sum(out)
	res := PatchResult{File: name, Base: filepath.Base(basePath), Format: info.Format, SHA1: hex.EncodeToString(sum[:]), Checked: info.Checked()}
	res.Game = gameFor(res.SHA1)
	if p := s.profileFor(res.SHA1, dest); res.Game == "" && p != nil {
		res.Game = p.Name
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// patchBase finds the ROM a patch applies to: the one named by SHA-1
// prefix, or (UPS/BPS) the one matching the patch's size and CRC32.
func (s *Server) patchBase(info patch.Info, prefix string) (string, []byte, error) {
	for _, rom := range s.saves.ROMs() {
		st, err := os.Stat(rom.Path)
		if err != nil {
			continue
		}
		if prefix != "" {
			if _, sum, err := s.cachedHash(rom.Path); err != nil || sum == "" || !strings.HasPrefix(sum, strings.ToLower(prefix)) {
				continue
			}
		} else if !info.Checked() || st.Size() != int64(info.SourceSize) {
			continue
		}
		b, err := os.ReadFile(rom.Path)
		if err != nil {
			return "", nil, err
		}
		if !info.Matches(b) {
			if prefix != "" {
				return "", nil, patch.ErrWrongBase
			}
			continue
		}
		return rom.Path, b, nil
	}
	switch {
	case prefix != "":
		return "", nil, errors.New("that base ROM isn't in your library")
	case !info.Checked():
		return "", nil, fmt.Errorf("%s patches don't say which ROM they're for: choose the base ROM", info.Format)
	}
	return "", nil, fmt.Errorf("none of your ROMs is the one this patch needs (%s, %d bytes, CRC32 %08X): it's usually the original game, unmodified",
		info.Format, info.SourceSize, info.SourceCRC)
}

// gameFor names a focus game by its ROM's SHA-1.
func gameFor(sum string) string {
	for _, g := range setupGames {
		for _, want := range g.SHA1 {
			if want == sum {
				return g.Name
			}
		}
	}
	return ""
}
