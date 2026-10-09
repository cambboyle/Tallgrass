package gen3

import (
	"fmt"
	"os"
	"testing"
)

// regions is fake memory made of byte slices at base addresses.
type regions map[uint32][]byte

func (r regions) read(addr uint32, n int) ([]byte, error) {
	for base, b := range r {
		if addr >= base && int(addr-base)+n <= len(b) {
			return b[addr-base : int(addr-base)+n], nil
		}
	}
	return nil, fmt.Errorf("unmapped read %08X+%d", addr, n)
}

// romReader reads the ROM file named by TALLGRASS_ROM, or skips the test.
func romReader(t *testing.T) ReadMemory {
	path := os.Getenv("TALLGRASS_ROM")
	if path == "" {
		t.Skip("set TALLGRASS_ROM to a Radical Red ROM to run")
	}
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return regions{0x08000000: rom}.read
}
