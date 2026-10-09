package gen4

import (
	"fmt"

	"tallgrass/gen3"
)

// Trainer data: one file per trainer in two archives. trdata is 20 bytes:
// party flags (bit 0 custom moves, bit 1 held items), class, unused,
// party size, 4 battle items (u16), AI flags (u32), battle type (u32,
// 2 = double). trpoke has each Pokémon as difficulty (u16), level (u16),
// species (u16; vanilla keeps the form in the top 6 bits), then an item
// and/or 4 moves if the flags say so, then a ball capsule (u16).
var trainerArchives = map[string][2]string{
	"Platinum":             {"/poketool/trainer/trdata.narc", "/poketool/trainer/trpoke.narc"},
	"HeartGold/SoulSilver": {"/a/0/5/5", "/a/0/5/6"},
}

// Trainers reads every trainer battle in the ROM.
func (d *Data) Trainers() ([]gen3.Trainer, error) {
	paths, ok := trainerArchives[d.Layout.Family]
	if !ok {
		return nil, fmt.Errorf("no trainer data for %s", d.Layout.Family)
	}
	rom, err := OpenROM(d.romPath)
	if err != nil {
		return nil, err
	}
	defer rom.Close()
	data, err := rom.Archive(paths[0])
	if err != nil {
		return nil, err
	}
	pokes, err := rom.Archive(paths[1])
	if err != nil {
		return nil, err
	}
	names := d.TrainerNames
	if len(names) != len(data) {
		names = nil // not the list we think it is
	}
	// Hacks with more than 1,023 species can't pack a form into the
	// species field, so they use all 16 bits.
	speciesMask := uint16(0x3FF)
	if len(d.Species) > 0x3FF {
		speciesMask = 0xFFFF
	}

	var out []gen3.Trainer
	for id := 1; id < len(data) && id < len(pokes); id++ {
		td, tp := data[id], pokes[id]
		if len(td) < 20 || td[3] == 0 {
			continue
		}
		flags, count := td[0], int(td[3])
		tr := gen3.Trainer{ID: id, Class: Name(d.TrainerClasses, int(td[1])), Double: le.Uint32(td[16:]) == 2,
			Party: []gen3.TrainerMon{}}
		if names != nil {
			tr.Name = names[id]
		}
		for i := 0; i < 4; i++ {
			if item := int(le.Uint16(td[4+i*2:])); item != 0 {
				tr.Items = append(tr.Items, Name(d.Items, item))
			}
		}
		size := 8
		if flags&2 != 0 {
			size += 2
		}
		if flags&1 != 0 {
			size += 8
		}
		for i := 0; i < count && (i+1)*size <= len(tp); i++ {
			p := tp[i*size:]
			m := gen3.TrainerMon{Level: int(le.Uint16(p[2:])), Species: int(le.Uint16(p[4:]) & speciesMask)}
			m.SpeciesName = d.speciesName(m.Species)
			o := 6
			if flags&2 != 0 {
				if item := int(le.Uint16(p[o:])); item != 0 {
					m.Item = Name(d.Items, item)
				}
				o += 2
			}
			if flags&1 != 0 {
				for k := 0; k < 4; k++ {
					if mv := int(le.Uint16(p[o+k*2:])); mv != 0 {
						m.Moves = append(m.Moves, Name(d.Moves, mv))
					}
				}
			}
			tr.Party = append(tr.Party, m)
		}
		out = append(out, tr)
	}
	return out, nil
}
