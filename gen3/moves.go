package gen3

import (
	"encoding/binary"
	"fmt"
)

// Machine, tutor and egg moves. TM and tutor compatibility are bitfields,
// one per species, whose bit i means "can learn list entry i". Their size
// depends on the game: FireRed has 58 TMs/HMs in 8 bytes and 15 tutors in
// 2; CFRU hacks have 128 TMs/HMs in 16 bytes and 128+ tutors.

// readBits returns the list indexes set in a species' bitfield.
func readBits(read ReadMemory, table uint32, size, species int) ([]int, error) {
	b, err := read(table+uint32(species*size), size)
	if err != nil {
		return nil, err
	}
	var set []int
	for i := 0; i < size*8; i++ {
		if b[i/8]&(1<<(i%8)) != 0 {
			set = append(set, i)
		}
	}
	return set, nil
}

func (t SpeciesTables) readMachineMoves(read ReadMemory, names *Names, species int, info *SpeciesInfo) error {
	if t.TMMoves == 0 || t.TMLearnsets == 0 || t.TMLearnsetSize == 0 {
		return nil
	}
	set, err := readBits(read, t.TMLearnsets, t.TMLearnsetSize, species)
	if err != nil || len(set) == 0 {
		return err
	}
	b, err := read(t.TMMoves, t.TMLearnsetSize*8*2)
	if err != nil {
		return err
	}
	moves := make([]int, len(b)/2)
	for i := range moves {
		moves[i] = int(binary.LittleEndian.Uint16(b[i*2:]))
	}
	tms := tmCount(moves)
	for _, i := range set {
		if moves[i] == 0 {
			continue
		}
		label := MachineLabel("TM", i+1)
		if i >= tms {
			label = MachineLabel("HM", i-tms+1)
		}
		name, _ := names.Move(moves[i])
		info.TMMoves = append(info.TMMoves, TMMove{Machine: label, Move: name})
	}
	return nil
}

// tmCount finds where HMs start: HM01 Cut (15) then HM02 Fly (19).
func tmCount(moves []int) int {
	for i := 0; i+1 < len(moves); i++ {
		if moves[i] == 15 && moves[i+1] == 19 {
			return i
		}
	}
	return len(moves)
}

func MachineLabel(kind string, n int) string {
	return fmt.Sprintf("%s%02d", kind, n)
}

func (t SpeciesTables) readTutorMoves(read ReadMemory, names *Names, species int, info *SpeciesInfo) error {
	if t.TutorMoves == 0 || t.TutorLearnsets == 0 || t.TutorLearnsetSize == 0 {
		return nil
	}
	set, err := readBits(read, t.TutorLearnsets, t.TutorLearnsetSize, species)
	if err != nil || len(set) == 0 {
		return err
	}
	b, err := read(t.TutorMoves, t.TutorLearnsetSize*8*2)
	if err != nil {
		return err
	}
	for _, i := range set {
		if move := int(binary.LittleEndian.Uint16(b[i*2:])); move != 0 {
			name, _ := names.Move(move)
			info.TutorMoves = append(info.TutorMoves, name)
		}
	}
	return nil
}

// eggMoveMarker starts a species' entry in the egg move list.
const eggMoveMarker = 20000

func (t SpeciesTables) readEggMoves(read ReadMemory, names *Names, species int, info *SpeciesInfo) error {
	if t.EggMoves == 0 {
		return nil
	}
	const chunk = 0x1000
	found := false
	for addr := t.EggMoves; addr < t.EggMoves+0x10000; addr += chunk {
		b, err := read(addr, chunk)
		if err != nil {
			return err
		}
		for o := 0; o+2 <= len(b); o += 2 {
			v := int(binary.LittleEndian.Uint16(b[o:]))
			switch {
			case v == 0xFFFF:
				return nil
			case v > eggMoveMarker:
				if found {
					return nil // the next species' entry
				}
				found = v-eggMoveMarker == species
			case found:
				name, _ := names.Move(v)
				info.EggMoves = append(info.EggMoves, name)
			}
		}
	}
	return nil
}
