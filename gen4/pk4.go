package gen4

import (
	"errors"
	"fmt"
	"strings"

	"tallgrass/gen3"
)

// Gen 4 Pokémon: 136 bytes in boxes, 236 in the party. After an 8-byte
// header (personality, checksum), 128 bytes hold four 32-byte blocks,
// shuffled by personality and encrypted with a PRNG seeded by the checksum.
// The party-only tail (level, HP, stats) is encrypted with one seeded by
// the personality. Offsets below are as in the unshuffled, decrypted data.
const (
	BoxSize   = 136
	PartySize = 236
)

var blockOrders = [24]string{
	"ABCD", "ABDC", "ACBD", "ACDB", "ADBC", "ADCB", "BACD", "BADC", "BCAD", "BCDA", "BDAC", "BDCA",
	"CABD", "CADB", "CBAD", "CBDA", "CDAB", "CDBA", "DABC", "DACB", "DBAC", "DBCA", "DCAB", "DCBA",
}

// crypt XORs 16-bit words with the game's PRNG (x*0x41C64E6D + 0x6073).
func crypt(b []byte, seed uint32) {
	for i := 0; i+1 < len(b); i += 2 {
		seed = seed*0x41C64E6D + 0x6073
		le.PutUint16(b[i:], le.Uint16(b[i:])^uint16(seed>>16))
	}
}

// Decode parses a Gen 4 Pokémon into the shared Pokémon struct; nil for an
// empty slot. Box Pokémon (136 bytes) have no level, HP or stats.
func Decode(raw []byte) (*gen3.Pokemon, error) { return decode(raw, false) }

// DecodeLive is Decode for Pokémon read from a running game's RAM. While the
// game holds a party Pokémon decrypted (in battle, for instance) it edits
// fields in place and only fixes the checksum when it re-encrypts, so plain
// data with a stale checksum is accepted if its party tail is plain too and
// the values are sane.
func DecodeLive(raw []byte) (*gen3.Pokemon, error) { return decode(raw, true) }

