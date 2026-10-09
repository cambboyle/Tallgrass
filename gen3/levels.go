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
		growth: map[int]int{}, rows: map[int][]uint32{}}
}

func (l *Levels) Level(species int, exp uint32) (int, error) {
	g, ok := l.growth[species]
	if !ok {
		b, err := l.read(l.baseStats+uint32(species*baseStatsSize+growthOffset), 1)
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
