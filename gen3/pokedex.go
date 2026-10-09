package gen3

import "encoding/binary"

// National converts internal species IDs to national Pokédex numbers via the
// ROM's u16 table (indexed by species - 1), caching each lookup.
// Not safe for concurrent use.
type National struct {
	read    ReadMemory
	table   uint32
	cache   map[int]int
	reverse map[int]int // national number -> first species with it
}

func NewNational(read ReadMemory, table uint32) *National {
	return &National{read: read, table: table, cache: map[int]int{}}
}

// maxSpecies bounds the reverse lookup; Radical Red has about 1300 species.
const maxSpecies = 2048

// Species returns the first species ID with a national number, so base
// forms win over regional forms. 0 if none.
func (n *National) Species(national int) (int, error) {
	if n.reverse == nil {
		b, err := n.read(n.table, maxSpecies*2)
		if err != nil {
			return 0, err
		}
		n.reverse = map[int]int{}
		for i := maxSpecies - 1; i >= 0; i-- {
			if num := int(binary.LittleEndian.Uint16(b[i*2:])); num != 0 {
				n.reverse[num] = i + 1
			}
		}
	}
	return n.reverse[national], nil
}

func (n *National) Number(species int) (int, error) {
	if species <= 0 {
		return 0, nil
	}
	if num, ok := n.cache[species]; ok {
		return num, nil
	}
	b, err := n.read(n.table+uint32(species-1)*2, 2)
	if err != nil {
		return 0, err
	}
	num := int(binary.LittleEndian.Uint16(b))
	n.cache[species] = num
	return num, nil
}

// HasFlag reports whether a Pokédex flag array has the bit for a national
// number set. Bit 0 of byte 0 is #1.
func HasFlag(flags []byte, national int) bool {
	i := national - 1
	return i >= 0 && i/8 < len(flags) && flags[i/8]&(1<<(i%8)) != 0
}
