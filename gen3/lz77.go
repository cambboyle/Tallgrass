package gen3

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// lz77MaxCompressed is the most bytes LZ77 data of a given decompressed
// size can take: a 4-byte header plus one flag byte per 8 literal bytes.
func lz77MaxCompressed(size int) int {
	return 4 + size + (size+7)/8
}

// decompressLZ77 decodes the GBA BIOS LZ77 format (type 0x10) that Gen 3
// uses for sprites and palettes.
//
// After a 4-byte header (0x10, then the 24-bit decompressed size), each flag
// byte describes the next 8 blocks, most significant bit first: 0 means copy
// one literal byte, 1 means a 2-byte back-reference to repeat earlier output.
func decompressLZ77(src []byte) ([]byte, error) {
	if len(src) < 4 || src[0] != 0x10 {
		return nil, errors.New("not LZ77 data")
	}
	size := int(binary.LittleEndian.Uint32(src) >> 8)
	out := make([]byte, 0, size)
	pos := 4

	next := func() (byte, error) {
		if pos >= len(src) {
			return 0, errors.New("LZ77 data ended early")
		}
		pos++
		return src[pos-1], nil
	}

	for len(out) < size {
		flags, err := next()
		if err != nil {
			return nil, err
		}
		for bit := 7; bit >= 0 && len(out) < size; bit-- {
			if flags&(1<<bit) == 0 {
				b, err := next()
				if err != nil {
					return nil, err
				}
				out = append(out, b)
				continue
			}

			hi, err := next()
			if err != nil {
				return nil, err
			}
			lo, err := next()
			if err != nil {
				return nil, err
			}
			length := int(hi>>4) + 3
			dist := (int(hi&0xF)<<8 | int(lo)) + 1
			if dist > len(out) {
				return nil, fmt.Errorf("LZ77 back-reference %d before start of output", dist)
			}
			// Byte by byte, because a reference may overlap what it's writing.
			for i := 0; i < length && len(out) < size; i++ {
				out = append(out, out[len(out)-dist])
			}
		}
	}
	return out, nil
}
