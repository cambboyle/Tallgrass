// Package autoprofile builds a best-effort profile for a FireRed-engine ROM
// that has no hand-made one (CFRU hacks and classic binary hacks), by
// reading the same pointers we'd follow by hand: the ROM header, literal
// pools in FireRed's own code, and a couple of instructions decoded where
// hacks patch them. Generated profiles are marked "auto" and treated as
// unverified.
package autoprofile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"strings"

	"tallgrass/gen3"
	"tallgrass/profile"
)

var le = binary.LittleEndian

// FireRed (US) code locations whose literal pools point at the tables.
const (
	headerSpecies = 0x08000144
	headerMoves   = 0x08000148
	litExpTable   = 0x0802FFB8
	litWild       = 0x08082990
	litMapNames   = 0x080C0C94
	litNational   = 0x080432B0
	// GetSetPokedexFlag: the instructions that load the seen and caught
	// flag addresses, which CFRU-style hacks patch to fixed RAM.
	dexSeenLoad   = 0x08104AFE
	dexCaughtLoad = 0x08104B14
	saveBlock1    = 0x0202552C // where CFRU hacks keep SaveBlock1
	// CFRU's custom wild encounter override variable; its address shows up
	// in literal pools only in CFRU builds.
	cfruWildOverride = 0x0203C758
)

// ForFireRed generates a profile for a FireRed-engine ROM (game code BPRE).
// rom reads ROM addresses; size is the ROM's length.
func ForFireRed(name, sha1 string, rom gen3.ReadMemory, size int) (*profile.Profile, error) {
	ptr := func(addr uint32) (uint32, error) {
		b, err := rom(addr, 4)
		if err != nil {
			return 0, err
		}
		return le.Uint32(b), nil
	}
	inROM := func(a uint32) bool { return a >= 0x08000000 && a < 0x08000000+uint32(size) }

	species, err := ptr(headerSpecies)
	if err != nil || !inROM(species) {
		return nil, errors.New("no species names in the ROM header")
	}
	if n, _ := readName(rom, species+11, 11); !strings.EqualFold(n, "Bulbasaur") {
		return nil, fmt.Errorf("species name table doesn't start with Bulbasaur (%q)", n)
	}

	cfru, err := containsWord(rom, size, cfruWildOverride)
	if err != nil {
		return nil, err
	}

	p := &profile.Profile{
		Name: name, Generation: 3, ROMSHA1: []string{sha1}, Verified: []string{"auto"},
		PartySlots: 6, PokemonSize: 100, EncryptedData: !cfru,
		Trainer: &profile.Trainer{Name: deref(0x0300500C, 0), ID: deref(0x0300500C, 0xA)},
		Location: &profile.Location{MapID: deref(0x03005008, 4), MapSection: profile.Ref{Addr: 0x02036E10},
			MapNames: deref(litMapNames, 0), MapGroups: deref(0x0805524C, 0), FirstMapSection: 0x58},
		WildEncounters: &profile.WildEncounters{Headers: deref(litWild, 0)},
		Experience:     &profile.Experience{BaseStats: deref(0x080001BC, 0), Table: deref(litExpTable, 0)},
		Trainers:       &profile.TrainerData{Table: deref(0x08044028, 0), ClassNames: deref(0x080D80A0, 0), ClassNameLength: 13},
	}
	p.PartyAddress = 0x02024284

	// Move names: the header pointer, unless it's stale (Radical Red's is):
	// then the table with "Pound" at entry 1 that the code points to.
	moves, _ := ptr(headerMoves)
	if n, _ := readName(rom, moves+13, 13); !strings.EqualFold(n, "Pound") {
		found, err := findTable(rom, size, "Pound", 13)
		if err != nil {
			return nil, fmt.Errorf("move names: %w", err)
		}
		p.MoveNames = profile.Addr(found)
	}

	// Experience rows: 101 levels in vanilla, 256 in CFRU hacks. Find where
	// the second row's (Erratic) Lv 100 total of 600,000 sits.
	table, err := ptr(litExpTable)
	if err != nil || !inROM(table) {
		return nil, errors.New("no experience table")
	}
	p.Experience.RowLength = 101
	for _, n := range []int{101, 256} {
		if v, err := ptr(table + uint32((n+100)*4)); err == nil && v == 600000 {
			p.Experience.RowLength = n
		}
	}

	// Pokédex: patched to fixed RAM (CFRU) or vanilla, inside SaveBlock2.
	seen, okSeen := literalLoad(rom, dexSeenLoad)
	caught, okCaught := literalLoad(rom, dexCaughtLoad)
	p.Pokedex = &profile.Pokedex{SpeciesToNational: deref(litNational, 0)}
	sf := &profile.SaveFile{Boxes: 14, TotalBoxes: 14, BoxNames: 0x8344, BoxMonSize: gen3.BoxSize,
		SectorDataSize: 0xF80, RAM: &profile.SaveRAM{SaveBlock1: deref(0x03005008, 0), SaveBlock2: deref(0x0300500C, 0),
			Storage: deref(0x03005010, 0)}}
	if okSeen && okCaught && seen>>24 == 0x02 && caught > seen && caught-seen <= 256 {
		p.Pokedex.Seen, p.Pokedex.Caught = profile.Ref{Addr: seen}, profile.Ref{Addr: caught}
		p.Pokedex.FlagBytes = int(caught - seen)
		sf.DexSeen = profile.BlockOffset{Block: 1, Offset: profile.Addr(seen - saveBlock1)}
		sf.DexCaught = profile.BlockOffset{Block: 1, Offset: profile.Addr(caught - saveBlock1)}
	} else {
		p.Pokedex.Caught, p.Pokedex.Seen = deref(0x0300500C, 0x28), deref(0x0300500C, 0x5C)
		p.Pokedex.FlagBytes = 52
		sf.DexCaught, sf.DexSeen = profile.BlockOffset{Block: 2, Offset: 0x28}, profile.BlockOffset{Block: 2, Offset: 0x5C}
	}
	if cfru {
		// CFRU's expanded PC: packed sectors, 58-byte Pokémon, 25 boxes of
		// which the first 19 sit in the usual place.
		sf.SectorDataSize, sf.BoxMonSize, sf.Boxes, sf.TotalBoxes = 0xFF0, gen3.CompressedSize, 19, 25
	}
	p.SaveFile = sf

	sd, err := speciesData(rom, cfru)
	if err != nil {
		return nil, fmt.Errorf("species data: %w", err)
	}
	p.SpeciesData = sd
	return p, nil
}

