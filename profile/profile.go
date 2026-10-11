// Package profile loads per-game settings (memory addresses, struct quirks)
// so new romhacks are a config change rather than a code change.
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"tallgrass/profiles"
)

type Profile struct {
	Name       string `json:"name"`
	Generation int    `json:"generation"`

	// SHA-1s of the ROM files this profile is for, to match save files to
	// a profile without a running game.
	ROMSHA1 []string `json:"romSha1"`

	// What's been checked against the real game: "rom" (tables decode),
	// "save" (a real save parses), "live" (RAM addresses while playing).
	Verified []string `json:"verified"`

	PartyAddress Addr `json:"partyAddress"`

	PartySlots  int `json:"partySlots"`
	PokemonSize int `json:"pokemonSize"`

	// Vanilla Gen 3 XOR-encrypts and shuffles the 48-byte data block.
	// CFRU hacks like Radical Red store it plain, in growth/attacks/EVs/misc order.
	EncryptedData bool `json:"encryptedData"`

	// Name tables in ROM. Leave unset to use the ROM header's pointers;
	// set one when a romhack moved a table without updating the header.
	SpeciesNames Addr `json:"speciesNames"`
	MoveNames    Addr `json:"moveNames"`
	ItemNames    Addr `json:"itemNames"`
	// Name entry sizes when they differ from vanilla (11, 13, 44).
	SpeciesNameLength int `json:"speciesNameLength"`
	MoveNameLength    int `json:"moveNameLength"`
	ItemSize          int `json:"itemSize"`

	// PokemonFormat is "" for the vanilla/CFRU layout, or "rowe" (see
	// gen3.DecodeROWE). FormSpecies, if set, maps species + form to the
	// form's own species ID (gen3.ResolveForm).
	PokemonFormat string `json:"pokemonFormat"`
	FormSpecies   Addr   `json:"formSpecies"`

	// Optional; leave out to skip that dashboard section.
	Trainer        *Trainer        `json:"trainer"`
	Location       *Location       `json:"location"`
	WildEncounters *WildEncounters `json:"wildEncounters"`
	Pokedex        *Pokedex        `json:"pokedex"`
	SaveFile       *SaveFile       `json:"saveFile"`
	Experience     *Experience     `json:"experience"`
	SpeciesData    *SpeciesData    `json:"speciesData"`
	Trainers       *TrainerData    `json:"trainers"`
}

// TrainerData locates trainer battles (gTrainers, 40 bytes each) and
// trainer class names, for the docs and the Route section.
type TrainerData struct {
	Table           Ref `json:"table"`
	ClassNames      Ref `json:"classNames"`
	ClassNameLength int `json:"classNameLength"` // 13
}

// SpeciesData locates what a species page shows (base stats come from
// experience.baseStats).
type SpeciesData struct {
	TypeNames            Ref  `json:"typeNames"` // 7 bytes per name
	AbilityNames         Ref  `json:"abilityNames"`
	AbilityNameLength    int  `json:"abilityNameLength"` // 13 vanilla, 17 CFRU
	HiddenAbility        bool `json:"hiddenAbility"`     // CFRU: base stats byte 26
	Learnsets            Ref  `json:"learnsets"`
	CFRULearnsets        bool `json:"cfruLearnsets"` // 3-byte entries instead of packed u16s
	Evolutions           Ref  `json:"evolutions"`
	EvolutionsPerSpecies int  `json:"evolutionsPerSpecies"` // 5 vanilla, 16 CFRU
	// Machine, tutor and egg moves; each is optional (Addr 0 = not set).
	TMMoves           Ref `json:"tmMoves"`           // u16 move per TM, then HMs
	TMLearnsets       Ref `json:"tmLearnsets"`       // bitfield per species
	TMLearnsetSize    int `json:"tmLearnsetSize"`    // bytes per species: 8 vanilla, 16 CFRU
	TutorMoves        Ref `json:"tutorMoves"`        // u16 move per tutor
	TutorLearnsets    Ref `json:"tutorLearnsets"`    // bitfield per species
	TutorLearnsetSize int `json:"tutorLearnsetSize"` // bytes per species: 2 FireRed, 4 Emerald, 16+ CFRU
	EggMoves          Ref `json:"eggMoves"`          // u16 list: 20000+species, then its moves; 0xFFFF ends
	// EvolutionScheme names extra evolution methods: "" (vanilla), "cfru",
	// or a hack that numbers its later methods differently.
	EvolutionScheme string `json:"evolutionScheme"`
	// SpeciesCount bounds whole-ROM scans; 0 means "count the names", which
	// works for hacks but not vanilla, where move names follow species names.
	SpeciesCount int `json:"speciesCount"`
}

