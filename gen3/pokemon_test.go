package gen3

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
)

// Party slot 1 from Radical Red on the Pi: level 6 Cyndaquil.
const radicalRedCyndaquil = "" +
	"05 56 D7 79 FC FA CC 18 BD ED E2 D8 D5 E5 E9 DD E0 FF 02 02 " +
	"BD D5 E1 D9 E6 E3 E2 00 00 00 00 00 9B 00 8B 00 C3 00 00 00 " +
	"00 37 03 00 21 00 2B 00 00 00 00 00 23 1E 23 23 00 00 01 00 " +
	"00 00 00 00 00 00 00 00 00 9D 05 02 0F 16 38 16 00 00 00 00 " +
	"00 00 00 00 06 FF 15 00 15 00 0C 00 0A 00 0B 00 0C 00 0C 00"

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func checkCyndaquil(t *testing.T, p *Pokemon) {
	t.Helper()
	if p == nil {
		t.Fatal("got empty slot")
	}
	if p.Species != 155 || p.Nickname != "Cyndaquil" || p.OTName != "Cameron" {
		t.Errorf("species/nickname/OT = %d %q %q", p.Species, p.Nickname, p.OTName)
	}
	if p.Level != 6 || p.CurrentHP != 21 || p.Stats.HP != 21 {
		t.Errorf("level/HP = %d %d/%d", p.Level, p.CurrentHP, p.Stats.HP)
	}
	if len(p.Moves) != 2 || p.Moves[0] != (Move{33, 35}) || p.Moves[1] != (Move{43, 30}) {
		t.Errorf("moves = %+v", p.Moves)
	}
	if p.MetLocation != 0x9D || p.MetLevel != 5 {
		t.Errorf("met location/level = %#x %d", p.MetLocation, p.MetLevel)
	}
	if p.Nature != "Sassy" || p.NatureUp != "spDef" || p.NatureDown != "speed" {
		t.Errorf("nature = %s +%s -%s", p.Nature, p.NatureUp, p.NatureDown)
	}
	wantIVs := Stats{HP: 15, Attack: 16, Defense: 5, Speed: 16, SpAtk: 3, SpDef: 11}
	if p.IVs != wantIVs {
		t.Errorf("IVs = %+v, want %+v", p.IVs, wantIVs)
	}
}

func TestDecodeUnencrypted(t *testing.T) {
	b := mustHex(t, radicalRedCyndaquil)
	p, err := Decode(b, false)
	if err != nil {
		t.Fatal(err)
	}
	checkCyndaquil(t, p)
}

// Shuffle and encrypt the plain block the way vanilla Gen 3 does, then
// check Decode undoes it.
func TestDecodeEncrypted(t *testing.T) {
	b := mustHex(t, radicalRedCyndaquil)
	pid := binary.LittleEndian.Uint32(b[0:4])
	otid := binary.LittleEndian.Uint32(b[4:8])

	plain := map[byte][]byte{
		'G': append([]byte{}, b[32:44]...),
		'A': append([]byte{}, b[44:56]...),
		'E': append([]byte{}, b[56:68]...),
		'M': append([]byte{}, b[68:80]...),
	}
	for pos, letter := range []byte(subBlockOrders[pid%24]) {
		copy(b[32+pos*12:], plain[letter])
	}
	for i := 32; i < 80; i += 4 {
		binary.LittleEndian.PutUint32(b[i:], binary.LittleEndian.Uint32(b[i:])^pid^otid)
	}

	p, err := Decode(b, true)
	if err != nil {
		t.Fatal(err)
	}
	checkCyndaquil(t, p)
}

func TestDecodeEmptySlot(t *testing.T) {
	p, err := Decode(make([]byte, PartySize), false)
	if err != nil || p != nil {
		t.Errorf("got %+v, %v; want nil, nil", p, err)
	}
}
