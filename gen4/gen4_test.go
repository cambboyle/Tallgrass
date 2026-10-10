package gen4

import (
	"fmt"
	"image/png"
	"os"
	"slices"
	"strings"
	"tallgrass/gen3"
	"testing"
)

func encodeText(s string) []byte {
	b := make([]byte, 0, len(s)*2+2)
	for _, r := range s {
		var c uint16
		switch {
		case r >= 'A' && r <= 'Z':
			c = 0x012B + uint16(r-'A')
		case r >= 'a' && r <= 'z':
			c = 0x0145 + uint16(r-'a')
		}
		b = le.AppendUint16(b, c)
	}
	return le.AppendUint16(b, 0xFFFF)
}

// encode builds a 236-byte party Pokémon the way the game stores it.
func encode(pid uint32, species, level int) []byte {
	b := make([]byte, PartySize)
	le.PutUint32(b, pid)
	le.PutUint16(b[0x08:], uint16(species))
	le.PutUint16(b[0x0C:], 25380) // trainer ID
	le.PutUint32(b[0x10:], 164)   // experience
	le.PutUint16(b[0x28:], 33)    // Tackle
	b[0x30] = 35
	copy(b[0x48:], encodeText("Blaze"))
	copy(b[0x68:], encodeText("Cam"))
	b[0x84] = 5 // met level
	b[0x88+4] = byte(level)
	le.PutUint16(b[0x88+6:], 15)                           // HP
	for i, stat := range []uint16{19, 12, 9, 10, 12, 10} { // max HP, then stats
		le.PutUint16(b[0x88+8+i*2:], stat)
	}

	var sum uint16
	for i := 8; i < BoxSize; i += 2 {
		sum += le.Uint16(b[i:])
	}
	le.PutUint16(b[6:], sum)

	out := make([]byte, PartySize)
	copy(out, b[:8])
	order := blockOrders[((pid&0x3E000)>>13)%24]
	for pos, letter := range order {
		copy(out[8+pos*32:], b[8+int(letter-'A')*32:8+int(letter-'A')*32+32])
	}
	crypt(out[8:BoxSize], uint32(sum))
	copy(out[BoxSize:], b[BoxSize:])
	crypt(out[BoxSize:], pid)
	return out
}

func TestDecode(t *testing.T) {
	for _, pid := range []uint32{0x12345678, 0x0003E000, 7} { // different block orders
		p, err := Decode(encode(pid, 155, 5))
		if err != nil {
			t.Fatal(err)
		}
		if p.Species != 155 || p.Nickname != "Blaze" || p.OTName != "Cam" || p.Level != 5 || p.CurrentHP != 15 ||
			p.Experience != 164 || len(p.Moves) != 1 || p.Moves[0].PP != 35 || p.MetLevel != 5 || p.OTID&0xFFFF != 25380 {
			t.Errorf("pid %08X: %+v", pid, p)
		}
	}
	// Live RAM: the same Pokémon momentarily decrypted in place.
	enc := encode(0x12345678, 155, 5)
	data := append([]byte{}, enc[8:BoxSize]...)
	crypt(data, uint32(le.Uint16(enc[6:])))
	copy(enc[8:], data)
	crypt(enc[BoxSize:], 0x12345678)
	if p, err := Decode(enc); err != nil || p.Species != 155 || p.Level != 5 || p.Nickname != "Blaze" {
		t.Errorf("decrypted in place: %+v, %v", p, err)
	}

	// Held decrypted while the game edits it: the checksum is stale.
	stale := append([]byte{}, enc...)
	stale[0x08+8] ^= 0x01 // change a byte somewhere in the plain blocks
	if _, err := Decode(stale); err == nil {
		t.Error("a save file must not accept a stale checksum")
	}
	if p, err := DecodeLive(stale); err != nil || p.Species != 155 {
		t.Errorf("live, stale checksum: %+v, %v", p, err)
	}

	// Caught part-way: blocks A and B plain, C (nickname) still encrypted.
	torn := append([]byte{}, stale...)
	order := blockOrders[((0x12345678&0x3E000)>>13)%24]
	pos := strings.IndexByte(order, 'C')
	for i := 0; i < 32; i++ {
		torn[8+pos*32+i] ^= 0x5A
	}
	if _, err := DecodeLive(torn); err == nil {
		t.Error("a half-decrypted Pokémon should be rejected")
	}

	if p, err := Decode(make([]byte, BoxSize)); p != nil || err != nil {
		t.Errorf("empty slot: %+v, %v", p, err)
	}
	bad := encode(1, 155, 5)
	bad[20] ^= 0xFF
	if _, err := Decode(bad); err == nil {
		t.Error("corrupt data should fail its checksum")
	}
}

