// Package gen4 reads Generation 4 games (Diamond/Pearl/Platinum and
// HeartGold/SoulSilver, and hacks of them): DS ROM data and save files.
package gen4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var le = binary.LittleEndian

// ROM is a DS ROM's file system, read on demand from disk.
type ROM struct {
	f      *os.File
	Code   string // game code, e.g. "CPUE" (Platinum US), "IPKE" (HeartGold US)
	fatOff uint32
	paths  map[string]int // file path -> file ID
}

// ReadGameCode returns a DS ROM's 4-letter game code without loading more.
func ReadGameCode(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b := make([]byte, 4)
	if _, err := f.ReadAt(b, 0x0C); err != nil {
		return "", err
	}
	return string(b), nil
}

func OpenROM(path string) (*ROM, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	r := &ROM{f: f}
	h := make([]byte, 0x50)
	if _, err := f.ReadAt(h, 0); err != nil {
		f.Close()
		return nil, err
	}
	r.Code = string(h[0x0C:0x10])
	fnt := make([]byte, le.Uint32(h[0x44:]))
	if _, err := f.ReadAt(fnt, int64(le.Uint32(h[0x40:]))); err != nil {
		f.Close()
		return nil, fmt.Errorf("reading file names: %w", err)
	}
	r.fatOff = le.Uint32(h[0x48:])
	r.paths = map[string]int{}
	if err := r.walk(fnt, 0xF000, "/"); err != nil {
		f.Close()
		return nil, err
	}
	return r, nil
}

func (r *ROM) Close() error { return r.f.Close() }

// walk reads the file name table: per directory, an offset to its entries
// and its first file ID; entries are length-prefixed names, with the high
// bit marking subdirectories (followed by their directory ID).
func (r *ROM) walk(fnt []byte, dir uint16, prefix string) error {
	i := int(dir&0xFFF) * 8
	if i+8 > len(fnt) {
		return errors.New("corrupt file name table")
	}
	o := int(le.Uint32(fnt[i:]))
	id := int(le.Uint16(fnt[i+4:]))
	for o < len(fnt) {
		t := fnt[o]
		o++
		if t == 0 {
			return nil
		}
		n := int(t & 0x7F)
		if o+n > len(fnt) {
			return errors.New("corrupt file name table")
		}
		name := string(fnt[o : o+n])
		o += n
		if t&0x80 != 0 {
			sub := le.Uint16(fnt[o:])
			o += 2
			if err := r.walk(fnt, sub, prefix+name+"/"); err != nil {
				return err
			}
		} else {
			r.paths[prefix+name] = id
			id++
		}
	}
	return errors.New("corrupt file name table")
}

func (r *ROM) Has(path string) bool { _, ok := r.paths[path]; return ok }

// File reads a file by path.
func (r *ROM) File(path string) ([]byte, error) {
	id, ok := r.paths[path]
	if !ok {
		return nil, fmt.Errorf("%s isn't in the ROM", path)
	}
	return r.fileByID(id)
}

func (r *ROM) fileByID(id int) ([]byte, error) {
	fat := make([]byte, 8)
	if _, err := r.f.ReadAt(fat, int64(r.fatOff)+int64(id)*8); err != nil {
		return nil, err
	}
	start, end := le.Uint32(fat), le.Uint32(fat[4:])
	b := make([]byte, end-start)
	if _, err := r.f.ReadAt(b, int64(start)); err != nil && err != io.EOF {
		return nil, err
	}
	return b, nil
}

// Archive reads a NARC: a container of numbered files.
func (r *ROM) Archive(path string) ([][]byte, error) {
	b, err := r.File(path)
	if err != nil {
		return nil, err
	}
	return parseNARC(b)
}

func parseNARC(b []byte) ([][]byte, error) {
	if len(b) < 0x10 || string(b[:4]) != "NARC" {
		return nil, errors.New("not a NARC archive")
	}
	o := int(le.Uint16(b[0x0C:]))
	section := func(magic string) (int, int, error) {
		if o+8 > len(b) || string(b[o:o+4]) != magic {
			return 0, 0, fmt.Errorf("NARC: missing %s", magic)
		}
		return o, int(le.Uint32(b[o+4:])), nil
	}
	fatAt, fatLen, err := section("BTAF")
	if err != nil {
		return nil, err
	}
	n := int(le.Uint16(b[fatAt+8:]))
	o += fatLen
	_, fntLen, err := section("BTNF")
	if err != nil {
		return nil, err
	}
	o += fntLen
	if _, _, err := section("GMIF"); err != nil {
		return nil, err
	}
	base := o + 8
	files := make([][]byte, n)
	for i := range files {
		e := fatAt + 12 + i*8
		start, end := base+int(le.Uint32(b[e:])), base+int(le.Uint32(b[e+4:]))
		if start > end || end > len(b) {
			return nil, fmt.Errorf("NARC: file %d out of range", i)
		}
		files[i] = b[start:end]
	}
	return files, nil
}

// decodeMessages decrypts a Gen 4 message file. Its table of (offset,
// length) pairs is XORed with a key from the file's seed and the entry
// number; each string's characters are XORed with a key that starts from
// the entry number and steps by 0x493D.
func decodeMessages(b []byte) ([]string, error) {
	if len(b) < 4 {
		return nil, errors.New("message file too short")
	}
	count, seed := int(le.Uint16(b)), uint32(le.Uint16(b[2:]))
	out := make([]string, count)
	for i := range out {
		k := (seed * 0x2FD * uint32(i+1)) & 0xFFFF
		k32 := k | k<<16
		e := 4 + i*8
		if e+8 > len(b) {
			return nil, errors.New("message table out of range")
		}
		off, n := int(le.Uint32(b[e:])^k32), int(le.Uint32(b[e+4:])^k32)
		if off < 0 || n < 0 || off+n*2 > len(b) {
			return nil, errors.New("message out of range")
		}
		key := uint16(0x91BD3 * (i + 1))
		chars := make([]uint16, 0, n)
		for j := 0; j < n; j++ {
			c := le.Uint16(b[off+j*2:]) ^ key
			key += 0x493D
			if c == 0xFFFF {
				break
			}
			chars = append(chars, c)
		}
		if len(chars) > 0 && chars[0] == 0xF100 {
			chars = unpack9(chars[1:])
		}
		out[i] = decodeChars(chars)
	}
	return out, nil
}

// unpack9 expands compressed text (used for trainer names): 9-bit
// characters packed 15 bits to a 16-bit word, low bits first, ending at
// 0x1FF.
func unpack9(words []uint16) []uint16 {
	var out []uint16
	var acc uint32
	bits := 0
	for _, w := range words {
		acc |= uint32(w&0x7FFF) << bits
		bits += 15
		for bits >= 9 {
			c := uint16(acc & 0x1FF)
			acc >>= 9
			bits -= 9
			if c == 0x1FF {
				return out
			}
			out = append(out, c)
		}
	}
	return out
}

// titleCase turns "MR. MIME" into "Mr. Mime", for games whose name lists
// are all capitals.
func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// titleCaseIfCaps title-cases all-capitals text and leaves the rest alone.
func titleCaseIfCaps(s string) string {
	if s == strings.ToUpper(s) {
		return strings.ReplaceAll(titleCase(s), "Pkmn", "PKMN")
	}
	return s
}
