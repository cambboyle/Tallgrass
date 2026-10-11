package dashboard

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tallgrass/saves"
)

// bpsNumber encodes a UPS/BPS number.
func bpsNumber(n int) []byte {
	var out []byte
	for {
		x := byte(n & 0x7F)
		n >>= 7
		if n == 0 {
			return append(out, x|0x80)
		}
		out = append(out, x)
		n--
	}
}

// simpleBPS is a BPS patch that writes dst whole.
func simpleBPS(src, dst []byte) []byte {
	p := append([]byte("BPS1"), bpsNumber(len(src))...)
	p = append(p, bpsNumber(len(dst))...)
	p = append(p, bpsNumber(0)...)
	p = append(p, bpsNumber((len(dst)-1)<<2|1)...)
	p = append(p, dst...)
	p = binary.LittleEndian.AppendUint32(p, crc32.ChecksumIEEE(src))
	p = binary.LittleEndian.AppendUint32(p, crc32.ChecksumIEEE(dst))
	return binary.LittleEndian.AppendUint32(p, crc32.ChecksumIEEE(p))
}

func TestLibraryPatch(t *testing.T) {
	roms := t.TempDir()
	gba := filepath.Join(roms, "gba")
	os.MkdirAll(gba, 0o755)
	base := []byte("an original game, unmodified....")
	os.WriteFile(filepath.Join(gba, "Original.gba"), base, 0o644)
	os.WriteFile(filepath.Join(gba, "Other.gba"), []byte("another game, same length......."), 0o644)
	watcher := saves.NewWatcher([]saves.Source{{System: "gba", SaveDir: gba, ROMDir: gba}}, saves.NewBackups(t.TempDir()))
	s := New(testProfile, func(uint32, int) ([]byte, error) { return nil, errors.New("no game") })
	s.EnableSaves(watcher, nil)
	h := s.Handler()
	post := func(query string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/library/patch?"+query, strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/octet-stream")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	hack := []byte("a romhack!")
	rec := post("name=My+Hack", simpleBPS(base, hack))
	var res PatchResult
	json.Unmarshal(rec.Body.Bytes(), &res)
	if rec.Code != http.StatusOK || res.File != "My Hack.gba" || res.Base != "Original.gba" || !res.Checked {
		t.Fatalf("patch = %d %s", rec.Code, rec.Body)
	}
	if got, _ := os.ReadFile(filepath.Join(gba, "My Hack.gba")); string(got) != string(hack) {
		t.Errorf("patched ROM = %q", got)
	}
	if rec := post("name=My+Hack", simpleBPS(base, hack)); rec.Code != http.StatusConflict {
		t.Errorf("existing name: %d", rec.Code)
	}
	if rec := post("name=x", simpleBPS([]byte("a ROM nobody has"), hack)); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "none of your ROMs") {
		t.Errorf("unknown base: %d %s", rec.Code, rec.Body)
	}
	if rec := post("name=x", []byte("PATCH\x00\x00\x00\x00\x01AEOF")); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "choose the base ROM") {
		t.Errorf("IPS without a base: %d %s", rec.Code, rec.Body)
	}
	if rec := post("name=../escape", simpleBPS(base, hack)); rec.Code != http.StatusOK {
		t.Errorf("name with a path: %d %s", rec.Code, rec.Body)
	} else if _, err := os.Stat(filepath.Join(gba, "escape.gba")); err != nil {
		t.Errorf("a name with a path should land in the ROM's folder: %v", err)
	}
}
