# Writing a game profile

A **profile** tells Tallgrass where a Game Boy Advance Pokémon game keeps its data: where the party is in memory, how a Pokémon is stored, where the maps, encounters, Pokédex, boxes and trainers are. Profiles are plain JSON files in [`profiles/`](../profiles). Supporting a new romhack is a matter of writing one; no code change, unless the hack invents a new data format.

This guide covers what's in a profile, how to try one out on your own device, and how the addresses were found for the games Tallgrass already supports, so you can do the same for another hack.

**DS games don't need a profile.** Tallgrass reads Gen 4 games (Platinum, HeartGold, SoulSilver and hacks of them) by their game family.

## Do you need one?

Open the dashboard's **Library** page. Each ROM shows one of:

- **A profile name.** It's supported.
- **auto-detected.** It's a FireRed-engine hack, and Tallgrass worked out a profile from the ROM by itself. The party usually works; some sections may be missing. A hand-written profile makes it complete.
- **not supported yet.** No profile. The note under it says why: "Built from the pokeemerald decomp…" means the hack changed the data layouts, which is the case where a profile takes the most work.

The Library also shows each ROM's **SHA-1**, the fingerprint a profile is matched by. Different versions of a hack have different fingerprints.

## Trying a profile on your device

1. Start from the closest existing profile and copy it:
   - FireRed hacks built with CFRU (Radical Red, Unbound): [`radical-red.json`](../profiles/radical-red.json).
   - Vanilla Emerald and lightly modified Emerald hacks: [`emerald.json`](../profiles/emerald.json).
   - Hacks built from the pokeemerald-expansion decomp: [`rowe.json`](../profiles/rowe.json), though expect to check every address.
2. Change `name`, and put the ROM's SHA-1 (from the Library page) in `romSha1`.
3. Save it in the `profiles` folder next to Tallgrass, e.g. `~/tallgrass/profiles/my-hack.json`, and restart Tallgrass (`sudo systemctl restart tallgrass`). Profiles there take priority over the built-in ones; one that doesn't load is skipped and the reason logged (`journalctl -u tallgrass -n 50`).
4. Start the game and open the Party page. Work section by section: get the party right first, then add the rest.

Only `name`, `generation`, `romSha1`, `partyAddress`, `partySlots` and `pokemonSize` are required. Every other section is optional: leave one out and that part of the dashboard is simply not shown.

## Addresses

Addresses are hex strings. There are two kinds:

- A **fixed address**: `"0x02024284"`.
- A **pointer**: `"[0x03005008]+0x4"` means "read the 4-byte pointer at `0x03005008`, then add 4". Use this for anything the game moves around. Emerald moves its save blocks every time you enter a map, so their pointers are the only stable way to reach them. Pointers stored in the ROM's code, like `"[0x08082990]"`, also survive hacks that moved a table without changing the code that uses it.

GBA memory: EWRAM `0x02000000`–`0x0203FFFF`, IWRAM `0x03000000`–`0x03007FFF`, the ROM from `0x08000000`.

## Tools

- **`ramdump`** (`cmd/ramdump` in this repo): saves EWRAM and IWRAM from the running game to files. Cross-compile it for the Pi (`GOOS=linux GOARCH=arm GOARM=7 go build ./cmd/ramdump`), copy it over, and run it while the game is open. Take dumps before and after doing something in the game (catching a Pokémon, walking onto a route) and compare them.
- **A disassembler for Thumb code.** Python's [capstone](https://www.capstone-engine.org/) works well. When the data isn't where you'd expect, the code that reads it says where it is.
- **A text encoder** for the Gen 3 character set, to search the ROM for names: `A`–`Z` are `0xBB`–`0xD4`, `a`–`z` are `0xD5`–`0xEE`, `0`–`9` are `0xA1`–`0xAA`, space is `0x00`, `0xFF` ends a string.

## Finding things: techniques that worked

These are the methods used for Pokémon R.O.W.E., a decomp hack where almost nothing was in its vanilla place. Each one starts from something you know about your own game.

**The save blocks.** Your player name is in SaveBlock2. Search IWRAM for three consecutive pointers into EWRAM where the second points at your name: those are SaveBlock1, SaveBlock2 and PC storage (R.O.W.E.: `0x03004B7C`). Re-read them in every dump, as they move.

**The party.** Your trainer ID (SaveBlock2 + `0xA`) is stored in every one of your Pokémon at offset 4. Search EWRAM for it: hits a fixed distance apart are the party, and that distance is the size of a party Pokémon (100 in vanilla, 76 in R.O.W.E.). The party count is the byte a little before the first one.

**The Pokémon format.** Decode a party slot and compare it with what the summary screen says. If species, level and moves come out right, the format is vanilla (set `encryptedData` to match). If not, find `GetMonData` in the ROM: its long `switch` over field numbers is compiled into a table of code addresses, and each case shows which bytes and bits a field comes from. That's how R.O.W.E.'s packed format (`pokemonFormat: "rowe"`) was mapped. New formats need code; open an issue with what you found.

