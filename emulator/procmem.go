package emulator

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProcMem reads another process's memory through /proc/<pid>/mem. Linux
// allows it for processes run by the same user (no Yama restriction on
// RetroPie). Used for emulators without a memory API, like DraStic.
type ProcMem struct {
	f *os.File
}

func OpenProcMem(proc string, pid int) (*ProcMem, error) {
	f, err := os.Open(filepath.Join(proc, strconv.Itoa(pid), "mem"))
	if err != nil {
		return nil, err
	}
	return &ProcMem{f: f}, nil
}

func (m *ProcMem) Close() error { return m.f.Close() }

func (m *ProcMem) Read(addr uint32, n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := m.f.ReadAt(b, int64(addr)); err != nil {
		return nil, fmt.Errorf("reading %08X: %w", addr, err)
	}
	return b, nil
}

// Mapped reports whether [lo, hi) lies inside one readable mapping of the
// process, e.g. DraStic's copy of the DS's main RAM at 0x02000000.
func Mapped(proc string, pid int, lo, hi uint64) bool {
	f, err := os.Open(filepath.Join(proc, strconv.Itoa(pid), "maps"))
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "r") {
			continue
		}
		start, end, ok := strings.Cut(fields[0], "-")
		if !ok {
			continue
		}
		a, err1 := strconv.ParseUint(start, 16, 64)
		b, err2 := strconv.ParseUint(end, 16, 64)
		if err1 == nil && err2 == nil && a <= lo && hi <= b {
			return true
		}
	}
	return false
}
