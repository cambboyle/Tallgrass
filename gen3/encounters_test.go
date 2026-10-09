package gen3

import (
	"encoding/binary"
	"testing"
)

// header builds a 20-byte wild header entry.
func header(group, num uint8, info ...uint32) []byte {
	b := make([]byte, wildHeaderSize)
	b[0], b[1] = group, num
	for i, p := range info {
		binary.LittleEndian.PutUint32(b[4+i*4:], p)
	}
	return b
}

// list builds an encounter list: rate byte, pointer to slots.
func list(slots uint32) []byte {
	b := make([]byte, 8)
	b[0] = 20
	binary.LittleEndian.PutUint32(b[4:], slots)
	return b
}

// slotData builds n slots of (level, level, species).
func slotData(species ...int) []byte {
	var b []byte
	for _, s := range species {
		b = append(b, 3, 4)
		b = binary.LittleEndian.AppendUint16(b, uint16(s))
	}
	return b
}

func TestWildPeriodsFallBackPerMethod(t *testing.T) {
	end := []byte{0xFF}
	pad := make([]byte, 64*wildHeaderSize)
	mem := regions{
		// Main table: map 3.19 has grass and surfing.
		0x1000: append(append(header(3, 19, 0x2000, 0x2100), end...), pad...),
		0x2000: list(0x3000),
		0x2100: list(0x3100),
		0x3000: slotData(16, 16, 19, 19, 19, 19, 19, 19, 19, 19, 19, 25), // Pidgey 40%, Rattata 59%, Pikachu 1%
		0x3100: slotData(129, 129, 129, 129, 129),
		// Night table overrides only grass.
		0x4000: append(append(header(3, 19, 0x2200), end...), pad...),
		0x2200: list(0x3200),
		0x3200: slotData(163, 163, 163, 163, 163, 163, 163, 163, 163, 163, 163, 163),
	}
	w := NewWild(mem.read, 0x1000, []Period{
		{Name: "Night", Headers: 0x4000, FromHour: 20, ToHour: 4},
		{Name: "Day", FromHour: 4, ToHour: 20}, // no own table: main only
	})

	got, err := w.ForMap(3, 19)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d periods", len(got))
	}

	night, day := got[0], got[1]
	if night.Methods[0].Encounters[0].Species != 163 || night.Methods[0].Encounters[0].Chance != 100 {
		t.Errorf("night grass = %+v", night.Methods[0])
	}
	if night.Methods[1].Method != "Surfing" || night.Methods[1].Encounters[0].Species != 129 {
		t.Errorf("night should fall back to main surfing, got %+v", night.Methods)
	}
	want := []Encounter{{19, 3, 4, 59}, {16, 3, 4, 40}, {25, 3, 4, 1}}
	for i, e := range day.Methods[0].Encounters {
		if e != want[i] {
			t.Errorf("day grass[%d] = %+v, want %+v", i, e, want[i])
		}
	}

	if none, _ := w.ForMap(4, 3); none != nil {
		t.Errorf("map without encounters: %+v", none)
	}
}

func TestPeriodContains(t *testing.T) {
	night := Period{FromHour: 17, ToHour: 4}
	for hour, want := range map[int]bool{16: false, 17: true, 23: true, 0: true, 3: true, 4: false} {
		if night.Contains(hour) != want {
			t.Errorf("hour %d: want %v", hour, want)
		}
	}
}

// Against the real Radical Red ROM: Route 1 (map 3.19) by day and night.
func TestRadicalRedRoute1(t *testing.T) {
	read := romReader(t)
	w := NewWild(read, 0x0872C984, []Period{
		{Name: "Evening & night", Headers: 0x09166428, FromHour: 17, ToHour: 4},
		{Name: "Day", Headers: 0x09166AB8, FromHour: 4, ToHour: 17},
	})
	got, err := w.ForMap(3, 19)
	if err != nil {
		t.Fatal(err)
	}
	names := NewNames(read, NameTables{})
	for _, p := range got {
		for _, m := range p.Methods {
			var list []string
			for _, e := range m.Encounters {
				n, _ := names.Species(e.Species)
				list = append(list, n)
			}
			t.Logf("%s / %s: %v", p.Name, m.Method, list)
		}
	}
	has := func(p PeriodEncounters, name string) bool {
		for _, e := range p.Methods[0].Encounters {
			if n, _ := names.Species(e.Species); n == name {
				return true
			}
		}
		return false
	}
	if !has(got[0], "Hoothoot") || has(got[1], "Hoothoot") {
		t.Error("Hoothoot should appear only at night")
	}
	if !has(got[1], "Yungoos") {
		t.Error("Yungoos should appear by day")
	}
}

func TestRadicalRedMapNameAndNational(t *testing.T) {
	read := romReader(t)
	name, err := NewNames(read, NameTables{}).MapName(0x083F1CAC, 4, 0, 0x58, 0x58)
	if err != nil || name != "Pallet Town" {
		t.Errorf("map name = %q, %v", name, err)
	}
	nat := NewNational(read, 0x098218F0)
	if n, _ := nat.Number(155); n != 155 {
		t.Errorf("Cyndaquil national = %d", n)
	}
	if n, _ := nat.Number(1020); n != 19 {
		t.Errorf("Alolan Rattata national = %d", n)
	}
	if sp, _ := nat.Species(19); sp != 19 {
		t.Errorf("national #19 -> species %d, want base Rattata", sp)
	}
}

func TestHasFlag(t *testing.T) {
	flags := make([]byte, 20)
	flags[19] = 1 << 2 // #155
	if !HasFlag(flags, 155) || HasFlag(flags, 154) || HasFlag(flags, 0) || HasFlag(flags, 9999) {
		t.Error("flag lookup wrong")
	}
}
