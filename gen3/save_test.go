package gen3

import (
	"encoding/binary"
	"os"
	"testing"
)

var vanillaLayout = SaveLayout{SectorDataSize: 0xF80, VerifyChecksums: true,
	Block2Size: 0xF24, Block1Size: 0x3D68, StorageSize: 0x83D0}

// buildSave makes a two-slot save. fill writes each slot's blocks; slot 2 is
// written rotated, like the game does.
func buildSave(counters [2]uint32, fill func(slot int, block2, block1, storage []byte)) []byte {
	data := make([]byte, 2*sectorsPerSlot*sectorSize)
	for slot := 0; slot < 2; slot++ {
		size := vanillaLayout.SectorDataSize
		b2, b1, st := make([]byte, size), make([]byte, 4*size), make([]byte, 9*size)
		fill(slot, b2, b1, st)
		chunk := func(id int) []byte {
			switch {
			case id == 0:
				return b2
			case id <= 4:
				return b1[(id-1)*size : id*size]
			default:
				return st[(id-5)*size : (id-4)*size]
			}
		}
		totals := map[int]int{0: 0xF24, 4: 0x3D68 - 3*size, 13: 0x83D0 - 8*size}
		for i := 0; i < sectorsPerSlot; i++ {
			id := (i + slot) % sectorsPerSlot
			sec := data[(slot*sectorsPerSlot+i)*sectorSize:]
			copy(sec, chunk(id))
			n := size
			if t, ok := totals[id]; ok {
				n = t
			}
			binary.LittleEndian.PutUint16(sec[footerID:], uint16(id))
			binary.LittleEndian.PutUint16(sec[footerChecksum:], sectorChecksum(sec[:n]))
			binary.LittleEndian.PutUint32(sec[footerSignature:], saveSignature)
			binary.LittleEndian.PutUint32(sec[footerCounter:], counters[slot])
		}
	}
	return data
}

func TestParseSavePicksNewestValidSlot(t *testing.T) {
	data := buildSave([2]uint32{7, 8}, func(slot int, b2, b1, st []byte) {
		copy(b2, encode("RED", 8))
		b1[0x34] = 1
		b1[0x38+32] = byte(10 + slot) // party species: 10 in slot 1, 11 in slot 2
		// Box 2, slot 3: species 25.
		st[4+(1*BoxSlots+2)*BoxSize+32] = 25
	})

	s, err := ParseSave(data, vanillaLayout)
	if err != nil {
		t.Fatal(err)
	}
	if name, _ := s.Trainer(); s.Counter != 8 || name != "RED" {
		t.Errorf("counter %d, name %q", s.Counter, name)
	}
	party, _ := s.Party(false)
	if len(party) != 1 || party[0].Species != 11 {
		t.Errorf("party = %+v", party)
	}
	box, _ := s.Box(1, BoxFormat{Size: BoxSize})
	if box[2] == nil || box[2].Species != 25 || box[0] != nil {
		t.Errorf("box 2 = %v", box[:3])
	}

	// Corrupt the newest slot: the older one is used instead.
	data[(sectorsPerSlot+3)*sectorSize+10] ^= 0xFF
	s, err = ParseSave(data, vanillaLayout)
	if err != nil || s.Counter != 7 {
		t.Errorf("after corruption: counter %v, %v", s, err)
	}
}

