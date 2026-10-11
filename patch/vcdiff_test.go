package patch

import (
	"bytes"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// vcdFixtures is a base ROM and an edited copy: changed bytes, an insert,
// a delete, a long run, a block moved from elsewhere and growth at the end.
// testdata/*.xdelta were made from these by xdelta3 3.2.2 (see
// TestWriteVCDIFFFixtures).
func vcdFixtures() (base, target []byte) {
	rng := rand.New(rand.NewSource(3284))
	base = make([]byte, 300_000)
	rng.Read(base)
	t := append([]byte{}, base[:50_000]...)
	t = append(t, []byte("TALLGRASS WAS HERE")...)     // insert
	t = append(t, base[50_000:120_000]...)             //
	t = append(t, bytes.Repeat([]byte{0xFF}, 5000)...) // run
	t = append(t, base[130_000:200_000]...)            // 10 KB deleted before this
	t = append(t, base[10_000:30_000]...)              // a block from earlier in the source
	t = append(t, t[1000:9000]...)                     // a block from earlier in the output
	t = append(t, base[200_000:]...)
	for i := 0; i < 200; i++ {
		t[rng.Intn(len(t))] ^= byte(1 + rng.Intn(255))
	}
	extra := make([]byte, 40_000)
	rng.Read(extra)
	return base, append(t, extra...)
}

// TestWriteVCDIFFFixtures writes the fixtures for xdelta3 when
// TALLGRASS_WRITE_FIXTURES is a folder:
//
//	xdelta3 -e -S none -s base.bin target.bin testdata/default.xdelta
//	xdelta3 -e -S none -W 16384 -s base.bin target.bin testdata/windows.xdelta
//	xdelta3 -e -S none -n -A -s base.bin target.bin testdata/bare.xdelta
//	xdelta3 -e -s base.bin target.bin testdata/lzma.xdelta
func TestWriteVCDIFFFixtures(t *testing.T) {
	dir := os.Getenv("TALLGRASS_WRITE_FIXTURES")
	if dir == "" {
		t.Skip("set TALLGRASS_WRITE_FIXTURES to write them")
	}
	base, target := vcdFixtures()
	os.WriteFile(filepath.Join(dir, "base.bin"), base, 0o644)
	os.WriteFile(filepath.Join(dir, "target.bin"), target, 0o644)
}

func TestVCDIFF(t *testing.T) {
	base, target := vcdFixtures()
	for _, name := range []string{"default", "windows", "bare"} {
		p, err := os.ReadFile(filepath.Join("testdata", name+".xdelta"))
		if err != nil {
			t.Fatal(err)
		}
		out, err := Apply(p, base)
		if err != nil || !bytes.Equal(out, target) {
			t.Errorf("%s: %v, %d bytes (want %d), equal %v", name, err, len(out), len(target), bytes.Equal(out, target))
		}
		if info, _ := Read(p); info.Format != "xdelta" || info.Checked() {
			t.Errorf("%s: info = %+v", name, info)
		}
	}

	p, _ := os.ReadFile("testdata/default.xdelta")
	wrong := append([]byte{}, base...)
	wrong[60_000] ^= 1
	if _, err := Apply(p, wrong); !errors.Is(err, ErrWrongBase) {
		t.Errorf("wrong base: %v", err)
	}
	if lz, err := os.ReadFile("testdata/lzma.xdelta"); err == nil {
		if _, err := Apply(lz, base); err == nil || !bytes.Contains([]byte(err.Error()), []byte("secondary compression")) {
			t.Errorf("secondary compression: %v", err)
		}
	}
}
