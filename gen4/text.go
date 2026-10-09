package gen4

import "strings"

// Gen 4 text is 16-bit characters from the game's own table. These are the
// ones used in English names; anything else becomes "?".
var charTable = map[uint16]string{
	0x01DE: " ", 0x01AB: "!", 0x01AC: "?", 0x01AD: ",", 0x01AE: ".", 0x01AF: "…",
	0x01B1: "/", 0x01B2: "‘", 0x01B3: "'", 0x01B4: "'", 0x01B5: "“", 0x01B6: "”",
	0x01BE: "-", 0x01BB: "♂", 0x01BC: "♀", 0x0188: "é", 0x01C4: ":", 0x01C0: "(", 0x01C1: ")",
	0x01C5: "&", 0x01C6: "+",
	0x01E0: "PK", 0x01E1: "MN", // two glyphs that spell "PKMN"
}

func decodeChars(chars []uint16) string {
	var sb strings.Builder
	for _, c := range chars {
		switch {
		case c >= 0x012B && c < 0x012B+26:
			sb.WriteByte(byte('A' + c - 0x012B))
		case c >= 0x0145 && c < 0x0145+26:
			sb.WriteByte(byte('a' + c - 0x0145))
		case c >= 0x0121 && c <= 0x012A:
			sb.WriteByte(byte('0' + c - 0x0121))
		default:
			if s, ok := charTable[c]; ok {
				sb.WriteString(s)
			} else {
				sb.WriteString("?")
			}
		}
	}
	return sb.String()
}

// DecodeText reads a name stored in a Pokémon or save: 16-bit characters
// ending at 0xFFFF.
func DecodeText(b []byte) string {
	var chars []uint16
	for i := 0; i+1 < len(b); i += 2 {
		c := le.Uint16(b[i:])
		if c == 0xFFFF {
			break
		}
		chars = append(chars, c)
	}
	return decodeChars(chars)
}

// validText reports whether b holds a terminated name made only of
// characters we know.
func validText(b []byte) bool {
	n := 0
	for i := 0; i+1 < len(b); i += 2 {
		c := le.Uint16(b[i:])
		if c == 0xFFFF {
			return n > 0
		}
		if decodeChars([]uint16{c}) == "?" {
			return false
		}
		n++
	}
	return false
}
