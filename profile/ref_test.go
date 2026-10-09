package profile

import (
	"encoding/binary"
	"testing"
)

func TestParseRef(t *testing.T) {
	for in, want := range map[string]Ref{
		"0x020258E0":       {Addr: 0x020258E0},
		"[0x08082990]":     {Addr: 0x08082990, Deref: true},
		"[0x03005008]+0x4": {Addr: 0x03005008, Deref: true, Offset: 4},
		"[0x03005008]+5":   {Addr: 0x03005008, Deref: true, Offset: 5},
	} {
		got, err := ParseRef(in)
		if err != nil || got != want {
			t.Errorf("%s: got %+v, %v", in, got, err)
		}
	}
	if _, err := ParseRef("[0x1234"); err == nil {
		t.Error("expected error")
	}
}

func TestResolve(t *testing.T) {
	read := func(addr uint32, n int) ([]byte, error) {
		return binary.LittleEndian.AppendUint32(nil, 0x0202552C), nil
	}
	got, _ := Ref{Addr: 0x03005008, Deref: true, Offset: 4}.Resolve(read)
	if got != 0x02025530 {
		t.Errorf("got %08X", got)
	}
}