// SaveFile describes the .srm/.sav layout, for the box viewer.
type SaveFile struct {
	Signature       Addr `json:"signature"`      // sector footer signature; vanilla 0x08012025 if unset
	PartyOffset     Addr `json:"partyOffset"`    // party in SaveBlock1: FireRed 0x38 (default), Emerald 0x238
	SectorDataSize  Addr `json:"sectorDataSize"` // vanilla 0xF80, CFRU 0xFF0
	VerifyChecksums bool `json:"verifyChecksums"`
	Block2Size      Addr `json:"block2Size"` // only needed to verify checksums
	Block1Size      Addr `json:"block1Size"`
	StorageSize     Addr `json:"storageSize"`

	BoxMonSize int  `json:"boxMonSize"` // 80 (vanilla) or 58 (CFRU compressed)
	Boxes      int  `json:"boxes"`      // boxes we can read, stored back to back
	TotalBoxes int  `json:"totalBoxes"` // boxes the game has
	BoxNames   Addr `json:"boxNames"`   // offset of box 1's name in PC storage
	// BoxNamesInOrder: every box's name follows the last (ROWE), instead of
	// CFRU's boxes 15+ stored before box 1's name in reverse.
	BoxNamesInOrder bool `json:"boxNamesInOrder"`
	// Boxes past Boxes that a hack keeps elsewhere, and the extra sectors
	// (outside both save slots) some of them use.
	ExtraBoxes   []BoxGroup `json:"extraBoxes"`
	ExtraSectors []int      `json:"extraSectors"`

	DexSeen   BlockOffset `json:"dexSeen"` // flag arrays; size from pokedex.flagBytes
	DexCaught BlockOffset `json:"dexCaught"`

	// Where the same blocks live in RAM while the game runs, for live boxes.
	RAM *SaveRAM `json:"ram"`
}

type SaveRAM struct {
	SaveBlock1 Ref `json:"saveBlock1"`
	SaveBlock2 Ref `json:"saveBlock2"`
	Storage    Ref `json:"storage"`
	Extra      Ref `json:"extra"` // the RAM that ExtraSectors are saved from
}

// BoxGroup is a run of boxes (numbered from 1) stored back to back from
// Offset in a block: "block1", "block2" or "extra".
type BoxGroup struct {
	First  int    `json:"first"`
	Last   int    `json:"last"`
	Block  string `json:"block"`
	Offset Addr   `json:"offset"`
}

// Place says where box n (from 1) lives, if it's in an extra group.
func (sf *SaveFile) Place(n int) (block string, offset int, ok bool) {
	for _, g := range sf.ExtraBoxes {
		if n >= g.First && n <= g.Last {
			return g.Block, int(g.Offset) + (n-g.First)*30*sf.BoxMonSize, true
		}
	}
	return "", 0, false
}

// BlockOffset is a position in SaveBlock1 or SaveBlock2.
type BlockOffset struct {
	Block  int  `json:"block"`
	Offset Addr `json:"offset"`
}

