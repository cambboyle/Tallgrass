package patch

import (
	"errors"
	"fmt"
	"hash/adler32"
)

// VCDIFF (RFC 3284), the format xdelta3 writes: many DS hacks ship as
// .xdelta patches. Windows of instructions build the output from the
// source ROM, data carried in the patch and the output so far. xdelta3
// adds an Adler-32 per window (checked here, which also catches a wrong
// base ROM) and an application header. Secondary compression (xdelta3
// -S djw/fgk/lzma) and custom code tables aren't supported.

var vcdiffMagic = []byte{0xD6, 0xC3, 0xC4}

const (
	vcdDecompress = 1 << 0 // header: secondary compressor ID follows
	vcdCodeTable  = 1 << 1 // header: custom code table follows
	vcdAppHeader  = 1 << 2 // header (xdelta3): application data follows

	vcdSource  = 1 << 0 // window: copies come from the source file
	vcdTarget  = 1 << 1 // window: copies come from earlier output
	vcdAdler32 = 1 << 2 // window (xdelta3): Adler-32 of the window's output

	instNoop, instAdd, instRun, instCopy = 0, 1, 2, 3
)

type vcdInst struct{ typ, size, mode byte }

// vcdCodes is RFC 3284's default code table: each byte of the instruction
// section is one or two instructions.
var vcdCodes = func() (t [256][2]vcdInst) {
	i := 0
	t[i][0] = vcdInst{instRun, 0, 0}
	i++
	for size := 0; size <= 17; size++ {
		t[i][0] = vcdInst{instAdd, byte(size), 0}
		i++
	}
	for mode := 0; mode <= 8; mode++ {
		t[i][0] = vcdInst{instCopy, 0, byte(mode)}
		i++
		for size := 4; size <= 18; size++ {
			t[i][0] = vcdInst{instCopy, byte(size), byte(mode)}
			i++
		}
	}
	for mode := 0; mode <= 5; mode++ {
		for add := 1; add <= 4; add++ {
			for cp := 4; cp <= 6; cp++ {
				t[i] = [2]vcdInst{{instAdd, byte(add), 0}, {instCopy, byte(cp), byte(mode)}}
				i++
			}
		}
	}
	for mode := 6; mode <= 8; mode++ {
		for add := 1; add <= 4; add++ {
			t[i] = [2]vcdInst{{instAdd, byte(add), 0}, {instCopy, 4, byte(mode)}}
			i++
		}
	}
	for mode := 0; mode <= 8; mode++ {
		t[i] = [2]vcdInst{{instCopy, 4, byte(mode)}, {instAdd, 1, 0}}
		i++
	}
	return t
}()

// vcdReader reads VCDIFF integers: big-endian base 128, high bit = more.
type vcdReader struct {
	b   []byte
	pos int
	err error
}

func (r *vcdReader) fail(msg string) {
	if r.err == nil {
		r.err = errors.New(msg)
	}
}

func (r *vcdReader) byte() byte {
	if r.pos >= len(r.b) {
		r.fail("xdelta patch is truncated")
		return 0
	}
	r.pos++
	return r.b[r.pos-1]
}

func (r *vcdReader) int() int {
	n := 0
	for i := 0; ; i++ {
		x := r.byte()
		if r.err != nil {
			return 0
		}
		if i > 8 || n > 1<<50 {
			r.fail("bad number in xdelta patch")
			return 0
		}
		n = n<<7 | int(x&0x7F)
		if x&0x80 == 0 {
			return n
		}
	}
}

func (r *vcdReader) bytes(n int) []byte {
	if n < 0 || r.pos+n > len(r.b) {
		r.fail("xdelta patch is truncated")
		return nil
	}
	r.pos += n
	return r.b[r.pos-n : r.pos]
}

