// Package patch applies romhack patches (IPS, UPS, BPS and xdelta) to a ROM the
// user already has. Hacks are shared as patches precisely so that nobody
// passes ROMs around; this is the same job Lunar IPS or Floating IPS does.
//
// UPS and BPS record the size and CRC32 of the ROM they expect and of the
// result, so a wrong base ROM is caught before and after patching. xdelta
// checks each block of output as it goes (so a wrong base fails while
// patching); IPS checks nothing.
package patch

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

// Info is what a patch says about itself.
type Info struct {
	Format     string // "IPS", "UPS", "BPS" or "xdelta"
	SourceSize int    // -1 if the format doesn't say (IPS, xdelta)
	SourceCRC  uint32
	TargetSize int
	TargetCRC  uint32
}

// Checked reports whether the patch names its base ROM (UPS, BPS).
func (i Info) Checked() bool { return i.SourceSize >= 0 }

// Matches reports whether rom is the base ROM this patch expects. Always
// true for IPS, which can't tell.
func (i Info) Matches(rom []byte) bool {
	return !i.Checked() || len(rom) == i.SourceSize && crc32.ChecksumIEEE(rom) == i.SourceCRC
}

var ErrWrongBase = errors.New("this patch is for a different ROM")

// Read identifies a patch and checks its own checksum.
func Read(p []byte) (Info, error) {
	switch {
	case bytes.HasPrefix(p, []byte("PATCH")):
		return Info{Format: "IPS", SourceSize: -1, TargetSize: -1}, nil
	case bytes.HasPrefix(p, vcdiffMagic) && len(p) > 4:
		if p[3] != 0 {
			return Info{}, fmt.Errorf("unsupported xdelta (VCDIFF) version %d", p[3])
		}
		return Info{Format: "xdelta", SourceSize: -1, TargetSize: -1}, nil
	case bytes.HasPrefix(p, []byte("UPS1")), bytes.HasPrefix(p, []byte("BPS1")):
		if len(p) < 4+12 {
			return Info{}, errors.New("patch is truncated")
		}
		if crc32.ChecksumIEEE(p[:len(p)-4]) != binary.LittleEndian.Uint32(p[len(p)-4:]) {
			return Info{}, errors.New("the patch file is damaged (its checksum doesn't match)")
		}
		r := &reader{b: p, pos: 4}
		src, dst := r.number(), r.number()
		if r.err != nil {
			return Info{}, r.err
		}
		f := p[len(p)-12:]
		return Info{Format: string(p[:3]), SourceSize: src, TargetSize: dst,
			SourceCRC: binary.LittleEndian.Uint32(f), TargetCRC: binary.LittleEndian.Uint32(f[4:])}, nil
	}
	return Info{}, errors.New("not an IPS, UPS, BPS or xdelta patch")
}

// Apply patches rom and returns the result. For UPS and BPS the base ROM
// and the result are checked against the patch's checksums.
func Apply(p, rom []byte) ([]byte, error) {
	info, err := Read(p)
	if err != nil {
		return nil, err
	}
	if !info.Matches(rom) {
		return nil, ErrWrongBase
	}
	var out []byte
	switch info.Format {
	case "IPS":
		out, err = applyIPS(p, rom)
	case "UPS":
		out, err = applyUPS(p, rom, info)
	case "BPS":
		out, err = applyBPS(p, rom, info)
	case "xdelta":
		out, err = applyVCDIFF(p, rom)
	}
	if err != nil {
		return nil, err
	}
	if info.Checked() && (len(out) != info.TargetSize || crc32.ChecksumIEEE(out) != info.TargetCRC) {
		return nil, errors.New("the patched ROM doesn't match the patch's checksum")
	}
	return out, nil
}

