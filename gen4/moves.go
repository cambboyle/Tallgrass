package gen4

import (
	"bytes"
	"encoding/binary"
	"sync"

	"tallgrass/gen3"
)

// romExtras is ROM data only species pages need, loaded once on first use:
// the TM list and map headers live in the (decompressed) ARM9 binary, egg
// moves in an archive or an overlay.
type romExtras struct {
	once sync.Once
	err  error
	arm9 *ARM9
	tms  []int         // move per TM/HM; HMs after the 92 TMs
	eggs map[int][]int // species -> egg moves
	// Move tutors: their moves, and a bitfield per species (from
	// Bulbasaur) saying which it can learn.
	tutors      []int
	tutorBits   []byte
	tutorStride int
}

const (
	tmCount   = 92
	hmCount   = 8
	eggMarker = 20000 // egg move lists start with 20000 + species
)

func (d *Data) extras() (*romExtras, error) {
	x := d.extra
	x.once.Do(func() {
		rom, err := OpenROM(d.romPath)
		if err != nil {
			x.err = err
			return
		}
		defer rom.Close()
		if x.arm9, x.err = rom.ARM9(); x.err != nil {
			return
		}
		x.tms = findTMs(x.arm9.Data)
		ovs, _ := rom.Overlays()
		x.findTutors(rom, ovs)
		if d.Layout.EggMoves != "" {
			if b, err := rom.ArchiveFile(d.Layout.EggMoves, 0); err == nil {
				x.eggs = parseEggMoves(b, findEggMoves(b))
			}
		} else {
			for _, ov := range ovs {
				if at := findEggMoves(ov); at >= 0 {
					x.eggs = parseEggMoves(ov, at)
					break
				}
			}
		}
	})
	return x, x.err
}

// findTutors finds the move tutor list in an overlay by its first four
// moves (Dive, Mud-Slap, Fury Cutter, Icy Wind). HeartGold/SoulSilver
// entries are 4 bytes (move, BP cost, tutor) with compatibility in
// /fielddata/wazaoshie/waza_oshie.bin; Platinum's are 12 (move, 4 shard
// costs, tutor) with compatibility right after the list. Either way it's
// one bitfield per species from Bulbasaur, rounded up to whole bytes
// (HeartGold/SoulSilver pad to 8).
func (x *romExtras) findTutors(rom *ROM, ovs [][]byte) {
	first := []uint16{291, 189, 210, 196}
	for _, ov := range ovs {
		for _, stride := range []int{4, 12} {
			for o := 0; o+stride*4 <= len(ov); o += 2 {
				match := true
				for i, m := range first {
					if le.Uint16(ov[o+i*stride:]) != m {
						match = false
						break
					}
				}
				if !match {
					continue
				}
				var moves []int
				for n := 0; n < 128 && o+(n+1)*stride <= len(ov); n++ {
					mv := int(le.Uint16(ov[o+n*stride:]))
					if mv == 0 || mv >= 2000 {
						break
					}
					moves = append(moves, mv)
				}
				x.tutors = moves
				if stride == 4 {
					x.tutorStride = 8
					x.tutorBits, _ = rom.File("/fielddata/wazaoshie/waza_oshie.bin")
				} else {
					x.tutorStride = (len(moves) + 7) / 8
					x.tutorBits = ov[o+len(moves)*stride:]
				}
				return
			}
		}
	}
}

// tutorMoves lists the tutor moves a species can learn.
func (d *Data) tutorMoves(species int, x *romExtras) []string {
	out := []string{}
	at := (species - 1) * x.tutorStride
	if species < 1 || x.tutorStride == 0 || at+x.tutorStride > len(x.tutorBits) {
		return out
	}
	bits := x.tutorBits[at : at+x.tutorStride]
	for i, mv := range x.tutors {
		if i/8 < len(bits) && bits[i/8]&(1<<(i%8)) != 0 {
			out = append(out, Name(d.Moves, mv))
		}
	}
	return out
}