// speciesData works out the species tables' formats from what's in them.
func speciesData(rom gen3.ReadMemory, cfru bool) (*profile.SpeciesData, error) {
	ptr := func(addr uint32) uint32 {
		b, err := rom(addr, 4)
		if err != nil {
			return 0
		}
		return le.Uint32(b)
	}
	sd := &profile.SpeciesData{TypeNames: deref(0x080309C8, 0), AbilityNames: deref(0x080001C0, 0),
		Learnsets: deref(0x0803EA7C, 0), Evolutions: deref(0x08042F6C, 0), HiddenAbility: cfru}

	// Type names: FireRed code points at the table from two places. Hacks
	// that add the Fairy type (type 23) may extend only one copy, so
	// prefer whichever has it.
	for _, lit := range []uint32{0x080309C8, 0x080D8510} {
		if n, _ := readName(rom, ptr(lit)+23*7, 7); strings.EqualFold(n, "Fairy") {
			sd.TypeNames = deref(lit, 0)
			break
		}
	}

	// Ability names: 13 bytes in vanilla, 17 in CFRU; ability 1 is Stench.
	abilities := ptr(0x080001C0)
	sd.AbilityNameLength = 13
	if n, _ := readName(rom, abilities+17, 17); strings.EqualFold(n, "Stench") {
		sd.AbilityNameLength = 17
	}

	// Evolutions: Bulbasaur (species 1) evolves at level 16 (method 4)
	// into Ivysaur (2); its entry is 40 or 128 bytes in.
	evos := ptr(0x08042F6C)
	for _, n := range []int{5, 16} {
		if b, err := rom(evos+uint32(n*8), 6); err == nil && le.Uint16(b) == 4 && le.Uint16(b[4:]) == 2 {
			sd.EvolutionsPerSpecies = n
		}
	}
	if sd.EvolutionsPerSpecies == 0 {
		return nil, errors.New("can't find Bulbasaur's evolution")
	}
	if cfru {
		sd.EvolutionScheme = "cfru"
	}

	// Learnsets: Bulbasaur's starts with a level-1 move. CFRU stores
	// (move u16, level u8); vanilla packs level<<9 | move into a u16.
	first, err := rom(ptr(ptr(0x0803EA7C)+4), 3)
	if err != nil {
		return nil, err
	}
	sd.CFRULearnsets = first[2] == 1 && le.Uint16(first)>>9 != 1

	// TM, tutor and egg moves, via FireRed's code literals. The bitfield
	// sizes vary between hacks, so they're measured.
	sd.TMMoves, sd.TMLearnsets = deref(0x08125A8C, 0), deref(0x08043C68, 0)
	sd.TutorMoves, sd.TutorLearnsets = deref(0x08120BE4, 0), deref(0x08120C30, 0)
	sd.EggMoves = deref(0x08045C50, 0)
	sd.TMLearnsetSize = bitfieldSize(rom, ptr(0x08043C68), []int{8, 16, 20, 24, 32})
	sd.TutorLearnsetSize = bitfieldSize(rom, ptr(0x08120C30), []int{2, 4, 8, 12, 16, 20, 24, 32})
	if sd.TMLearnsetSize == 0 {
		sd.TMMoves, sd.TMLearnsets = profile.Ref{}, profile.Ref{}
	}
	if sd.TutorLearnsetSize == 0 {
		sd.TutorMoves, sd.TutorLearnsets = profile.Ref{}, profile.Ref{}
	}
	return sd, nil
}

