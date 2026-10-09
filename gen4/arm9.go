package gen4

import (
	"bytes"
	"errors"
	"fmt"
)

// ARM9 is the game's main code and static data, as loaded into DS RAM.
type ARM9 struct {
	Base uint32 // RAM address of Data[0], usually 0x02000000
	Data []byte
}

// Read returns n bytes at a RAM address inside the ARM9 image.
func (a *ARM9) Read(addr uint32, n int) ([]byte, error) {
	if addr < a.Base || int(addr-a.Base)+n > len(a.Data) {
		return nil, fmt.Errorf("%08X is outside the ARM9 binary", addr)
	}
	return a.Data[addr-a.Base : int(addr-a.Base)+n], nil
}

var nitrocode = []byte{0x21, 0x06, 0xC0, 0xDE, 0xDE, 0xC0, 0x06, 0x21}

// ARM9 reads the main binary and decompresses it if it's compressed (as
// in HeartGold/SoulSilver), so it matches what the game has in RAM.
func (r *ROM) ARM9() (*ARM9, error) {
	h := make([]byte, 0x30)
	if _, err := r.f.ReadAt(h, 0); err != nil {
		return nil, err
	}
	off, base, size := le.Uint32(h[0x20:]), le.Uint32(h[0x28:]), le.Uint32(h[0x2C:])
	b := make([]byte, size)
	if _, err := r.f.ReadAt(b, int64(off)); err != nil {
		return nil, err
	}
	// The SDK's module parameters end with "nitrocode" magic; the word
	// just before the magic is where the compressed part ends in
	// RAM (0 if the binary isn't compressed).
	i := bytes.Index(b, nitrocode)
	if i < 8 {
		return &ARM9{Base: base, Data: b}, nil
	}
	end := le.Uint32(b[i-8:])
	if end == 0 {
		return &ARM9{Base: base, Data: b}, nil
	}
	if end < base || end-base > size {
		return nil, fmt.Errorf("compressed ARM9 end %08X out of range", end)
	}
	data, err := decompressBLZ(b[:end-base])
	if err != nil {
		return nil, fmt.Errorf("decompressing ARM9: %w", err)
	}
	return &ARM9{Base: base, Data: data}, nil
}

// decompressBLZ undoes Nintendo's "bottom LZ" compression, which works
// backwards from the end of the data. The last 8 bytes say how much of the
// end is compressed (and the footer's own length) and how much it grows.
func decompressBLZ(b []byte) ([]byte, error) {
	if len(b) < 8 {
		return nil, errors.New("too short")
	}
	n := len(b)
	bounds, grow := le.Uint32(b[n-8:]), int(le.Uint32(b[n-4:]))
	footer, packed := int(bounds>>24), int(bounds&0xFFFFFF)
	if footer < 8 || packed > n || footer > packed {
		return nil, errors.New("bad footer")
	}
	out := make([]byte, n+grow)
	copy(out, b)
	src, dst, stop := n-footer, n+grow, n-packed
	for src > stop {
		src--
		flags := b[src]
		for bit := 0; bit < 8 && src > stop; bit++ {
			if flags&0x80 == 0 {
				src--
				dst--
				if dst < 0 {
					return nil, errors.New("overflow")
				}
				out[dst] = b[src]
			} else {
				if src < 2 {
					return nil, errors.New("truncated")
				}
				src -= 2
				v := int(b[src+1])<<8 | int(b[src])
				length, disp := v>>12+3, v&0xFFF+3
				for k := 0; k < length; k++ {
					dst--
					if dst < 0 || dst+disp >= len(out) {
						return nil, errors.New("bad back-reference")
					}
					out[dst] = out[dst+disp]
				}
			}
			flags <<= 1
		}
	}
	return out, nil
}

// Overlays reads the ARM9 overlays (code and data loaded on demand),
// decompressed, by overlay number.
func (r *ROM) Overlays() ([][]byte, error) {
	h := make([]byte, 0x58)
	if _, err := r.f.ReadAt(h, 0); err != nil {
		return nil, err
	}
	ovt := make([]byte, le.Uint32(h[0x54:]))
	if _, err := r.f.ReadAt(ovt, int64(le.Uint32(h[0x50:]))); err != nil {
		return nil, err
	}
	var out [][]byte
	for o := 0; o+32 <= len(ovt); o += 32 {
		b, err := r.fileByID(int(le.Uint32(ovt[o+0x18:])))
		if err != nil {
			return nil, err
		}
		if flags := le.Uint32(ovt[o+0x1C:]); flags&(1<<24) != 0 {
			if size := int(flags & 0xFFFFFF); size <= len(b) {
				b = b[:size]
			}
			if b, err = decompressBLZ(b); err != nil {
				return nil, fmt.Errorf("overlay %d: %w", o/32, err)
			}
		}
		out = append(out, b)
	}
	return out, nil
}