// Real saves and ROMs: set TALLGRASS_DS (a folder of .nds files and
// their .dsv saves, matched by name). Run on the Pi, where the ROMs are.
func TestRealGames(t *testing.T) {
	dir := os.Getenv("TALLGRASS_DS")
	if dir == "" {
		t.Skip("set TALLGRASS_DS to a folder of DS ROMs and .dsv saves")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".dsv") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ".dsv")
		t.Run(stem, func(t *testing.T) {
			data, _ := os.ReadFile(dir + "/" + e.Name())
			save, err := ParseSave(data)
			if err != nil {
				t.Fatal(err)
			}
			d, err := LoadData(dir + "/" + stem + ".nds")
			if err != nil {
				t.Fatal(err)
			}
			name, id := save.Trainer(d.Layout)
			party, err := save.Party(d.Layout)
			if err != nil || len(party) == 0 {
				t.Fatalf("party: %v, %v", party, err)
			}
			p := party[0]
			t.Logf("%s: %s (ID %d), save #%d, %d boxes (%q … %q); %d species, %d moves, %d items",
				d.Layout.Family, name, id&0xFFFF, save.Counter, save.Boxes(d.Layout),
				save.BoxName(d.Layout, 0), save.BoxName(d.Layout, save.Boxes(d.Layout)-1),
				len(d.Species), len(d.Moves), len(d.Items))
			moves := []string{}
			for _, m := range p.Moves {
				moves = append(moves, Name(d.Moves, m.ID))
			}
			t.Logf("party 1: %s (%s) Lv %d (from exp: %d), %d/%d HP, %s, ability %s, met %s, moves %v",
				p.Nickname, Name(d.Species, p.Species), p.Level, d.Level(p.Species, p.Experience), p.CurrentHP, p.Stats.HP,
				p.Nature, Name(d.Abilities, p.Ability), Name(d.Locations, p.MetLocation), moves)
			if name != "Cameron" || d.Level(p.Species, p.Experience) != p.Level || Name(d.Species, p.Species) == "" {
				t.Errorf("unexpected data")
			}
			// Sprites: normal and shiny, saved for a look when
			// TALLGRASS_SPRITES_OUT is set.
			for _, shiny := range []bool{false, true} {
				img, err := d.Sprite(p.Species, p.Female, shiny)
				if err != nil {
					t.Fatalf("sprite (shiny %v): %v", shiny, err)
				}
				opaque := 0
				for _, px := range img.Pix {
					if px != 0 {
						opaque++
					}
				}
				if opaque < 200 {
					t.Errorf("sprite looks empty (%d pixels)", opaque)
				}
				if out := os.Getenv("TALLGRASS_SPRITES_OUT"); out != "" {
					f, _ := os.Create(fmt.Sprintf("%s/%s-%v.png", out, stem, shiny))
					png.Encode(f, img)
					f.Close()
				}
			}
			sp, err := d.SpeciesInfo(p.Species)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("species page: %s %v %+v abilities %v, evolves %+v, first moves %v", sp.Name, sp.Types, sp.BaseStats,
				sp.Abilities, sp.EvolvesTo, sp.LevelMoves[:min(4, len(sp.LevelMoves))])
			if len(sp.Types) == 0 || sp.Types[0] == "" || len(sp.LevelMoves) < 5 || sp.LevelMoves[0].Move == "" || len(sp.EvolvesTo) == 0 {
				t.Error("species page looks incomplete")
			}
			for b := 0; b < save.Boxes(d.Layout); b++ {
				if _, err := save.Box(d.Layout, b); err != nil {
					t.Errorf("box %d: %v", b+1, err)
				}
			}
		})
	}
}

func TestFindGeneral(t *testing.T) {
	l, _ := LayoutFor("IPKE")
	ram := make([]byte, 0x20000)
	general := 0x1234 * 4
	copy(ram[general+l.TrainerName:], encodeText("Cam"))
	le.PutUint32(ram[general+l.TrainerName+16:], 25380) // trainer ID, secret ID 0
	ram[general+l.Party-4] = 1
	copy(ram[general+l.Party:], encode(0x12345678, 155, 5))

	// A decoy: a valid Pokémon with a count, but the wrong trainer.
	decoy := 0x100 * 4
	ram[decoy+l.Party-4] = 1
	copy(ram[decoy+l.Party:], encode(0x9999, 25, 3))
	le.PutUint32(ram[decoy+l.TrainerName+16:], 1)

	addr, err := FindGeneral(ram, MainRAM, l)
	if err != nil || addr != MainRAM+uint32(general) {
		t.Fatalf("got %08X, %v; want %08X", addr, err, MainRAM+general)
	}
	name, _, party, err := LiveParty(ram[general:general+l.GeneralSize()], l)
	if err != nil || name != "Cam" || len(party) != 1 || party[0].Level != 5 {
		t.Errorf("live party: %q %+v %v", name, party, err)
	}
}

