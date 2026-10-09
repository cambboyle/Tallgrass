package profile

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
)

// Ref is a memory location that may sit behind a pointer. In JSON it's
// either a plain address ("0x020258E0") or a pointer to follow plus an
// offset ("[0x03005008]+0x4"), for structures that move around in RAM or
// tables whose ROM address is stored in the game's code.
type Ref struct {
	Addr   uint32
	Deref  bool // read a pointer at Addr, then add Offset
	Offset uint32
}

var refPattern = regexp.MustCompile(`^\[(0x[0-9a-fA-F]+)\](?:\+(0x[0-9a-fA-F]+|[0-9]+))?$`)

func (r *Ref) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("address must be a string like \"0x020258E0\" or \"[0x03005008]+0x4\"")
	}
	ref, err := ParseRef(s)
	if err != nil {
		return err
	}
	*r = ref
	return nil
}

func ParseRef(s string) (Ref, error) {
	if m := refPattern.FindStringSubmatch(s); m != nil {
		addr, _ := strconv.ParseUint(m[1], 0, 32)
		var off uint64
		if m[2] != "" {
			off, _ = strconv.ParseUint(m[2], 0, 32)
		}
		return Ref{Addr: uint32(addr), Deref: true, Offset: uint32(off)}, nil
	}
	addr, err := strconv.ParseUint(s, 0, 32)
	if err != nil {
		return Ref{}, fmt.Errorf("bad address %q", s)
	}
	return Ref{Addr: uint32(addr)}, nil
}

func (r Ref) IsZero() bool { return r == Ref{} }

// Resolve returns the final address, reading the pointer if there is one.
func (r Ref) Resolve(read func(addr uint32, n int) ([]byte, error)) (uint32, error) {
	if !r.Deref {
		return r.Addr, nil
	}
	b, err := read(r.Addr, 4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b) + r.Offset, nil
}