// findTMs finds the TM/HM move list by its HMs: HM01 Cut (15), HM02 Fly
// (19), HM03 Surf (57), HM04 Strength (70), after 92 TMs.
func findTMs(arm9 []byte) []int {
	pat := make([]byte, 8)
	for i, m := range []uint16{15, 19, 57, 70} {
		binary.LittleEndian.PutUint16(pat[i*2:], m)
	}
	for at := 0; ; {
		i := bytes.Index(arm9[at:], pat)
		if i < 0 {
			return nil
		}
		i += at
		if start := i - tmCount*2; start >= 0 && i%2 == 0 && start+(tmCount+hmCount)*2 <= len(arm9) {
			tms := make([]int, tmCount+hmCount)
			for k := range tms {
				tms[k] = int(le.Uint16(arm9[start+k*2:]))
			}
			if tms[0] > 0 && tms[0] < 1000 {
				return tms
			}
		}
		at = i + 1
	}
}

// findEggMoves finds where an egg move list starts: Bulbasaur's marker,
// then moves, then another marker soon after.
func findEggMoves(b []byte) int {
	for o := 0; o+4 <= len(b); o += 2 {
		if le.Uint16(b[o:]) != eggMarker+1 {
			continue
		}
		for k := o + 2; k+2 <= len(b) && k < o+64; k += 2 {
			v := int(le.Uint16(b[k:]))
			if v > eggMarker && v < eggMarker+20 && k > o+2 {
				return o
			}
			if v == 0 || v >= 2000 {
				break
			}
		}
	}
	return -1
}

func parseEggMoves(b []byte, at int) map[int][]int {
	if at < 0 {
		return nil
	}
	eggs := map[int][]int{}
	species := 0
	for o := at; o+2 <= len(b); o += 2 {
		v := int(le.Uint16(b[o:]))
		switch {
		case v == 0xFFFF:
			return eggs
		case v > eggMarker:
			species = v - eggMarker
		case v > 0 && species > 0:
			eggs[species] = append(eggs[species], v)
		}
	}
	return eggs
}

// machineMoves lists the TMs and HMs a species can learn: bits 0–99 of its
// base stats bytes 0x1C–0x2B.
func (d *Data) machineMoves(species int, tms []int) []gen3.TMMove {
	out := []gen3.TMMove{}
	p := d.personal[species]
	if len(tms) == 0 || len(p) < 0x2C {
		return out
	}
	for i := range tms {
		if p[0x1C+i/8]&(1<<(i%8)) == 0 || tms[i] == 0 {
			continue
		}
		label := gen3.MachineLabel("TM", i+1)
		if i >= tmCount {
			label = gen3.MachineLabel("HM", i-tmCount+1)
		}
		out = append(out, gen3.TMMove{Machine: label, Move: Name(d.Moves, tms[i])})
	}
	return out
}

// FoundIn lists where each species appears in the wild, from every map
// header's encounter file (HeartGold/SoulSilver format).
func (d *Data) FoundIn() (map[int][]gen3.Found, error) {
	x, err := d.extras()
	if err != nil {
		return nil, err
	}
	if d.Layout.MapHeaders == 0 || d.Layout.Encounters == "" {
		return map[int][]gen3.Found{}, nil
	}
	rom, err := OpenROM(d.romPath)
	if err != nil {
		return nil, err
	}
	defer rom.Close()
	files, err := rom.Archive(d.Layout.Encounters)
	if err != nil {
		return nil, err
	}
	fb := gen3.NewFoundBuilder(len(hgssPeriods))
	for m := 0; ; m++ {
		hb, err := x.arm9.Read(d.Layout.MapHeaders+uint32(m*mapHeaderSize), mapHeaderSize)
		if err != nil {
			break
		}
		h := d.Layout.MapHeader(hb)
		if !d.Layout.realHeader(hb) || h.WildBank >= len(files) || h.Section >= len(d.Locations) {
			break // past the end of the table
		}
		if h.WildBank < 0 {
			continue
		}
		if periods, err := d.Layout.parseEncounters(files[h.WildBank]); err == nil {
			fb.Add(Name(d.Locations, h.Section), periods)
		}
	}
	return fb.Result(), nil
}