// Against the real Radical Red save and ROM.
func TestRadicalRedSave(t *testing.T) {
	path := os.Getenv("TALLGRASS_SAVE")
	if path == "" {
		t.Skip("set TALLGRASS_SAVE (and TALLGRASS_ROM) to run")
	}
	read := romReader(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ParseSave(data, SaveLayout{SectorDataSize: 0xFF0})
	if err != nil {
		t.Fatal(err)
	}
	name, id := s.Trainer()
	party, _ := s.Party(false)
	if name != "Cameron" || id != 0x18CCFAFC || len(party) == 0 || party[0].Species != 155 {
		t.Fatalf("trainer %q %08X, party %+v", name, id, party)
	}
	if got := s.BoxName(0, 0x8344); got != "Box1" {
		t.Errorf("box 1 name = %q", got)
	}
	for box, want := range map[int]string{13: "Box14", 14: "Box15", 24: "Box25"} {
		if got := s.BoxName(box, 0x8344); got != want {
			t.Errorf("box %d name = %q, want %q", box+1, got, want)
		}
	}

	levels := NewLevels(read, 0x097B98EC, 0x0915514C, 256)
	if lv, err := levels.Level(155, party[0].Experience); err != nil || lv != party[0].Level {
		t.Errorf("level from exp %d = %d (%v), party says %d", party[0].Experience, lv, err, party[0].Level)
	}
	for exp, want := range map[uint32]int{0: 1, 178: 5, 179: 6, 236: 7, 1059860: 100, 9999999: 100} {
		if lv, _ := levels.Level(155, exp); lv != want {
			t.Errorf("medium slow exp %d: level %d, want %d", exp, lv, want)
		}
	}
}

// Radical Red's Box 1 slot 1, read from RAM: a Wooloo.
const wooloo = "" +
	"85 85 50 b9 fc fa cc 18 d1 e3 e3 e0 e3 e3 ff 03 09 00 02 02 " +
	"bd d5 e1 d9 e6 e3 e2 00 63 04 00 00 40 00 00 00 00 32 03 2d " +
	"84 f0 06 00 00 00 00 00 00 00 00 59 04 02 84 5b e6 19"

func TestDecodeCompressed(t *testing.T) {
	p, err := DecodeCompressed(mustHex(t, wooloo))
	if err != nil {
		t.Fatal(err)
	}
	if p.Species != 1123 || p.Nickname != "Wooloo" || p.OTName != "Cameron" || p.Experience != 64 || p.Friendship != 50 {
		t.Errorf("got %+v", p)
	}
	if len(p.Moves) != 3 || p.Moves[0].ID != 45 || p.Moves[1].ID != 33 || p.Moves[2].ID != 111 {
		t.Errorf("moves = %+v", p.Moves)
	}
	if p.MetLocation != 0x59 || p.MetLevel != 4 || p.IVs.HP != 4 || p.IVs.Attack != 28 {
		t.Errorf("met %#x Lv %d, IVs %+v", p.MetLocation, p.MetLevel, p.IVs)
	}
	if empty, _ := DecodeCompressed(make([]byte, CompressedSize)); empty != nil {
		t.Error("empty slot should be nil")
	}

	if os.Getenv("TALLGRASS_ROM") == "" {
		return
	}
	read := romReader(t)
	names := NewNames(read, NameTables{Moves: 0x090F3188})
	species, _ := names.Species(1123)
	moves := []string{}
	for _, m := range p.Moves {
		n, _ := names.Move(m.ID)
		moves = append(moves, n)
	}
	lv, _ := NewLevels(read, 0x097B98EC, 0x0915514C, 256).Level(1123, p.Experience)
	t.Logf("%s Lv %d, moves %v", species, lv, moves)
	if species != "Wooloo" || lv != 4 {
		t.Errorf("species %q, level %d", species, lv)
	}
}

func TestTitleCase(t *testing.T) {
	for in, want := range map[string]string{
		"BULBASAUR": "Bulbasaur", "POKé BALL": "Poké Ball", "MR. MIME": "Mr. Mime", "HO-OH": "Ho-Oh",
		"TM01": "TM01", "S.S. TICKET": "S.S. Ticket", "FARFETCH'D": "Farfetch'd", "Pound": "Pound",
		"LITTLEROOT TOWN": "Littleroot Town", "": "",
	} {
		if got := TitleCase(in); got != want {
			t.Errorf("TitleCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadNameFollowsPointers(t *testing.T) {
	mem := regions{
		0x08000000: append(append([]byte{}, 0x00, 0x10, 0x00, 0x08, 0xE8, 0xED), make([]byte, 8)...), // pointer to 0x08001000
		0x08001000: append(EncodeText("Safety Goggles"), make([]byte, 20)...),
		0x08002000: append(EncodeText("Quick Claw"), make([]byte, 20)...),
	}
	for addr, want := range map[uint32]string{0x08000000: "Safety Goggles", 0x08002000: "Quick Claw"} {
		if got, err := ReadName(mem.read, addr, 14); got != want {
			t.Errorf("ReadName(%08X) = %q, %v; want %q", addr, got, err, want)
		}
	}
}
