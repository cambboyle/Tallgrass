// Package gen3 decodes the 100-byte Gen 3 Pokémon struct. The layout is the
// same in RAM and in save files, so this serves both.
package gen3

import (
	"encoding/binary"
	"fmt"
)

// EggSpecies is the species slot holding the egg sprite, in vanilla and CFRU.
const EggSpecies = 412

const (
	PartySize    = 100
	BoxSize      = 80 // boxed Pokémon lack the party-only status, level and stats
	dataStart    = 32
	dataSize     = 48
	subBlockSize = 12
)

type Stats struct {
	HP      int `json:"hp"`
	Attack  int `json:"attack"`
	Defense int `json:"defense"`
	Speed   int `json:"speed"`
	SpAtk   int `json:"spAtk"`
	SpDef   int `json:"spDef"`
}

type Move struct {
	ID int `json:"id"`
	PP int `json:"pp"`
}

type Pokemon struct {
	Personality uint32 `json:"personality"`
	OTID        uint32 `json:"otId"`
	Nickname    string `json:"nickname"`
	OTName      string `json:"otName"`

	Species    int    `json:"species"`
	HeldItem   int    `json:"heldItem"`
	Experience uint32 `json:"experience"`
	Friendship int    `json:"friendship"`
	Moves      []Move `json:"moves"` // only non-empty moves

	MetLocation int `json:"metLocation"` // region map section
	MetLevel    int `json:"metLevel"`    // 0 if hatched

	EVs Stats `json:"evs"`
	IVs Stats `json:"ivs"`

	IsEgg       bool   `json:"isEgg"`
	IsShiny     bool   `json:"isShiny"`
	Female      bool   `json:"female,omitempty"` // stored in Gen 4+; Gen 3 derives it from species data
	AbilitySlot int    `json:"abilitySlot"`
	Ability     int    `json:"ability,omitempty"` // ability ID; Gen 4+ only (Gen 3 stores a slot)
	Nature      string `json:"nature"`
	NatureUp    string `json:"natureUp"`   // JSON stat key the nature raises, "" if neutral
	NatureDown  string `json:"natureDown"` // JSON stat key the nature lowers, "" if neutral

	Status    uint32 `json:"-"`
	Condition string `json:"condition"` // "SLP", "PSN", "BRN", "FRZ", "PAR", "TOX" or ""
	Level     int    `json:"level"`
	CurrentHP int    `json:"currentHp"`
	Stats     Stats  `json:"stats"` // Stats.HP is max HP
}

// Decode parses one Pokémon: 100 bytes for the party, or 80 for a PC box
// (then Level, HP and Stats stay zero; see Levels). It returns (nil, nil) for
// an empty slot. Set encrypted for vanilla games; CFRU hacks store the data plain.
func Decode(b []byte, encrypted bool) (*Pokemon, error) {
	if len(b) < BoxSize {
		return nil, fmt.Errorf("need at least %d bytes, got %d", BoxSize, len(b))
	}

	le := binary.LittleEndian
	p := &Pokemon{
		Personality: le.Uint32(b[0:4]),
		OTID:        le.Uint32(b[4:8]),
		Nickname:    DecodeText(b[8:18]),
		OTName:      DecodeText(b[20:27]),
	}

	growth, attacks, evs, misc := subBlocks(b, p.Personality, p.OTID, encrypted)

	p.Species = int(le.Uint16(growth[0:2]))
	if p.Species == 0 {
		return nil, nil
	}
	p.HeldItem = int(le.Uint16(growth[2:4]))
	p.Experience = le.Uint32(growth[4:8])
	p.Friendship = int(growth[9])

	for i := 0; i < 4; i++ {
		id := int(le.Uint16(attacks[i*2 : i*2+2]))
		if id != 0 {
			p.Moves = append(p.Moves, Move{ID: id, PP: int(attacks[8+i])})
		}
	}

	p.EVs = Stats{
		HP: int(evs[0]), Attack: int(evs[1]), Defense: int(evs[2]),
		Speed: int(evs[3]), SpAtk: int(evs[4]), SpDef: int(evs[5]),
	}

	p.MetLocation = int(misc[1])
	p.MetLevel = int(le.Uint16(misc[2:4]) & 0x7F)

	p.setIVsAndPersonalityTraits(le.Uint32(misc[4:8]))

	if len(b) < PartySize {
		return p, nil
	}

	p.Status = le.Uint32(b[80:84])
	p.Condition = condition(p.Status)
	p.Level = int(b[84])
	p.CurrentHP = int(le.Uint16(b[86:88]))
	p.Stats = Stats{
		HP:      int(le.Uint16(b[88:90])),
		Attack:  int(le.Uint16(b[90:92])),
		Defense: int(le.Uint16(b[92:94])),
		Speed:   int(le.Uint16(b[94:96])),
		SpAtk:   int(le.Uint16(b[96:98])),
		SpDef:   int(le.Uint16(b[98:100])),
	}

	return p, nil
}