func TestHGSSEncounters(t *testing.T) {
	b := make([]byte, encFileSize)
	b[0], b[1] = 25, 15 // grass and surf rates
	for s := 0; s < 12; s++ {
		b[0x08+s] = 3
		le.PutUint16(b[0x14+s*2:], 10)  // morning: all Caterpie
		le.PutUint16(b[0x2C+s*2:], 13)  // day: all Weedle
		le.PutUint16(b[0x44+s*2:], 163) // night: all Hoothoot
	}
	le.PutUint16(b[0x44:], 165) // night slot 0 (20%): Ledyba
	for i := 0; i < 5; i++ {
		b[0x64+i*4], b[0x65+i*4] = 10, 20
		le.PutUint16(b[0x66+i*4:], 60) // Poliwag
	}
	periods := parseHGSSEncounters(b)
	if len(periods) != 3 || periods[0].Name != "Morning" || periods[2].Name != "Night" {
		t.Fatalf("periods = %+v", periods)
	}
	night := periods[2].Methods
	if night[0].Encounters[0].Species != 163 || night[0].Encounters[0].Chance != 80 || night[0].Encounters[1].Species != 165 {
		t.Errorf("night grass = %+v", night[0])
	}
	if night[1].Method != "Surfing" || night[1].Encounters[0].Chance != 100 || night[1].Encounters[0].MaxLevel != 20 {
		t.Errorf("surfing = %+v", night[1])
	}
	if !periods[2].Contains(23) || !periods[2].Contains(3) || periods[2].Contains(4) {
		t.Error("night should run 20:00 to 03:59")
	}
}

func TestFindLocation(t *testing.T) {
	g := make([]byte, 0x2000)
	put := func(off int, vals ...int32) {
		for i, v := range vals {
			le.PutUint32(g[off+i*4:], uint32(v))
		}
	}
	put(0x400, 127, 0, 4, 8, 1)      // a warp destination, not where the player is
	put(0x1424, 34, -1, 556, 342, 0) // the player
	if got := FindLocation(g); got != 0x1424 {
		t.Errorf("found %#x, want 0x1424", got)
	}
}

// Moves and where-to-find from every DS ROM in TALLGRASS_DS: the ARM9
// decompresses, the TM list starts with Focus Punch, Cyndaquil's egg
// moves start with Fury Swipes, Dratini is in Dragon's Den
// (HeartGold/SoulSilver) and Starly on Route 201 (Platinum).
func TestROMMoves(t *testing.T) {
	dir := os.Getenv("TALLGRASS_DS")
	if dir == "" {
		t.Skip("set TALLGRASS_DS to a folder of DS ROMs")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".nds") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			d, err := LoadData(dir + "/" + e.Name())
			if err != nil {
				t.Skip(err)
			}
			info, err := d.SpeciesInfo(155)
			if err != nil {
				t.Fatal(err)
			}
			if x, _ := d.extras(); len(x.tms) == 0 || Name(d.Moves, x.tms[0]) != "Focus Punch" {
				t.Errorf("TM01 isn't Focus Punch")
			}
			if len(info.TMMoves) == 0 || len(info.EggMoves) == 0 || info.EggMoves[0] != "Fury Swipes" {
				t.Errorf("Cyndaquil: TMs %v, egg moves %v", info.TMMoves, info.EggMoves)
			}
			if d.Layout.MapHeaders == 0 {
				return
			}
			found, err := d.FoundIn()
			if err != nil {
				t.Fatal(err)
			}
			if mew, _ := d.SpeciesInfo(151); len(mew.TutorMoves) < 30 {
				t.Errorf("Mew has %d tutor moves", len(mew.TutorMoves))
			}
			maps, err := d.Maps()
			if err != nil {
				t.Fatal(err)
			}
			trainers, _ := d.Trainers()
			leader, items := map[int]bool{}, 0
			for _, tr := range trainers {
				if tr.Class == "Leader" {
					leader[tr.ID] = true
				}
			}
			placedLeaders := 0
			for _, m := range maps {
				items += len(m.Items)
				for _, id := range m.Trainers {
					if leader[id] {
						placedLeaders++
					}
				}
			}
			if placedLeaders < 8 || items < 300 {
				t.Errorf("%d gym leader battles placed, %d items on maps", placedLeaders, items)
			}
			if d.Layout.Family == "Platinum" {
				if len(found[396]) == 0 || !slices.ContainsFunc(found[396], func(f gen3.Found) bool { return f.Area == "Route 201" }) {
					t.Errorf("Starly found in %v", found[396])
				}
			} else if len(found[147]) == 0 || found[147][0].Area != "Dragon's Den" {
				t.Errorf("Dratini found in %v", found[147])
			}
		})
	}
}

func TestFindPokedex(t *testing.T) {
	g := make([]byte, 0x2000)
	le.PutUint32(g[0x14A8:], dexMagic)
	caught, seen := 0x14AC, 0x14AC+0x3FC
	set := func(at, species int) { g[at+(species-1)/8] |= 1 << ((species - 1) % 8) }
	set(caught, 155)
	for _, sp := range []int{16, 155, 161, 714} {
		set(seen, sp)
	}
	for i := 0; i < 0x3C; i++ {
		g[caught+0x104+i] = 0xFF // filler between the arrays
	}
	dex, ok := FindPokedex(g, 1393)
	if !ok || dex.Caught != caught || dex.Seen != seen {
		t.Fatalf("got %+v %v, want caught %#x seen %#x", dex, ok, caught, seen)
	}
	if !Has(g[dex.Seen:dex.Seen+dex.Size], 714) || Has(g[dex.Caught:dex.Caught+dex.Size], 714) {
		t.Error("Scatterbug should be seen, not caught")
	}
}