// applyIPS: "PATCH", then records of offset (u24 BE), size (u16 BE) and
// data, or size 0 for a run (count u16 BE, byte); "EOF", then optionally a
// u24 size to truncate to.
func applyIPS(p, rom []byte) ([]byte, error) {
	out := append([]byte{}, rom...)
	pos := 5
	be24 := func(b []byte) int { return int(b[0])<<16 | int(b[1])<<8 | int(b[2]) }
	for {
		if pos+3 > len(p) {
			return nil, errors.New("IPS patch is truncated")
		}
		if string(p[pos:pos+3]) == "EOF" {
			if pos+6 <= len(p) {
				if n := be24(p[pos+3:]); n < len(out) {
					out = out[:n]
				}
			}
			return out, nil
		}
		if pos+5 > len(p) {
			return nil, errors.New("IPS patch is truncated")
		}
		at, size := be24(p[pos:]), int(binary.BigEndian.Uint16(p[pos+3:]))
		pos += 5
		var data []byte
		if size == 0 {
			if pos+3 > len(p) {
				return nil, errors.New("IPS patch is truncated")
			}
			data = bytes.Repeat([]byte{p[pos+2]}, int(binary.BigEndian.Uint16(p[pos:])))
			pos += 3
		} else {
			if pos+size > len(p) {
				return nil, errors.New("IPS patch is truncated")
			}
			data = p[pos : pos+size]
			pos += size
		}
		if need := at + len(data); need > len(out) {
			if need > 1<<30 {
				return nil, errors.New("IPS patch writes past 1 GB")
			}
			out = append(out, make([]byte, need-len(out))...)
		}
		copy(out[at:], data)
	}
}

// applyUPS: after the sizes, hunks of (skip count, bytes XORed with the
// source up to a 0 byte), ending 12 bytes from the end.
func applyUPS(p, rom []byte, info Info) ([]byte, error) {
	out := make([]byte, info.TargetSize)
	copy(out, rom)
	r := &reader{b: p[:len(p)-12], pos: 4}
	r.number()
	r.number()
	at := 0
	for r.err == nil && r.pos < len(r.b) {
		at += r.number()
		for r.err == nil {
			x := r.byte()
			if x == 0 {
				at++
				break
			}
			if at < len(out) {
				src := byte(0)
				if at < len(rom) {
					src = rom[at]
				}
				out[at] = src ^ x
			}
			at++
		}
	}
	return out, r.err
}

// applyBPS: after the sizes and metadata, actions until 12 bytes from the
// end: copy from the source at the same place, take bytes from the patch,
// or copy from elsewhere in the source or the output so far.
func applyBPS(p, rom []byte, info Info) ([]byte, error) {
	out := make([]byte, 0, info.TargetSize)
	r := &reader{b: p[:len(p)-12], pos: 4}
	r.number()
	r.number()
	r.pos += r.number() // metadata
	srcRel, dstRel := 0, 0
	signed := func(n int) int {
		if n&1 != 0 {
			return -(n >> 1)
		}
		return n >> 1
	}
	for r.err == nil && r.pos < len(r.b) {
		n := r.number()
		cmd, length := n&3, n>>2+1
		if len(out)+length > info.TargetSize {
			return nil, errors.New("BPS patch writes past the end of the ROM")
		}
		switch cmd {
		case 0: // source read
			if len(out)+length > len(rom) {
				return nil, errors.New("BPS patch reads past the end of the base ROM")
			}
			out = append(out, rom[len(out):len(out)+length]...)
		case 1: // target read
			if r.pos+length > len(r.b) {
				return nil, errors.New("BPS patch is truncated")
			}
			out = append(out, r.b[r.pos:r.pos+length]...)
			r.pos += length
		case 2: // source copy
			srcRel += signed(r.number())
			if srcRel < 0 || srcRel+length > len(rom) {
				return nil, errors.New("BPS patch reads outside the base ROM")
			}
			out = append(out, rom[srcRel:srcRel+length]...)
			srcRel += length
		case 3: // target copy, byte by byte: it may overlap what it writes
			dstRel += signed(r.number())
			if dstRel < 0 || dstRel >= len(out) {
				return nil, errors.New("BPS patch copies from outside the output")
			}
			for i := 0; i < length; i++ {
				out = append(out, out[dstRel])
				dstRel++
			}
		}
	}
	return out, r.err
}

// reader decodes the variable-length numbers UPS and BPS use: 7 bits per
// byte, low first, the last byte flagged with 0x80, each continuation
// adding one (so every number has exactly one encoding).
type reader struct {
	b   []byte
	pos int
	err error
}

func (r *reader) byte() byte {
	if r.pos >= len(r.b) {
		if r.err == nil {
			r.err = errors.New("patch is truncated")
		}
		return 0
	}
	r.pos++
	return r.b[r.pos-1]
}

func (r *reader) number() int {
	n, shift := 0, 1
	for i := 0; r.err == nil; i++ {
		if i > 8 {
			r.err = fmt.Errorf("bad number in patch at %d", r.pos)
			break
		}
		x := r.byte()
		n += int(x&0x7F) * shift
		if x&0x80 != 0 {
			break
		}
		shift <<= 7
		n += shift
	}
	return n
}
