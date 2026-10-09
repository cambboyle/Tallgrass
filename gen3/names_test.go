package gen3

import (
	"encoding/binary"
	"testing"
)

// fakeROM is sparse memory: address -> bytes.
type fakeROM map[uint32][]byte

func (r fakeROM) read(addr uint32, n int) ([]byte, error) {
	return r[addr][:n], nil
}

func encode(s string, size int) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = 0xFF
	}
	for i, c := range s {
		switch {
		case c == ' ':
			b[i] = 0x00
		case c >= 'A' && c <= 'Z':
			b[i] = 0xBB + byte(c-'A')
		case c >= 'a' && c <= 'z':
			b[i] = 0xD5 + byte(c-'a')
		}
	}
	return b
}

func ptr(addr uint32) []byte {
	return binary.LittleEndian.AppendUint32(nil, addr)
}

func TestNames(t *testing.T) {
	reads := 0
	rom := fakeROM{
		headerSpeciesNames:  ptr(0x09000000),
		0x09000000 + 155*11: encode("Cyndaquil", 11),
		0x090F3188 + 33*13:  encode("Tackle", 13), // override, header not consulted
		headerItems:         ptr(0x093C0000),
		0x093C0000 + 139*44: encode("Oran Berry", 44),
	}
	names := NewNames(func(addr uint32, n int) ([]byte, error) {
		reads++
		return rom.read(addr, n)
	}, NameTables{Moves: 0x090F3188})

	for _, tc := range []struct {
		lookup func(int) (string, error)
		id     int
		want   string
	}{
		{names.Species, 155, "Cyndaquil"},
		{names.Move, 33, "Tackle"},
		{names.Item, 139, "Oran Berry"},
	} {
		got, err := tc.lookup(tc.id)
		if err != nil || got != tc.want {
			t.Errorf("got %q, %v; want %q", got, err, tc.want)
		}
	}

	before := reads
	names.Species(155)
	if reads != before {
		t.Error("second lookup should be cached")
	}
}
