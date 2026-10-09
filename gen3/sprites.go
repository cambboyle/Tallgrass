package gen3

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

// ROM header pointers to the sprite tables. Each table has an 8-byte entry
// per species, starting with a pointer to LZ77-compressed data.
const (
	headerFrontPics      = 0x08000128
	headerNormalPalettes = 0x08000130
	headerShinyPalettes  = 0x08000134
)

const (
	spriteSize      = 64                          // pixels square
	spriteBytes     = spriteSize * spriteSize / 2 // 4 bits per pixel
	paletteBytes    = 16 * 2                      // 16 BGR555 colours
	tilesPerRow     = spriteSize / 8
	spriteTableStep = 8
)

// FrontSprite reads a species' 64x64 front sprite from ROM, using the shiny
// palette if asked. Colour 0 is transparent.
func FrontSprite(read ReadMemory, species int, shiny bool) (*image.Paletted, error) {
	pixels, err := readCompressed(read, headerFrontPics, species, spriteBytes)
	if err != nil {
		return nil, fmt.Errorf("sprite: %w", err)
	}

	palHeader := uint32(headerNormalPalettes)
	if shiny {
		palHeader = headerShinyPalettes
	}
	pal, err := readCompressed(read, palHeader, species, paletteBytes)
	if err != nil {
		return nil, fmt.Errorf("palette: %w", err)
	}

	img := image.NewPaletted(image.Rect(0, 0, spriteSize, spriteSize), decodePalette(pal))

	// Pixels are stored as 8x8 tiles, left to right then top to bottom.
	// Within a tile each byte is two pixels, low nibble first.
	for i, b := range pixels {
		tile, offset := i/32, i%32
		x := (tile%tilesPerRow)*8 + (offset%4)*2
		y := (tile/tilesPerRow)*8 + offset/4
		img.SetColorIndex(x, y, b&0xF)
		img.SetColorIndex(x+1, y, b>>4)
	}
	return img, nil
}

// readCompressed follows a header pointer to a sprite table, then the
// species' entry to its LZ77 data, decompresses it, and returns the first
// size bytes. Some romhack palettes pack several palettes together, so the
// data may be longer than needed.
func readCompressed(read ReadMemory, header uint32, species, size int) ([]byte, error) {
	ptr, err := read(header, 4)
	if err != nil {
		return nil, err
	}
	table := binary.LittleEndian.Uint32(ptr)

	entry, err := read(table+uint32(species*spriteTableStep), 4)
	if err != nil {
		return nil, err
	}
	data := binary.LittleEndian.Uint32(entry)

	lzHeader, err := read(data, 4)
	if err != nil {
		return nil, err
	}
	fullSize := int(binary.LittleEndian.Uint32(lzHeader) >> 8)
	if fullSize < size || fullSize > 64*1024 {
		return nil, fmt.Errorf("LZ77 data at %08X is %d bytes, need %d", data, fullSize, size)
	}

	compressed, err := read(data, lz77MaxCompressed(fullSize))
	if err != nil {
		return nil, err
	}
	out, err := decompressLZ77(compressed)
	if err != nil {
		return nil, err
	}
	return out[:size], nil
}

// decodePalette converts 16 BGR555 colours (5 bits each of red, green, blue).
func decodePalette(b []byte) color.Palette {
	pal := make(color.Palette, 16)
	for i := range pal {
		c := binary.LittleEndian.Uint16(b[i*2:])
		scale := func(v uint16) uint8 { v &= 0x1F; return uint8(v<<3 | v>>2) }
		pal[i] = color.NRGBA{scale(c), scale(c >> 5), scale(c >> 10), 255}
	}
	pal[0] = color.NRGBA{} // transparent background
	return pal
}
