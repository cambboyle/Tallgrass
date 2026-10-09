package gen3

import (
	"fmt"
	"os"
)

const romBase = 0x08000000

// ROMFile reads a ROM straight from disk, as the game would see it mapped at
// 0x08000000, so ROM data (names, sprites, tables) is available without a
// running game.
type ROMFile struct {
	f *os.File
}

func OpenROM(path string) (*ROMFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &ROMFile{f: f}, nil
}

func (r *ROMFile) Close() error { return r.f.Close() }

// Read is a ReadMemory for ROM addresses.
func (r *ROMFile) Read(addr uint32, n int) ([]byte, error) {
	if addr < romBase || addr >= 0x0A000000 {
		return nil, fmt.Errorf("address %08X isn't in ROM", addr)
	}
	b := make([]byte, n)
	got, err := r.f.ReadAt(b, int64(addr-romBase))
	if got == n {
		return b, nil
	}
	return nil, fmt.Errorf("reading ROM at %08X: %v", addr, err)
}