// bitfieldSize works out how many bytes per species a TM or tutor
// compatibility table uses: with the right size, the empty species 0 and
// Ditto (132, which learns nothing) are all zero, and Mew (151, which learns
// nearly everything) has the most bits set.
func bitfieldSize(rom gen3.ReadMemory, table uint32, sizes []int) int {
	best, bestScore := 0, 0.5 // Mew learns at least half of them
	for _, n := range sizes {
		entry := func(species int) []byte {
			b, err := rom(table+uint32(species*n), n)
			if err != nil {
				return nil
			}
			return b
		}
		zero, ditto, mew := entry(0), entry(132), entry(151)
		if zero == nil || ditto == nil || mew == nil || !allZero(zero) || !allZero(ditto) {
			continue
		}
		set := 0
		for _, c := range mew {
			set += bits.OnesCount8(c)
		}
		if score := float64(set) / float64(n*8); score > bestScore {
			best, bestScore = n, score
		}
	}
	return best
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

func deref(addr, offset uint32) profile.Ref {
	return profile.Ref{Addr: addr, Deref: true, Offset: offset}
}

func readName(rom gen3.ReadMemory, addr uint32, n int) (string, error) {
	b, err := rom(addr, n)
	if err != nil {
		return "", err
	}
	return gen3.DecodeText(b), nil
}

// literalLoad decodes a Thumb "ldr rX, [pc, #imm]" at addr and returns the
// word it loads.
func literalLoad(rom gen3.ReadMemory, addr uint32) (uint32, bool) {
	b, err := rom(addr, 2)
	if err != nil {
		return 0, false
	}
	ins := le.Uint16(b)
	if ins&0xF800 != 0x4800 {
		return 0, false
	}
	lit := (addr+4)&^3 + uint32(ins&0xFF)*4
	v, err := rom(lit, 4)
	if err != nil {
		return 0, false
	}
	return le.Uint32(v), true
}

const chunk = 1 << 20

// containsWord reports whether a 32-bit little-endian word appears
// (aligned) anywhere in the ROM, as it would in a literal pool.
func containsWord(rom gen3.ReadMemory, size int, word uint32) (bool, error) {
	found := false
	err := scan(rom, size, func(addr uint32, b []byte) bool {
		for i := 0; i+4 <= len(b); i += 4 {
			if le.Uint32(b[i:]) == word {
				found = true
				return false
			}
		}
		return true
	})
	return found, err
}

// findTable finds a name table whose entry 1 is name (entries stride bytes
// apart) and which some code points to.
func findTable(rom gen3.ReadMemory, size int, name string, stride int) (uint32, error) {
	want := gen3.EncodeText(name)
	var candidates []uint32
	err := scan(rom, size, func(addr uint32, b []byte) bool {
		for i := 0; i+len(want) <= len(b); i++ {
			if b[i] == want[0] && string(b[i:i+len(want)]) == string(want) {
				candidates = append(candidates, addr+uint32(i)-uint32(stride))
			}
		}
		return true
	})
	if err != nil {
		return 0, err
	}
	for _, c := range candidates {
		if ok, err := containsWord(rom, size, c); err == nil && ok {
			return c, nil
		}
	}
	return 0, fmt.Errorf("no table with %q at entry 1 that code refers to", name)
}

// scan reads the ROM in chunks (overlapping slightly, so a match can't fall
// between two); fn returns false to stop.
func scan(rom gen3.ReadMemory, size int, fn func(addr uint32, b []byte) bool) error {
	for off := 0; off < size; off += chunk {
		n := min(chunk+32, size-off)
		b, err := rom(0x08000000+uint32(off), n)
		if err != nil {
			return err
		}
		if !fn(0x08000000+uint32(off), b) {
			return nil
		}
	}
	return nil
}
