package patch

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"math/rand"
	"testing"
)

func number(n int) []byte {
	var out []byte
	for {
		x := byte(n & 0x7F)
		n >>= 7
		if n == 0 {
			return append(out, x|0x80)
		}
		out = append(out, x)
		n--
	}
}

func footer(p, src, dst []byte) []byte {
	p = binary.LittleEndian.AppendUint32(p, crc32.ChecksumIEEE(src))
	p = binary.LittleEndian.AppendUint32(p, crc32.ChecksumIEEE(dst))
	return binary.LittleEndian.AppendUint32(p, crc32.ChecksumIEEE(p))
}

// encodeUPS makes a UPS patch from src to dst.
func encodeUPS(src, dst []byte) []byte {
	p := append([]byte("UPS1"), number(len(src))...)
	p = append(p, number(len(dst))...)
	at := func(b []byte, i int) byte {
		if i < len(b) {
			return b[i]
		}
		return 0
	}
	last := 0
	for i := 0; i < len(dst); {
		if at(src, i) == dst[i] {
			i++
			continue
		}
		p = append(p, number(i-last)...)
		for i < len(dst) && at(src, i) != dst[i] {
			p = append(p, at(src, i)^dst[i])
			i++
		}
		p = append(p, 0)
		i++
		last = i
	}
	return footer(p, src, dst)
}

// bpsAction is one BPS command; offset is used by the copy commands.
type bpsAction struct {
	cmd, length, offset int
	data                []byte
}

func encodeBPS(src, dst []byte, actions []bpsAction) []byte {
	p := append([]byte("BPS1"), number(len(src))...)
	p = append(p, number(len(dst))...)
	p = append(p, number(0)...) // no metadata
	for _, a := range actions {
		p = append(p, number((a.length-1)<<2|a.cmd)...)
		switch a.cmd {
		case 1:
			p = append(p, a.data...)
		case 2, 3:
			n := a.offset << 1
			if a.offset < 0 {
				n = -a.offset<<1 | 1
			}
			p = append(p, number(n)...)
		}
	}
	return footer(p, src, dst)
}

func randomROM(n int) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(int64(n))).Read(b)
	return b
}

func TestUPS(t *testing.T) {
	src := randomROM(4096)
	for _, size := range []int{4096, 6000, 3000} {
		dst := append([]byte{}, src...)
		if size > len(dst) {
			dst = append(dst, randomROM(size-len(dst))...)
		}
		dst = dst[:size]
		dst[10], dst[11], dst[2000] = 1, 2, 3
		out, err := Apply(encodeUPS(src, dst), src)
		if err != nil || !bytes.Equal(out, dst) {
			t.Errorf("size %d: %v, equal %v", size, err, bytes.Equal(out, dst))
		}
	}
}

func TestBPS(t *testing.T) {
	src := []byte("HELLO WORLD, THIS IS THE BASE ROM")
	// Keep "HELLO ", write "TALL", copy "GRASS" from... the patch, repeat
	// "ab" with an overlapping target copy, then copy "BASE ROM" from the
	// source.
	dst := []byte("HELLO TALLGRASS abababab BASE ROM")
	actions := []bpsAction{
		{cmd: 0, length: 6},
		{cmd: 1, length: 4, data: []byte("TALL")},
		{cmd: 1, length: 8, data: []byte("GRASS ab")},
		{cmd: 3, length: 6, offset: 16}, // from dst[16] ("ab"), overlapping
		{cmd: 2, length: 9, offset: 24}, // " BASE ROM" at src[24]
	}
	p := encodeBPS(src, dst, actions)
	out, err := Apply(p, src)
	if err != nil || string(out) != string(dst) {
		t.Fatalf("Apply = %q, %v", out, err)
	}
	info, _ := Read(p)
	if info.Format != "BPS" || !info.Matches(src) || info.Matches(dst) {
		t.Errorf("info = %+v", info)
	}
	if _, err := Apply(p, dst); !errors.Is(err, ErrWrongBase) {
		t.Errorf("wrong base: %v", err)
	}
	p[10] ^= 1
	if _, err := Read(p); err == nil {
		t.Error("a damaged patch should fail its checksum")
	}
}

func TestIPS(t *testing.T) {
	src := make([]byte, 16)
	p := []byte("PATCH")
	p = append(p, 0, 0, 2, 0, 3, 'a', 'b', 'c') // 3 bytes at 2
	p = append(p, 0, 0, 8, 0, 0, 0, 4, 'z')     // run of 4 'z' at 8
	p = append(p, 0, 0, 18, 0, 1, '!')          // past the end: grows
	p = append(p, 'E', 'O', 'F')
	out, err := Apply(p, src)
	want := []byte("\x00\x00abc\x00\x00\x00zzzz\x00\x00\x00\x00\x00\x00!")
	if err != nil || !bytes.Equal(out, want) {
		t.Errorf("Apply = %q, %v", out, err)
	}
	if !bytes.Equal(src, make([]byte, 16)) {
		t.Error("Apply changed the base ROM")
	}
	truncated, _ := Apply(append(p, 0, 0, 4), src)
	if len(truncated) != 4 {
		t.Errorf("truncate: %d bytes", len(truncated))
	}
	if _, err := Apply([]byte("PATCH\x00\x00"), src); err == nil {
		t.Error("truncated IPS should fail")
	}
}

func TestNotAPatch(t *testing.T) {
	if _, err := Read([]byte("hello")); err == nil {
		t.Error("expected an error")
	}
}
