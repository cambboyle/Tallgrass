package gen3

import (
	"encoding/binary"
	"fmt"
)

// Levels works out a Pokémon's level from its experience, which is all a
// boxed Pokémon stores. Each species has a growth rate (byte 19 of its
// 28-byte base stats); the experience table has one row of u32 totals per
// growth rate. Not safe for concurrent use.
type Levels struct {
	read      ReadMemory
	baseStats uint32
	table     uint32
	rowLen    int // vanilla 101; Radical Red 256
	shared    int // if set, the species whose growth rate every species uses
	size, at  int // bytes per species record and where the growth rate is
	growth    map[int]int
	rows      map[int][]uint32
}

const (
	baseStatsSize = 28
	growthOffset  = 19
	maxLevel      = 100
)

func NewLevels(read ReadMemory, baseStats, table uint32, rowLen int) *Levels {
	return &Levels{read: read, baseStats: baseStats, table: table, rowLen: rowLen,
		size: baseStatsSize, at: growthOffset, growth: map[int]int{}, rows: map[int][]uint32{}}
}

// WithRecord sets the species record size and growth rate offset, for games
// whose species data isn't vanilla's 28-byte base stats (ROWE: 64, 0x1B).
func (l *Levels) WithRecord(size, growthAt int) *Levels {
	if size > 0 {
		l.size, l.at = size, growthAt
	}
	return l
}

// SameGrowthAs makes every species use one species' growth rate (ROWE
// gives all Pokémon the same experience curve).
func (l *Levels) SameGrowthAs(species int) *Levels {
	l.shared = species
	return l
}

func (l *Levels) Level(species int, exp uint32) (int, error) {
	if l.shared != 0 {
		species = l.shared
	}
	g, ok := l.growth[species]
	if !ok {
		b, err := l.read(l.baseStats+uint32(species*l.size+l.at), 1)
		if err != nil {
			return 0, err
		}
		g = int(b[0])
		if g > 5 {
			return 0, fmt.Errorf("species %d has growth rate %d", species, g)
		}
		l.growth[species] = g
	}

	row, ok := l.rows[g]
	if !ok {
		b, err := l.read(l.table+uint32(g*l.rowLen*4), (maxLevel+1)*4)
		if err != nil {
			return 0, err
		}
		row = make([]uint32, maxLevel+1)
		for i := range row {
			row[i] = binary.LittleEndian.Uint32(b[i*4:])
		}
		l.rows[g] = row
	}

	level := 1
	for lv := 2; lv <= maxLevel && row[lv] <= exp; lv++ {
		level = lv
	}
	return level, nil
}