// Experience locates what's needed to work out a boxed Pokémon's level.
type Experience struct {
	BaseStats Ref `json:"baseStats"` // 28 bytes per species, growth rate at 19
	Table     Ref `json:"table"`     // u32 totals, one row per growth rate
	RowLength int `json:"rowLength"` // levels per row: vanilla 101
	// Non-vanilla species data: record size and growth rate offset, and a
	// species whose growth rate every species shares (ROWE: 64, 0x1B, 10).
	BaseStatsSize       int `json:"baseStatsSize"`
	GrowthOffset        int `json:"growthOffset"`
	SharedGrowthSpecies int `json:"sharedGrowthSpecies"`
}

type Trainer struct {
	Name Ref `json:"name"` // 8 bytes of Gen 3 text
	ID   Ref `json:"id"`   // u32, same as party Pokémon's OT ID
}

type Location struct {
	MapID           Ref `json:"mapId"`           // map group byte, then map number
	MapSection      Ref `json:"mapSection"`      // region map section of the current map
	MapNames        Ref `json:"mapNames"`        // table of entries holding pointers to map names
	MapGroups       Ref `json:"mapGroups"`       // gMapGroups: map headers by group and number
	FirstMapSection int `json:"firstMapSection"` // section of mapNames[0]
	// Entry size and where the name pointer sits in it: FireRed has bare
	// pointers (4, 0); Emerald has region map entries (x, y, w, h, name: 8, 4).
	MapNameStride int `json:"mapNameStride"`
	MapNameOffset int `json:"mapNameOffset"`
}

// MapNameTable returns the entry size and name pointer offset, with
// FireRed's as the default.
func (l *Location) MapNameTable() (stride, offset int) {
	if l.MapNameStride == 0 {
		return 4, 0
	}
	return l.MapNameStride, l.MapNameOffset
}

type WildEncounters struct {
	Headers   Ref      `json:"headers"`   // main wild header table
	ClockHour Ref      `json:"clockHour"` // RTC hour byte, needed for periods
	Periods   []Period `json:"periods"`   // time-of-day tables, if the game has them
	// HeaderSize and Slots describe non-vanilla headers: bytes per header
	// (20 if unset) and which pointer is land, water, rock smash, fishing.
	HeaderSize int   `json:"headerSize"`
	Slots      []int `json:"slots"`
}

type Period struct {
	Name     string `json:"name"`
	Headers  Ref    `json:"headers"` // leave out to use only the main table
	FromHour int    `json:"fromHour"`
	ToHour   int    `json:"toHour"` // exclusive; may wrap past midnight
	// LandSlot picks this period's land pointer within the main header,
	// for games keeping day and night in one header.
	LandSlot *int `json:"landSlot"`
}

type Pokedex struct {
	Caught            Ref `json:"caught"` // bit flags by national dex number - 1
	Seen              Ref `json:"seen"`
	FlagBytes         int `json:"flagBytes"`
	SpeciesToNational Ref `json:"speciesToNational"` // u16 table indexed by species - 1
}

// Addr is a memory address written in JSON as a hex string, e.g. "0x02024284".
type Addr uint32

func (a *Addr) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("address must be a hex string like \"0x02024284\"")
	}
	v, err := strconv.ParseUint(s, 0, 32)
	if err != nil {
		return fmt.Errorf("bad address %q: %w", s, err)
	}
	*a = Addr(v)
	return nil
}

// All loads every bundled profile.
func All() ([]*Profile, error) {
	entries, err := profiles.FS.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var all []*Profile
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p, err := Load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		all = append(all, p)
	}
	return all, nil
}

// Load reads a bundled profile by name ("radical-red"), or a profile file
// from disk when given a path ending in .json.
func Load(path string) (*Profile, error) {
	var raw []byte
	var err error
	if strings.HasSuffix(path, ".json") {
		raw, err = os.ReadFile(path)
	} else {
		raw, err = profiles.FS.ReadFile(path + ".json")
	}
	if err != nil {
		return nil, fmt.Errorf("loading profile %q: %w", path, err)
	}

	var p Profile
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	if p.PartyAddress == 0 || p.PartySlots == 0 || p.PokemonSize == 0 {
		return nil, fmt.Errorf("%s: partyAddress, partySlots and pokemonSize are required", path)
	}

	return &p, nil
}
