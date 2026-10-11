package gen3

import (
	"encoding/binary"
	"fmt"
)

// ReadMemory reads n bytes of game memory (RAM or ROM) at addr.
type ReadMemory func(addr uint32, n int) ([]byte, error)

// NameTables holds ROM addresses of the name tables. Zero means "use the
// pointer from the ROM header", which romhacks usually keep up to date.
type NameTables struct {
	Species uint32
	Moves   uint32
	Items   uint32
	// Entry sizes, when a game differs from vanilla (11, 13 and 44 bytes):
	// decomp hacks use longer names.
	SpeciesLength, MoveLength, ItemSize int
}

// The Game Freak ROM header at 0x08000100 points at the game's data tables.
const (
	headerSpeciesNames = 0x08000144
	headerMoveNames    = 0x08000148
	headerItems        = 0x080001C8
)

type nameTable struct {
	header uint32 // where the ROM header stores the table's address
	stride int    // bytes per entry
	length int    // bytes of each entry that hold the name
}

var (
	speciesTable = nameTable{headerSpeciesNames, 11, 11}
	moveTable    = nameTable{headerMoveNames, 13, 13}
	itemTable    = nameTable{headerItems, 44, 14} // item structs start with the name
)

// Names looks up species, move and item names from the running game's ROM,
// one entry at a time, caching each name. Not safe for concurrent use.
type Names struct {
	read      ReadMemory
	overrides NameTables
	bases     map[nameTable]uint32
	cache     map[nameTable]map[int]string
	maps      map[int]string
}

func NewNames(read ReadMemory, overrides NameTables) *Names {
	n := &Names{read: read, overrides: overrides}
	n.Reset()
	return n
}

// Reset clears the cache, for when a different game may have been loaded.
func (n *Names) Reset() {
	n.bases = map[nameTable]uint32{}
	n.cache = map[nameTable]map[int]string{}
	n.maps = map[int]string{}
}

// MapName looks up a region map section's name. Names are reached through
// a table of entries (stride bytes each, the name pointer at offset) whose
// first entry is section first.
func (n *Names) MapName(table uint32, stride, offset, first, section int) (string, error) {
	if name, ok := n.maps[section]; ok {
		return name, nil
	}
	if section < first {
		return "", fmt.Errorf("map section %d is before the name table", section)
	}
	ptr, err := n.read(table+uint32((section-first)*stride+offset), 4)
	if err != nil {
		return "", err
	}
	raw, err := n.read(binary.LittleEndian.Uint32(ptr), 24)
	if err != nil {
		return "", err
	}
	name := TitleCase(DecodeText(raw))
	n.maps[section] = name
	return name, nil
}

func (n *Names) Species(id int) (string, error) {
	t := speciesTable
	if l := n.overrides.SpeciesLength; l > 0 {
		t.stride, t.length = l, l
	}
	return n.lookup(t, n.overrides.Species, id)
}

func (n *Names) Move(id int) (string, error) {
	t := moveTable
	if l := n.overrides.MoveLength; l > 0 {
		t.stride, t.length = l, l
	}
	return n.lookup(t, n.overrides.Moves, id)
}

func (n *Names) Item(id int) (string, error) {
	t := itemTable
	if size := n.overrides.ItemSize; size > 0 {
		t.stride, t.length = size, min(size, 20)
	}
	return n.lookup(t, n.overrides.Items, id)
}

func (n *Names) lookup(t nameTable, override uint32, id int) (string, error) {
	if name, ok := n.cache[t][id]; ok {
		return name, nil
	}

	base, ok := n.bases[t]
	if !ok {
		base = override
		if base == 0 {
			ptr, err := n.read(t.header, 4)
			if err != nil {
				return "", fmt.Errorf("reading ROM header: %w", err)
			}
			base = binary.LittleEndian.Uint32(ptr)
		}
		n.bases[t] = base
	}

	name, err := ReadName(n.read, base+uint32(id*t.stride), t.length)
	if err != nil {
		return "", err
	}

	if n.cache[t] == nil {
		n.cache[t] = map[int]string{}
	}
	n.cache[t][id] = name
	return name, nil
}

// ReadName reads a fixed-length name from a table. Some CFRU hacks (Unbound)
// store a pointer in place of names too long for their slot; its last byte
// (0x08 or 0x09) is never a printable character, so that marks it.
func ReadName(read ReadMemory, addr uint32, n int) (string, error) {
	raw, err := read(addr, n)
	if err != nil {
		return "", err
	}
	if n >= 4 && (raw[3] == 0x08 || raw[3] == 0x09) {
		if long, err := read(binary.LittleEndian.Uint32(raw), 32); err == nil {
			raw = long
		}
	}
	return TitleCase(DecodeText(raw)), nil
}