func decode(raw []byte, live bool) (*gen3.Pokemon, error) {
	if len(raw) < BoxSize {
		return nil, fmt.Errorf("need %d bytes, got %d", BoxSize, len(raw))
	}
	pid := le.Uint32(raw)
	checksum := le.Uint16(raw[6:])

	// In RAM the game decrypts Pokémon in place for a moment while it works
	// on them (several times a second for the party), without setting any
	// flag; save files are always encrypted. So accept plain data too when
	// it, rather than the decrypted data, matches the checksum.
	data := make([]byte, 128)
	copy(data, raw[8:BoxSize])
	plain := sum16(data) == checksum
	crypt(data, uint32(checksum))
	if sum16(data) != checksum {
		if pid == 0 && checksum == 0 {
			return nil, nil // empty slot
		}
		if !plain && !(live && len(raw) >= PartySize && plainTail(raw[BoxSize:PartySize])) {
			return nil, fmt.Errorf("checksum %04X, want %04X", sum16(data), checksum)
		}
		copy(data, raw[8:BoxSize]) // plain, possibly with a stale checksum
		if !sane(data, pid) {
			return nil, errors.New("plain data with a stale checksum doesn't look like a Pokémon")
		}
	}

	// Unshuffle into canonical A, B, C, D order: b[0x08:] is block A, etc.
	b := make([]byte, BoxSize)
	copy(b, raw[:8])
	order := blockOrders[((pid&0x3E000)>>13)%24]
	for pos, letter := range order {
		copy(b[8+int(letter-'A')*32:], data[pos*32:pos*32+32])
	}

	species := int(le.Uint16(b[0x08:]))
	if species == 0 {
		return nil, nil
	}
	tid, sid := le.Uint16(b[0x0C:]), le.Uint16(b[0x0E:])
	p := &gen3.Pokemon{
		Personality: pid,
		OTID:        uint32(tid) | uint32(sid)<<16,
		Species:     species,
		HeldItem:    int(le.Uint16(b[0x0A:])),
		Experience:  le.Uint32(b[0x10:]),
		Friendship:  int(b[0x14]),
		Ability:     int(b[0x15]),
		Nickname:    DecodeText(b[0x48:0x5E]),
		OTName:      DecodeText(b[0x68:0x78]),
		MetLevel:    int(b[0x84] & 0x7F),
	}
	p.Female = b[0x40]&0x02 != 0
	ev := b[0x18:0x1E]
	p.EVs = gen3.Stats{HP: int(ev[0]), Attack: int(ev[1]), Defense: int(ev[2]), Speed: int(ev[3]), SpAtk: int(ev[4]), SpDef: int(ev[5])}
	for i := 0; i < 4; i++ {
		if id := int(le.Uint16(b[0x28+i*2:])); id != 0 {
			p.Moves = append(p.Moves, gen3.Move{ID: id, PP: int(b[0x30+i])})
		}
	}
	// Platinum and HeartGold/SoulSilver use the newer met location field;
	// Diamond/Pearl only the older one.
	p.MetLocation = int(le.Uint16(b[0x46:]))
	if p.MetLocation == 0 {
		p.MetLocation = int(le.Uint16(b[0x80:]))
	}
	// Same packed IV word as Gen 3: IVs, egg bit, then (here) a nickname bit.
	gen3.SetIVsAndTraits(p, le.Uint32(b[0x38:]))
	p.AbilitySlot = 0

	if len(raw) >= PartySize {
		// The party tail is decrypted and re-encrypted separately from the
		// blocks in live RAM, so try it both ways and keep what's plausible.
		party := make([]byte, PartySize-BoxSize)
		copy(party, raw[BoxSize:PartySize])
		crypt(party, pid)
		if !plausibleParty(party) {
			copy(party, raw[BoxSize:PartySize])
			if !plausibleParty(party) {
				return nil, errors.New("party data is neither encrypted nor plain (caught mid-update?)")
			}
		}
		p.Status = le.Uint32(party)
		p.Condition = gen3.Condition(p.Status)
		p.Level = int(party[4])
		p.CurrentHP = int(le.Uint16(party[6:]))
		p.Stats = gen3.Stats{
			HP: int(le.Uint16(party[8:])), Attack: int(le.Uint16(party[10:])), Defense: int(le.Uint16(party[12:])),
			Speed: int(le.Uint16(party[14:])), SpAtk: int(le.Uint16(party[16:])), SpDef: int(le.Uint16(party[18:])),
		}
	}
	return p, nil
}

// plainTail reports whether a party tail is stored unencrypted.
func plainTail(b []byte) bool { return plausibleParty(b) }

// sane checks shuffled, unencrypted block data for believable values.
func sane(data []byte, pid uint32) bool {
	order := blockOrders[((pid&0x3E000)>>13)%24]
	block := func(letter byte) []byte {
		pos := strings.IndexByte(order, letter)
		return data[pos*32 : pos*32+32]
	}
	a, b := block('A'), block('B')
	species, exp := le.Uint16(a), le.Uint32(a[8:])
	if species == 0 || species > 2047 || exp > 2_000_000 {
		return false
	}
	moves := 0
	for i := 0; i < 4; i++ {
		m := le.Uint16(b[i*2:])
		if m > 1023 {
			return false
		}
		if m != 0 {
			moves++
		}
	}
	if moves == 0 {
		return false
	}
	// The game may have decrypted some blocks but not others yet: the
	// names must be real text too.
	return validText(block('C')[:22]) && validText(block('D')[:16])
}

// plausibleParty checks a decoded party tail: level 1-100, HP within max,
// stats in range.
func plausibleParty(b []byte) bool {
	level, hp, maxHP := int(b[4]), int(le.Uint16(b[6:])), int(le.Uint16(b[8:]))
	if level < 1 || level > 100 || maxHP == 0 || hp > maxHP {
		return false
	}
	for i := 8; i < 20; i += 2 {
		if le.Uint16(b[i:]) > 999 {
			return false
		}
	}
	return true
}

func sum16(b []byte) uint16 {
	var sum uint16
	for i := 0; i+1 < len(b); i += 2 {
		sum += le.Uint16(b[i:])
	}
	return sum
}
