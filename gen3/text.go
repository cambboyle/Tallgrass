package gen3

import (
	"strings"
	"unicode"
)

// charset maps Gen 3 (Western) character codes to text. Both single quotes
// become a plain apostrophe: Radical Red's map names use the left one (0xB3)
// where other text uses the right one (0xB4).
var charset = map[byte]string{
	0x00: " ", 0x1B: "é", 0xAB: "!", 0xAC: "?", 0xAD: ".", 0xAE: "-",
	0xB0: "…", 0xB1: "“", 0xB2: "”", 0xB3: "'", 0xB4: "'",
	0xB5: "♂", 0xB6: "♀", 0xB8: ",", 0xBA: "/",
	0x2D: "&", 0x5C: "(", 0x5D: ")",
	0x53: "PK", 0x54: "MN", // two glyphs that spell "PKMN"
}

func init() {
	for i := byte(0); i < 10; i++ {
		charset[0xA1+i] = string(rune('0' + i))
	}
	for i := byte(0); i < 26; i++ {
		charset[0xBB+i] = string(rune('A' + i))
		charset[0xD5+i] = string(rune('a' + i))
	}
}

// DecodeText converts Gen 3 encoded text, stopping at the 0xFF terminator.
// Unknown codes become "?".
func DecodeText(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c == 0xFF {
			break
		}
		if s, ok := charset[c]; ok {
			sb.WriteString(s)
		} else {
			sb.WriteString("?")
		}
	}
	return sb.String()
}

// TitleCase turns an all-capitals name (vanilla Gen 3 English) into title
// case: "POKé BALL" → "Poké Ball", "MR. MIME" → "Mr. Mime", "HO-OH" →
// "Ho-Oh". Mixed-case names and words with digits ("TM01") are left alone.
func TitleCase(s string) string {
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return s // already mixed case
		}
	}
	words := strings.Split(s, " ")
	for i, w := range words {
		if strings.IndexFunc(w, unicode.IsDigit) >= 0 {
			continue
		}
		out := []rune(strings.ToLower(w))
		for j := range out {
			if j == 0 || !unicode.IsLetter(out[j-1]) && out[j-1] != '\'' {
				out[j] = unicode.ToUpper(out[j])
			}
		}
		words[i] = string(out)
	}
	return strings.Join(words, " ")
}

// EncodeText is DecodeText's inverse for letters, digits and spaces, ending
// with the 0xFF terminator (enough to search ROMs for names).
func EncodeText(s string) []byte {
	out := make([]byte, 0, len(s)+1)
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			out = append(out, 0xBB+byte(r-'A'))
		case r >= 'a' && r <= 'z':
			out = append(out, 0xD5+byte(r-'a'))
		case r >= '0' && r <= '9':
			out = append(out, 0xA1+byte(r-'0'))
		default:
			out = append(out, 0x00)
		}
	}
	return append(out, 0xFF)
}