**Name tables.** Search the ROM for two known names (move 1 "Pound" and move 2 "Karate Chop"). The distance between them is the entry size, and the first entry's address minus one entry is the table. The ROM header at `0x08000144` / `0x08000148` / `0x080001C8` points at the species, move and item tables in most games; set `speciesNames`, `moveNames` or `itemNames` only when a hack moved one without updating the header.

**The current map.** The map group and number are at SaveBlock1 + 4 and + 5. The game keeps a copy of the current map's 28-byte header in EWRAM. Find a run of ROM pointers in EWRAM that also appears in the ROM, and you have both the RAM copy (its byte `0x14` is the region map section, for the area name) and the header in ROM. Search the ROM for pointers to that header to find its map group's list, then for a pointer to that list to find `gMapGroups`. A code literal pointing at `gMapGroups` gives a stable `"[…]"` reference.

**Map names.** Search for a town name, then for pointers to it: the pointers sit in a table of region map entries (8 bytes each in Emerald, name pointer at +4).

**Wild encounters.** Every encounter slot is 4 bytes: minimum level, maximum level, species. Search the ROM for 12 consecutive slots with sane levels and species, follow the pointer to them (the "info": a rate byte and that pointer), then the pointer to the info (a map's header). The headers form a table ending in `0xFF`; the distance between them is `headerSize`. Look at which pointer leads to which kind of list (12 slots: grass; 5: water or rock smash; 10: fishing) to fill in `slots`.

**Time of day.** Take two dumps a few minutes apart and look for three bytes that read like an hour, minute and second and moved forward between them (R.O.W.E.: the real-time clock at `0x03004AD8`, hour at +2). Then find the game's "is it day" function to learn which hours count as day.

**The Pokédex.** Take a dump, catch a Pokémon you hadn't seen, take another. Look for an array where exactly that species' bit (national number − 1) turned on. Seen and caught arrays usually sit next to each other.

**PC boxes.** Put a Pokémon you can recognise in the first slot of box 1 and another in the last slot of the last box. Their positions give the box record size and whether boxes run back to back. Radical Red's last boxes turned out to live in three different places, found this way.

**The save file.** A save has 4 KB sectors with a footer, but games use different amounts of each sector (vanilla `0xF80`, CFRU `0xFF0`, R.O.W.E. `0xFF4`). Find a known Pokémon in the save file and compare its position with where it sits in PC storage in RAM; that gives `sectorDataSize`.

**Trainers.** Search for a gym leader's name. Her rematches sit one record apart, which gives the record size; a code literal pointing at (name address − trainer number × record size − name offset) is the table.

## Field reference

### Top level

| Field | Meaning |
|---|---|
| `name` | The game's name, as shown everywhere. |
| `generation` | `3`. |
| `romSha1` | SHA-1s of the ROMs this profile is for (the Library page shows them). |
| `verified` | What's been checked against the real game: `rom` (tables decode), `save` (a real save parses), `live` (RAM addresses while playing). Don't claim what you haven't checked. |
| `partyAddress`, `partySlots`, `pokemonSize` | The party: first Pokémon, number of slots (6), bytes per Pokémon. |
| `encryptedData` | `true` for vanilla's encrypted, shuffled data block; `false` for CFRU's plain one. |
| `pokemonFormat` | `""` (vanilla/CFRU) or `"rowe"` (R.O.W.E.'s packed format). |
| `formSpecies` | Table of per-species pointers to form lists, for hacks that store forms separately (R.O.W.E.). |
| `speciesNames`, `moveNames`, `itemNames` | Name tables, when the ROM header's pointers are wrong. |
| `speciesNameLength`, `moveNameLength`, `itemSize` | Entry sizes when they're not vanilla's 11, 13 and 44 bytes. |
| `flags`, `vars` | The game's flag and variable arrays, for profiles that read game options. |
| `trainer` | `name` and `id` of the player (SaveBlock2 + 0 and + `0xA`). |

### `location`

| Field | Meaning |
|---|---|
| `mapId` | Map group, then map number (SaveBlock1 + 4). |
| `mapSection` | The current map's region map section (the RAM copy of the map header + `0x14`). |
| `mapNames` | Table of entries pointing at area names. |
| `firstMapSection` | The section of the table's first entry (FireRed: `0x58`; Emerald: 0). |
| `mapNameStride`, `mapNameOffset` | Entry size and where the name pointer is in it (FireRed 4 and 0, Emerald 8 and 4). |
| `mapGroups` | `gMapGroups`, for items, shops, trainers and exits on each map. |

### `wildEncounters`

| Field | Meaning |
|---|---|
| `headers` | The encounter header table. |
| `headerSize` | Bytes per header (20 in vanilla). |
| `slots` | Which header pointer is land, water, rock smash, fishing, and optionally hidden (DexNav) and headbutt. Vanilla: `[0, 1, 2, 3]`. |
| `clockHour` | The hour byte of the game's clock, to mark the current period. |
| `periods` | Times of day: `name`, `fromHour`, `toHour` (exclusive, may wrap past midnight), and either `headers` (a separate table, as in Radical Red) or `landSlot` (a different land pointer in the same header, as in R.O.W.E.). |
| `swapFlag` | A flag that swaps day and night (R.O.W.E.'s bed). |

### `pokedex`

| Field | Meaning |
|---|---|
| `caught`, `seen` | The flag arrays: one bit per national number − 1. |
| `flagBytes` | Size of each array. |
| `speciesToNational` | u16 table, by species − 1, of national numbers (hacks add forms with their own species numbers). |

### `saveFile`

| Field | Meaning |
|---|---|
| `sectorDataSize` | Bytes of data per 4 KB sector. |
| `signature` | The sector footer signature, if not vanilla's `0x08012025` (Unbound: `0x01121999`). |
| `verifyChecksums` | Whether sector checksums can be trusted (CFRU hacks write past the checksummed range: `false`). |
| `partyOffset` | Party in SaveBlock1 (FireRed `0x38`, Emerald `0x238`). |
| `boxMonSize` | Bytes per boxed Pokémon: 80 vanilla, 58 CFRU, 52 R.O.W.E. |
| `boxes`, `totalBoxes` | Boxes stored back to back from PC storage + 4, and boxes the game has. |
| `boxNames`, `boxNamesInOrder` | Where box 1's name is in PC storage; whether every name follows in order (otherwise CFRU's arrangement). |
| `extraBoxes`, `extraSectors` | Boxes kept elsewhere (`first`, `last`, `block`: `block1`, `block2` or `extra`, `offset`) and the extra save sectors they use. |
| `dexSeen`, `dexCaught` | Where the Pokédex arrays are in the save (`block` 1 or 2, `offset`). |
| `ram` | Where the same blocks are while the game runs (`saveBlock1`, `saveBlock2`, `storage`, `extra`), for live boxes. |

### `experience`

For boxed Pokémon's levels (boxes store experience, not level).

| Field | Meaning |
|---|---|
| `baseStats` | Species data (base stats). |
| `table` | Experience totals, a row per growth rate. |
| `rowLength` | Levels per row (101 vanilla; Radical Red 256). |
| `baseStatsSize`, `growthOffset` | Species record size and the growth rate's offset, if not 28 and 19. |
| `sharedGrowthSpecies` | A species whose growth rate every species uses (R.O.W.E.: 10). |

### `speciesData`

For species pages and the website's game docs.

| Field | Meaning |
|---|---|
| `format` | `""` or `"rowe"` (64-byte species records with 16-bit stats and abilities). |
| `typeNames`, `typeNameLength` | Type names and their length (7 if unset). |
| `abilityNames`, `abilityNameLength`, `hiddenAbility` | Ability names, their length (13 vanilla, 17 CFRU), and whether species have a hidden ability. |
| `learnsets` | Per-species pointers to level-up moves. |
| `cfruLearnsets`, `learnsetEntry` | Entry format: packed u16 (vanilla), 3 bytes (CFRU), or `learnsetEntry: 4` (u16 move, u16 level). |
| `evolutions`, `evolutionsPerSpecies`, `evolutionScheme` | Evolution table, entries per species, and which numbering later methods use (`cfru`, `radical-red`, `unbound`, `rowe`). |
| `tmMoves`, `tmLearnsets`, `tmLearnsetSize` | TM/HM moves and per-species compatibility bitfields. |
| `tmByteLists`, `tmCount` | Instead: per-species lists of TM numbers ending `0xFF`, and the TM count (R.O.W.E.). |
| `tutorMoves`, `tutorLearnsets`, `tutorLearnsetSize`, `eggMoves` | Move tutors and egg moves, if the game has them. |
| `speciesCount` | One past the last species, if counting names doesn't work. |

### `trainers`

| Field | Meaning |
|---|---|
| `table`, `classNames`, `classNameLength` | The trainer table, trainer class names and their length (13). |
| `format` | `""` (40-byte records) or `"rowe"` (64-byte records, move pools, a second team). |

## Contributing a profile

Profiles are released into the public domain ([CC0](../profiles/LICENSE)), so anyone can use them. To add yours to Tallgrass:

1. Make sure every section you include is right, and set `verified` honestly.
2. Open a pull request adding the file to `profiles/`, with a note on how you checked it (which version of the hack, what you tested live).
3. **Never include game data:** no ROM bytes, sprites, text or patches. A profile only says where things are; Tallgrass reads them from each player's own ROM.