// subBlockOrders[personality % 24] gives the stored position of each sub-block
// as letters: G=growth, A=attacks, E=EVs, M=misc.
var subBlockOrders = [24]string{
	"GAEM", "GAME", "GEAM", "GEMA", "GMAE", "GMEA",
	"AGEM", "AGME", "AEGM", "AEMG", "AMGE", "AMEG",
	"EGAM", "EGMA", "EAGM", "EAMG", "EMGA", "EMAG",
	"MGAE", "MGEA", "MAGE", "MAEG", "MEGA", "MEAG",
}

// subBlocks returns the four 12-byte sub-blocks in canonical order,
// decrypting and unshuffling first if needed.
func subBlocks(b []byte, personality, otID uint32, encrypted bool) (growth, attacks, evs, misc []byte) {
	data := make([]byte, dataSize)
	copy(data, b[dataStart:dataStart+dataSize])

	if !encrypted {
		return data[0:12], data[12:24], data[24:36], data[36:48]
	}

	key := personality ^ otID
	for i := 0; i < dataSize; i += 4 {
		v := binary.LittleEndian.Uint32(data[i:]) ^ key
		binary.LittleEndian.PutUint32(data[i:], v)
	}

	blocks := map[byte][]byte{}
	for pos, letter := range []byte(subBlockOrders[personality%24]) {
		blocks[letter] = data[pos*subBlockSize : (pos+1)*subBlockSize]
	}
	return blocks['G'], blocks['A'], blocks['E'], blocks['M']
}

var Natures = [25]string{
	"Hardy", "Lonely", "Brave", "Adamant", "Naughty",
	"Bold", "Docile", "Relaxed", "Impish", "Lax",
	"Timid", "Hasty", "Serious", "Jolly", "Naive",
	"Modest", "Mild", "Quiet", "Bashful", "Rash",
	"Calm", "Gentle", "Sassy", "Careful", "Quirky",
}

var natureStats = [5]string{"attack", "defense", "speed", "spAtk", "spDef"}

// Condition decodes a status word (the same bits in Gen 3 and 4).
func Condition(status uint32) string { return condition(status) }

// condition decodes the status word: bits 0-2 are sleep turns, then one bit
// each for poison, burn, freeze, paralysis and bad poison.
func condition(status uint32) string {
	switch {
	case status&0x7 != 0:
		return "SLP"
	case status&(1<<3) != 0:
		return "PSN"
	case status&(1<<4) != 0:
		return "BRN"
	case status&(1<<5) != 0:
		return "FRZ"
	case status&(1<<6) != 0:
		return "PAR"
	case status&(1<<7) != 0:
		return "TOX"
	}
	return ""
}

// SetIVsAndTraits is setIVsAndPersonalityTraits for other generations'
// decoders, which share the IV word layout and personality-based traits.
func SetIVsAndTraits(p *Pokemon, ivWord uint32) { p.setIVsAndPersonalityTraits(ivWord) }

// setIVsAndPersonalityTraits fills in what comes from the packed IV word
// (IVs, egg flag, ability slot) and from personality and OT ID (shininess,
// nature).
func (p *Pokemon) setIVsAndPersonalityTraits(ivWord uint32) {
	// IVs are packed 5 bits each, followed by the egg flag and ability bit.
	iv := func(i int) int { return int(ivWord>>(5*i)) & 0x1F }
	p.IVs = Stats{HP: iv(0), Attack: iv(1), Defense: iv(2), Speed: iv(3), SpAtk: iv(4), SpDef: iv(5)}
	p.IsEgg = ivWord&(1<<30) != 0
	p.AbilitySlot = int(ivWord >> 31)

	// Shiny when the four 16-bit halves of personality and OT ID XOR to under 8.
	p.IsShiny = (p.Personality>>16 ^ p.Personality&0xFFFF ^ p.OTID>>16 ^ p.OTID&0xFFFF) < 8

	nature := int(p.Personality % 25)
	p.Nature = Natures[nature]
	// Natures form a 5x5 grid: row = raised stat, column = lowered stat.
	if up, down := nature/5, nature%5; up != down {
		p.NatureUp, p.NatureDown = natureStats[up], natureStats[down]
	}
}
