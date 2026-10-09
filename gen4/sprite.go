package gen4

import (
	"errors"
	"fmt"
	"image"
	"image/color"
)

// Pokémon sprites: each species has 6 files in the sprite archive: back
// (female, male), front (female, male), normal palette, shiny palette. The
// images (NCGR) are 160×80, two 80×80 animation frames side by side, 4 bits
// per pixel, stored line by line and encrypted from the first 16-bit word on
// with seed = that word (Platinum and HeartGold/SoulSilver). Palettes (NCLR)
// are 16 BGR555 colours; colour 0 is transparent.
const (
	spriteFiles   = 6
	frontFemale   = 2
	frontMale     = 3
	paletteNormal = 4
	paletteShiny  = 5
	spriteW       = 160
	spriteH       = 80
	FrameSize     = 80
)

// ArchiveFile reads one file from a NARC inside the ROM, without loading
// the whole archive (sprite archives are tens of megabytes).
func (r *ROM) ArchiveFile(path string, index int) ([]byte, error) {
	id, ok := r.paths[path]
	if !ok {
		return nil, fmt.Errorf("%s isn't in the ROM", path)
	}
	fat := make([]byte, 8)
	if _, err := r.f.ReadAt(fat, int64(r.fatOff)+int64(id)*8); err != nil {
		return nil, err
	}
	base := int64(le.Uint32(fat))
	read := func(off int64, n int) ([]byte, error) {
		b := make([]byte, n)
		_, err := r.f.ReadAt(b, base+off)
		return b, err
	}
	head, err := read(0, 0x10)
	if err != nil || string(head[:4]) != "NARC" {
		return nil, errors.New("not a NARC archive")
	}
	o := int64(le.Uint16(head[0x0C:]))
	btaf, err := read(o, 12)
	if err != nil || string(btaf[:4]) != "BTAF" {
		return nil, errors.New("NARC: missing BTAF")
	}
	count := int(le.Uint16(btaf[8:]))
	if index < 0 || index >= count {
		return nil, fmt.Errorf("NARC has %d files, no %d", count, index)
	}
	entry, err := read(o+12+int64(index)*8, 8)
	if err != nil {
		return nil, err
	}
	o += int64(le.Uint32(btaf[4:]))
	btnf, err := read(o, 8)
	if err != nil || string(btnf[:4]) != "BTNF" {
		return nil, errors.New("NARC: missing BTNF")
	}
	o += int64(le.Uint32(btnf[4:]))
	start, end := le.Uint32(entry), le.Uint32(entry[4:])
	if end < start {
		return nil, errors.New("NARC: bad file entry")
	}
	return read(o+8+int64(start), int(end-start))
}

// Sprite returns a Pokémon's front sprite (the first animation frame).
func (d *Data) Sprite(species int, female, shiny bool) (*image.Paletted, error) {
	if d.romPath == "" || d.Layout.Sprites == "" {
		return nil, errors.New("no sprite archive for this game")
	}
	rom, err := OpenROM(d.romPath)
	if err != nil {
		return nil, err
	}
	defer rom.Close()

	base := species * spriteFiles
	first, second := frontMale, frontFemale
	if female {
		first, second = frontFemale, frontMale
	}
	ncgr, err := rom.ArchiveFile(d.Layout.Sprites, base+first)
	if err == nil && len(ncgr) < 0x30 {
		ncgr, err = rom.ArchiveFile(d.Layout.Sprites, base+second) // species without gender differences
	}
	if err != nil {
		return nil, err
	}
	which := paletteNormal
	if shiny {
		which = paletteShiny
	}
	nclr, err := rom.ArchiveFile(d.Layout.Sprites, base+which)
	if err != nil {
		return nil, err
	}
	return decodeSprite(ncgr, nclr)
}

func decodeSprite(ncgr, nclr []byte) (*image.Paletted, error) {
	if len(ncgr) < 0x30+spriteW*spriteH/2 || string(ncgr[:4]) != "RGCN" {
		return nil, errors.New("not a sprite image")
	}
	pal, err := decodePalette(nclr)
	if err != nil {
		return nil, err
	}
	data := make([]byte, spriteW*spriteH/2)
	copy(data, ncgr[0x30:])
	seed := le.Uint16(data)
	for i := 0; i+1 < len(data); i += 2 {
		le.PutUint16(data[i:], le.Uint16(data[i:])^seed)
		seed = seed*0x4E6D + 0x6073
	}
	img := image.NewPaletted(image.Rect(0, 0, FrameSize, FrameSize), pal)
	for y := 0; y < FrameSize; y++ {
		for x := 0; x < FrameSize; x++ {
			b := data[(y*spriteW+x)/2]
			if x%2 == 0 {
				img.SetColorIndex(x, y, b&0xF)
			} else {
				img.SetColorIndex(x, y, b>>4)
			}
		}
	}
	return img, nil
}

func decodePalette(nclr []byte) (color.Palette, error) {
	if len(nclr) < 0x10 || string(nclr[:4]) != "RLCN" {
		return nil, errors.New("not a palette")
	}
	o := int(le.Uint16(nclr[0x0C:])) + 0x18 // TTLP section, then its colours
	if o+32 > len(nclr) {
		return nil, errors.New("palette too short")
	}
	pal := make(color.Palette, 16)
	for i := range pal {
		c := le.Uint16(nclr[o+i*2:])
		scale := func(v uint16) uint8 { v &= 0x1F; return uint8(v<<3 | v>>2) }
		pal[i] = color.NRGBA{scale(c), scale(c >> 5), scale(c >> 10), 255}
	}
	pal[0] = color.NRGBA{}
	return pal, nil
}