// applyVCDIFF decodes a VCDIFF patch against source.
func applyVCDIFF(p, source []byte) ([]byte, error) {
	r := &vcdReader{b: p, pos: 4}
	hdr := r.byte()
	if hdr&vcdDecompress != 0 {
		return nil, errors.New("this xdelta patch uses secondary compression, which isn't supported yet: make it with xdelta3 -S none")
	}
	if hdr&vcdCodeTable != 0 {
		return nil, errors.New("this xdelta patch uses a custom code table, which isn't supported")
	}
	if hdr&vcdAppHeader != 0 {
		r.bytes(r.int())
	}
	var out []byte
	for r.err == nil && r.pos < len(p) {
		win := r.byte()
		var seg []byte
		if win&(vcdSource|vcdTarget) != 0 {
			length, pos := r.int(), r.int()
			from := source
			if win&vcdTarget != 0 {
				from = out
			}
			if pos < 0 || length < 0 || pos+length > len(from) {
				return nil, ErrWrongBase // the patch reads past the end of the base ROM
			}
			seg = from[pos : pos+length]
		}
		r.int() // length of the delta encoding
		size := r.int()
		if size < 0 || len(out)+size > 1<<30 {
			return nil, errors.New("xdelta patch makes a file over 1 GB")
		}
		if r.byte() != 0 {
			return nil, errors.New("this xdelta patch uses secondary compression, which isn't supported yet: make it with xdelta3 -S none")
		}
		dataLen, instLen, addrLen := r.int(), r.int(), r.int()
		var sum []byte
		if win&vcdAdler32 != 0 {
			sum = r.bytes(4)
		}
		data := &vcdReader{b: r.bytes(dataLen)}
		inst := &vcdReader{b: r.bytes(instLen)}
		addr := &vcdReader{b: r.bytes(addrLen)}
		if r.err != nil {
			return nil, r.err
		}
		w, err := vcdWindow(seg, size, data, inst, addr)
		if err != nil {
			return nil, err
		}
		if sum != nil && adler32.Checksum(w) != uint32(sum[0])<<24|uint32(sum[1])<<16|uint32(sum[2])<<8|uint32(sum[3]) {
			return nil, ErrWrongBase // the output doesn't match: almost always the wrong base ROM
		}
		out = append(out, w...)
	}
	return out, r.err
}

// vcdWindow builds one window's output. Copy addresses run through the
// source segment and then the window itself.
func vcdWindow(seg []byte, size int, data, inst, addr *vcdReader) ([]byte, error) {
	out := make([]byte, 0, size)
	var near [4]int
	var same [3 * 256]int
	next := 0
	for inst.err == nil && inst.pos < len(inst.b) {
		for _, in := range vcdCodes[inst.byte()] {
			if in.typ == instNoop {
				continue
			}
			n := int(in.size)
			if n == 0 {
				n = inst.int()
			}
			if len(out)+n > size {
				return nil, errors.New("xdelta patch writes past its window")
			}
			switch in.typ {
			case instAdd:
				out = append(out, data.bytes(n)...)
			case instRun:
				b := data.byte()
				for i := 0; i < n; i++ {
					out = append(out, b)
				}
			case instCopy:
				here := len(seg) + len(out)
				var a int
				switch m := int(in.mode); {
				case m == 0:
					a = addr.int()
				case m == 1:
					a = here - addr.int()
				case m < 6:
					a = near[m-2] + addr.int()
				default:
					a = same[(m-6)*256+int(addr.byte())]
				}
				near[next] = a
				next = (next + 1) % len(near)
				same[a%len(same)] = a
				if a < 0 || a >= here {
					return nil, fmt.Errorf("xdelta patch copies from outside its window")
				}
				for i := 0; i < n; i++ { // byte by byte: a copy may overlap its own output
					if p := a + i; p < len(seg) {
						out = append(out, seg[p])
					} else {
						out = append(out, out[p-len(seg)])
					}
				}
			}
			if err := firstErr(data.err, inst.err, addr.err); err != nil {
				return nil, err
			}
		}
	}
	if err := firstErr(data.err, inst.err, addr.err); err != nil {
		return nil, err
	}
	if len(out) != size {
		return nil, errors.New("xdelta window is the wrong size")
	}
	return out, nil
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
