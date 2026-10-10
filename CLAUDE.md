# Tallgrass

A companion app for Cam's RetroPie (Raspberry Pi 4, 4GB), used only for Pokémon games and romhacks. One Go service with a web dashboard, combining five tools:

1. **Live party dashboard**: species, level, HP, nature, IVs/EVs and moves, read from game RAM
2. **Route encounter tracker**: what's catchable on the current map, and what's already been caught
3. **Automatic Nuzlocke tracker**: logs catches, faints and new routes from RAM events
4. **Save backup and viewer**: watches `.srm`/`.sav` files, versions every change, shows boxes and Pokédex
5. **Romhack library manager**: applies `.ips`/`.bps`/`.ups` patches and generates EmulationStation `gamelist.xml`

## Architecture

```
RetroArch (UDP 55355) ──► Memory client ──► Decoders (Gen 1/2/3)
                                              │
Save files (.srm) ──► Save watcher ───────────┤
                                              ▼
                              Event engine (catch / faint / map change)
                                              │
                         SQLite ◄─────────────┤
                                              ▼
                               Web dashboard (phone / laptop)
ROMs + patches ──► Romhack manager ──► gamelist.xml
```

- The final app runs **on the Pi** (cross-compile with `GOOS=linux GOARCH=arm GOARM=7`), so it can reach saves and ROMs directly. It must stay lightweight so games don't stutter.
- During development, run on the Mac and talk to the Pi over the LAN.
- Per-game memory addresses live in **game profile config files**, not code, so new romhacks are a config change.
- The Gen 3 Pokémon struct is identical in RAM and save files, so one decoder serves both.

## Build phases

| Phase | Work | Status |
|---|---|---|
| 0 | Prove RetroArch answers memory reads | ✅ Done |
| 1 | Go UDP memory client (`readMemory`) | ✅ Done |
| 2 | Gen 3 party decoder, FireRed/Radical Red first | ✅ Done |
| 3 | Web dashboard (tool 1) | ✅ Done |
| 4 | Map ID and encounter tables (tool 2) | ✅ Done |
| 5 | Event engine and SQLite (tool 3) | ✅ Done |
| 6 | Save watcher and versioned backups (tool 4, part 1) | ✅ Done |
| 7 | Save parser and box viewer, reusing the Phase 2 decoder (tool 4) | ✅ Done (boxes 1–19 of 25) |
| 8 | Patcher and `gamelist.xml` generator (tool 5) | Moved to ROADMAP M7 |

**What's next lives in [ROADMAP.md](ROADMAP.md):** the plan to release this publicly as an ecosystem (agent, website and docs, accounts, profile registry, OpenHome-based bank). Read its principles before adding features; above all, never ship game data, only read it from the user's ROM.

## Environment

- **Pi:** `pi@192.168.0.227`, Raspbian 10 (Buster), RetroPie, RetroArch **1.10.0**
- **GBA core:** `lr-mgba` (default in `/opt/retropie/configs/gba/emulators.cfg`). It supports memory maps, so `READ_CORE_MEMORY` works.
- **Network commands:** `network_cmd_enable = "true"` in `/opt/retropie/configs/all/retroarch.cfg`. RetroArch listens on `0.0.0.0:55355` (UDP, unauthenticated, LAN-reachable) only while a game is running.
- **Dev machine:** MacBook Pro M4, project at `~/Projects/pokecompanion` (folder keeps its old name), Go module `tallgrass`. Repo: `git@github.com:cambboyle/Tallgrass.git`.
- **DS:** DraStic (standalone, not RetroArch, so no network commands), via `/opt/retropie/emulators/drastic/drastic.sh`. Saves are `.dsv` files in `/opt/retropie/configs/nds/drastic/backup/` (not next to the ROMs). DS ROMs: Platinum and HeartGold (Gen 4) and hacks of each. No Yama module, so same-user processes can read each other's memory (`/proc/<pid>/mem`); that's the route to live DS data.
- **Current test game:** Radical Red (FireRed-based CFRU romhack); a level 6 Cyndaquil is in party slot 1.

## RetroArch protocol notes

- Request: `READ_CORE_MEMORY <addr hex> <length>`, for example `READ_CORE_MEMORY 02024284 100`
- Reply: `READ_CORE_MEMORY <addr without leading zeros, lowercase hex> <hex bytes separated by spaces>`
- Reply on failure: the data section is `-1` followed by an error message.
- **Unknown or misspelt commands get no reply at all.** A timeout can mean a bad command, no game running, or a network issue; it can't tell you which.
- `VERSION` is a cheap health check.
- ROM is readable too, mapped at `0x08000000` (with `lr-mgba`).
- `ReadMemory` splits reads into 8 KB chunks (`retroarch.MaxChunk`): replies are text in one UDP datagram (max 64 KB). The client sizes its UDP buffer from the requested length. `main.go` serialises reads with a mutex, since the poller and web requests share the socket.
- RetroArch answers one network command per frame (~16 ms), so many small reads add up: the first poll after a restart takes seconds. Don't test sizes with `nc`, which truncates at 1024 characters.

## Gen 3 party data (FireRed US / Radical Red)

- First party slot is at `0x02024284`, 100 bytes per Pokémon, 6 slots back to back.
- All multi-byte values are little-endian; use `encoding/binary.LittleEndian`.
- Text uses the Gen 3 character set: `A`=`0xBB`, `a`=`0xD5`, `0xFF` ends a string.

| Offset | Size | Field |
|---|---|---|
| 0 | 4 | Personality value |
| 4 | 4 | Original trainer ID |
| 8 | 10 | Nickname |
| 18 | 1 | Language |
| 19 | 1 | Flags |
| 20 | 7 | Original trainer name |
| 27 | 1 | Markings |
| 28 | 2 | Checksum |
| 30 | 2 | Unused |
| 32 | 48 | Data block: growth, attacks, EVs, misc (4 × 12 bytes) |
| 80 | 4 | Status |
| 84 | 1 | Level |
| 85 | 1 | Mail ID |
| 86 | 2 | Current HP |
| 88 | 2 | Max HP |
| 90 | 2 each | Attack, Defense, Speed, Sp. Atk, Sp. Def |

**Radical Red difference (observed, verify further):** the 48-byte data block appears to be **unencrypted and unshuffled**. The checksum is `00 00`, the growth block comes first (species `0x009B` = 155, Cyndaquil), and the attacks block follows (moves 33 Tackle and 43 Leer, PP 35/30). In **vanilla** Gen 3, the block is XOR-encrypted with `personality ^ OT ID` and its four sub-blocks are shuffled into one of 24 orders by `personality % 24`. The decoder needs a per-profile flag for this.

Romhacks may move addresses or expand structures, so keep them in game profiles.

## Name tables (species, moves, items)

Names are read from the running game's ROM, not bundled, because romhacks renumber everything. The Game Freak ROM header at `0x08000100` holds pointers: species names at `0x08000144` (11-byte entries), move names at `0x08000148` (13-byte), items at `0x080001C8` (44-byte structs, name in the first 14 bytes). A profile can override any of these (`speciesNames`, `moveNames`, `itemNames`).

**Radical Red:** the species and item header pointers are correct (`0x094042CC`, `0x093C0000`), but the move-name pointer is stale. The real table is at `0x090F3188` (found by searching the ROM for move names and for pointers to them; 32 code references). Move IDs are CFRU's own numbering, e.g. 464 = Moonblast.

## Location, encounters and Pokédex (Radical Red)

Verified live: map ID, map name, clock, dex flags. Verified from the ROM: encounter tables and the day/night logic (not yet confirmed against real in-game encounters).

Profiles use `Ref` addresses: `"0x020258E0"` is fixed, `"[0x03005008]+0x4"` means "read the pointer at 0x03005008, add 4". Pointers stored in vanilla FireRed code (e.g. `[0x08082990]`) work for vanilla and for hacks that repointed the data.

- **Current map:** SaveBlock1 pointer at `0x03005008`; map group and number at +4/+5 (Oak's Lab = 4.3, Route 1 = 3.19). Region map section at `0x02036E10` (`gMapHeader` + 0x14); names via the pointer table at `[0x080C0C94]` (= `0x083F1CAC`), first section `0x58` (Pallet Town).
- **Wild tables:** 20-byte headers (group, num, pad, then pointers to land/water/rock smash/fishing info), ended by group `0xFF`. Info = rate byte + pointer to slots of (min lvl, max lvl, u16 species). Slot chances: land 20,20,10,10,10,10,5,5,4,4,1,1; water and rock smash 60,30,5,4,1; fishing old 70,30 / good 60,20,20 / super 40,40,15,4,1.
- **Radical Red time of day:** main table `[0x08082990]` (= `0x0872C984`, 142 maps), plus Day `0x09166AB8` (hours 4–16) and Evening & night `0x09166428` (hours 17–3), 83 maps each. Found by disassembling the CFRU selector at `0x090C3550`: night = hour ≥ 20 or < 4, evening = 17–19, both share a table. A period table's empty method falls back to the main table. RTC hour byte: `0x03005EA6` (clock struct at `0x03005EA0`, byte 6). A script can override encounters via a ROM pointer at `0x0203C758`; not handled.
- **Pokédex:** Radical Red patches `GetSetPokedexFlag` (`0x08104AB0`) to fixed RAM: seen `0x0202583C`, caught `0x020258E0`, 164 bytes each, bit = national number − 1. Species → national: u16 table at `[0x080432B0]` (= `0x098218F0`). Regional forms have their own species IDs (Alolan Rattata = 1020) but map to the base national number.
- **Vanilla FireRed profile:** same code-pointer refs, dex flags at `[0x0300500C]+0x28` / `+0x5C`; **not verified**, as there's no vanilla FireRed ROM on the Pi.

## Unbound time of day (verified from its code, 2026-10-10)

- Hour byte `0x03005EA6` (clock struct `0x03005EA0` + 6, as in Radical Red); Unbound's code loads it directly 47 times.
- The wild table selector (`0x08A155B0`) tries, in order: night (hour ≥ 20 or < 5, test at `0x089D9120`) → `0x08A7DA74`; morning (5–7, `0x089D9588`) → `0x08A7DC18`; evening (17–19, `0x089D95A0`) → `0x08A7DC04`; else the main table. The evening table is empty (it starts with the `0xFF` end marker), so evenings use the day table: the profile's Morning 5–8 / Day 8–20 / Night 20–5 is exact.
- `cmd/ramdump` (cross-compile, copy to the Pi, run there) saves EWRAM/IWRAM through RetroArch, or `ramdump ds` DraStic's main RAM, for finding where data lives.

## Nuzlocke tracking (Phase 5)

- **Runs:** keyed by game + trainer ID + player name, so each save file gets its own log. Trainer name at `[0x0300500C]` (SaveBlock2), trainer ID at `+0xA` (matches party OT IDs). Nothing is recorded until a save is loaded (trainer ID 0 / empty name = title screen).
- **Nuzlocke mode** is a per-game toggle, off by default; events are recorded either way, and the mode only changes the view (Dead badges, "Routes" vs "Catches by area").
- **Events:** `tracking_started`, `new_area` (first visit to a map section), `obtained` / `egg` (new personality in the party, seen in two consecutive polls), `hatched`, `evolved`, `level_up`, `fainted` (HP to 0; the first faint is the Nuzlocke death, `mons.fainted_at`), `caught_pc`.
- **PC catches:** Gen 3 sets the caught flag at "Gotcha!" but adds the Pokémon to the party only after the dex entry and nickname prompt. So a new flag waits up to 2 minutes for that species to join the party; if it doesn't, it's logged as `caught_pc`, dated and placed at the flag time. Once the PC can be read (live, every 10 s), PC catches come from the boxes instead: new Pokémon appearing there are logged as `caught_pc` with their real name, level and met area, and the dex-flag guess switches off. The first PC read of a session records existing box Pokémon quietly.
- **When the service is down:** party changes are picked up on restart (e.g. a faint shows if HP is still 0), but level-by-level history in between is lost.
- **Met location:** misc block byte 1 (region map section); met level is origins bits 0–6. Radical Red's starter is met at `0x9D` "Oak's Lab".
- **Text:** Radical Red's map names use `0xB3` (left quote) as an apostrophe; both `0xB3` and `0xB4` decode to `'`.
- The Pi's DB is `/home/pi/tallgrass/tallgrass.db`; there's no `sqlite3` CLI on the Pi (don't install packages there), so fix data with a small cross-compiled Go program.

## Sprites

Also from ROM, via header pointers: front pics `0x08000128`, normal palettes `0x08000130`, shiny palettes `0x08000134` (back pics `0x0800012C`). Each table has an 8-byte entry per species whose first 4 bytes point at LZ77 data (GBA BIOS type `0x10`). A front sprite decompresses to 2048 bytes (64×64, 4bpp, 8×8 tiles, low nibble first), a palette to 32 bytes (16 BGR555 colours, index 0 transparent). Species 412 is the egg sprite in vanilla and CFRU. Some Radical Red palettes decompress to 128 bytes (several packed); the decoder reads the LZ77 size first and uses the first 32. Radical Red's header sprite pointers are correct, including Gen 4+ species. The Radical Red ROM on the Pi is `1636 - Pokemon Fire Red (U)(Squirrels) (patched).gba`.

## Current code

- `retroarch/`: `ParseReply` (hex text reply → bytes, surfaces `-1` errors) and `ReadMemory(conn, address, length)` (1-second read deadline, length check, skips stale replies for other addresses)
- `gen3/`: `FrontSprite` decodes sprites from ROM (`lz77.go` decompresses). `Names` looks up species/move/item names from ROM on demand and caches them (reset when the game disconnects). `Decode(b, encrypted)` turns a 100-byte struct into a `Pokemon` (returns `nil, nil` for an empty slot); handles vanilla XOR + 24-order shuffle, or the plain CFRU layout. `DecodeText` handles the Gen 3 charset. Tests use real Radical Red bytes.
- `profile/` + `profiles/*.json`: per-game config (`partyAddress`, `partySlots`, `pokemonSize`, `encryptedData`, optional name-table overrides; addresses are hex strings parsed by `profile.Addr`). The JSON files are embedded in the binary: `-profile radical-red` picks a bundled one, a path ending in `.json` loads from disk.
- `dashboard/`: one poller goroutine reads the party every `-interval`, adds names and a sprite URL (`Member` embeds `gen3.Pokemon`) and stores it as JSON; sprites are rendered to PNG once and served from `GET /api/sprite/{gen}/{key}`; browsers get it from `GET /api/party` or live over SSE at `GET /api/events` (pushed only when something changes). The UI is `dashboard/static/index.html` (vanilla JS, embedded with `go:embed`, no build step).
- `dashboard/location.go`: current map, encounters per time period with Pokédex seen/caught marks (`State.Location`).
- `tracker/`: the Nuzlocke event engine and SQLite store (`modernc.org/sqlite`, pure Go, no cgo). `Engine.Observe` diffs each poll against the last; `View` is what the dashboard shows (`State.Run`). Schema migrations are a list in `store.go`, tracked with `PRAGMA user_version`; append, never edit an applied one.
- `dashboard/tracking.go`: builds a `tracker.Observation` from each snapshot (trainer, party with met areas and national numbers, current section, dex caught flags).
- `saves/`: `Watcher` polls `<roms>/{gb,gbc,gba}` every 10 s for `.srm`/`.sav` files and backs up each distinct version (stable for two scans first); `Backups` stores them as plain files `backups/<system>/<save>/<UTC time>-<sha256 prefix><ext>.gz` (no DB, restorable by hand with `gunzip`). `Restore` backs up the current file first and refuses while a game is running.
- `gen3/save.go`: `ParseSave` picks the newest valid slot and reassembles SaveBlock2/SaveBlock1/PC storage; `Party`, `Box`, `BoxName`, `Trainer`. `gen3/levels.go`: level from experience (boxed Pokémon don't store it). `gen3/romfile.go`: `ROMFile.Read` is a `ReadMemory` over a ROM file on disk, so names/sprites/tables work with no game running.
- `dashboard/saveview.go` + `static/box.html`: the **Boxes tab** (`box.html`, no params) uses `GET /api/boxes`, which reads SaveBlock1/2 and PC storage **live from RAM** (`saveFile.ram` refs; same layout as the save file) and refreshes every 15 s; with `?file=&version=` the same page views a save file or backup via `GET /api/saves/view`. `GET /api/saves/sprite?rom=&species=&shiny=` serves sprites. Names/sprites/levels always come from the ROM file on disk; the live game's ROM is found by hashing ROMs against the profile's `romSha1`. The ROM next to a save (same name, `.gba`/`.gbc`/`.gb`) is SHA-1 hashed once to pick the profile (`romSha1` in profiles; `profile.All()` loads every bundled one).
- `dashboard/saves.go` + `static/saves.html`: `GET /api/saves`, `GET /api/saves/download?file=&version=`, `POST /api/saves/restore` (JSON only, as a cross-site form guard). Shared CSS is in `static/style.css`.
- `emulator/`: `Detect("/proc")` finds the running game from RetroArch/DraStic command lines (`retroarch -L <core> --config <cfg> <ROM>`, `drastic <ROM>`). The dashboard poller calls it every poll, hashes a new ROM (cached, also on disk in `rom-hashes.json`), and switches profile via `romSha1`; unsupported or DraStic games get `State.Running` with `live: false` and no RetroArch reads. Profile, running game and PC-read time are poller-owned; handlers read `prof`/`running` via `s.game()` under `s.mu`.
- `dashboard/library.go` + `static/library.html`: `GET /api/library` lists ROMs by system with profile status and saves; unknown ROMs are hashed one at a time in the background.
- Settings: `tracker.Store.Setting/SetSetting` (per game). `POST /api/settings/nuzlocke {"enabled": bool}` is applied by the poller (the engine's only user) on its next poll.
- `gen4/`: Gen 4 (DS). `nds.go` reads a DS ROM's file system, NARC archives and encrypted message files; `game.go` has per-family layouts (`LayoutFor(gameCode)`: `CPU*` Platinum, `IPK*`/`IPG*` HeartGold/SoulSilver) and `LoadData` (names found by content, growth rates, experience tables); `save.go` finds save blocks by their footers; `pk4.go` decodes 136/236-byte Pokémon into `gen3.Pokemon`; `live.go` finds and reads the party in live DS RAM. DS games need no profile.
- `emulator/procmem.go` + `dashboard/dslive.go`: following a DraStic game live by reading its process memory.
- `autoprofile/`: `ForFireRed` generates a profile for FireRed-engine ROMs without one (header pointers, FireRed code literal pools, decoded `ldr` instructions in GetSetPokedexFlag, CFRU detected by the `0x0203C758` literal). Used by `profileFor(sum, path)`; results cached per SHA-1. Profiles carry `verified` (`rom`, `save`, `live`, or `auto`).
- Gen 3 saves accept both known signatures (`0x08012025`, Unbound's `0x01121999`) unless a profile sets one; Emerald's party is at SaveBlock1 + `0x238` (`saveFile.partyOffset`); Emerald map names are 8-byte region map entries (`location.mapNameStride/mapNameOffset`). All-caps name lists are title-cased (`gen3.TitleCase`).
- `main.go`: `go run .` serves the dashboard on `:8080` (flags: `-host`, `-profile` (fallback where the running game can't be detected), `-listen`, `-interval`, `-drastic-saves` (default `/opt/retropie/configs/nds/drastic/backup`), `-db` for the SQLite file, default `tallgrass.db`, empty disables tracking; `-roms` default `/home/pi/RetroPie/roms`, save backups are off if it doesn't exist; `-backups` default `backups`); `go run . -print` prints the party once to the terminal

## Testing

- `go test ./...` runs everything that doesn't need a ROM.
- ROM-backed tests (Route 1 day/night, map names, an end-to-end dashboard test with fake RAM) skip unless `TALLGRASS_ROM` points at the Radical Red ROM; the real-save test also needs `TALLGRASS_SAVE` (a Radical Red `.srm`). Copy it locally (`scp` from the Pi path above, outside the repo) and run `TALLGRASS_ROM=/path/to/rr.gba go test ./...`.
- For poking at ROM code, Python `capstone` (Thumb mode) works well; nothing in the repo depends on it.

## Deploying to the Pi

- `make deploy` runs tests, cross-compiles to `build/`, copies the binary to `/home/pi/tallgrass/` and restarts the service.
- `make install-service` (one-time) installs `deploy/tallgrass.service`: runs as `pi`, reads RetroArch on `127.0.0.1:55355`, serves `:8080`, `Nice=10` so games keep priority.
- `make logs` / `make status` for journalctl and systemctl. Dashboard: `http://192.168.0.227:8080`.
- SSH uses key auth (`~/.ssh/id_ed25519`).

## Trainers and export (M3)

- **Gen 3 trainers:** `gTrainers` (40 bytes: flags, class, music, pic, name[12], items u16×4, double u32, AI u32, party size, party pointer) via `[0x08044028]` (FireRed engine) / `[0x0803587C]` (Emerald); class names 13 bytes via `[0x080D80A0]` / `[0x080183B4]`. Party entries are 8 bytes (iv, level, species, item or pad) or 16 with custom moves. Text glyphs 0x53/0x54 spell "PKMN" (shown as "Pokémon").
- **Trainer placement:** map groups' header lists sit back to back (a group ends where the next starts); each person's script is scanned for `trainerbattle` (0x5C, type ≤ 9, trainer u16, local u16, ROM pointer). Unbound's object events use a different layout, so nothing is placed there yet.
- **DS trainers:** `trdata` 20 bytes (flags bit 0 moves, bit 1 items; class; count at byte 3; battle type 2 = double) and `trpoke` (difficulty, level, species, [item], [4 moves], ball capsule). Trainer names are the message file just before the class list ("Pokémon Trainer, Pokémon Trainer, Youngster") and use 9-bit compressed text (first char 0xF100; 15 bits per word, ends at 0x1FF). Species keep a form in the top 6 bits unless the hack has more than 1,023 species.
- **Export:** `dashboard/export.go`; `-export <rom>` works anywhere (no emulator). Format documented in `docs/gamedata-export.md`; bump `ExportVersion` only when a field changes meaning.
- **Gen 3 maps:** `gen3.ReadMaps` reads headers (connections at +12, region map section at +0x14) and events (people 24 bytes, script at +16; warps 8 bytes; background events 12 bytes, kind 7 = hidden item, low 16 bits of the data = item). Item ball scripts start `1A 00 80 <item> 1A 01 80 <qty>`; shops are `0x86 <pointer to 0-terminated item list>`. Unbound's trainers use CFRU battle type 13.
- **DS maps:** zone events (u16 at header +0x10; format in `gen4/events.go`). Script IDs: 3000+ trainers (ID = script − 2999), 7000+ item balls (item-ball script file: `setvar 0x8008 item; setvar 0x8009 qty`, opcode 0x28 Platinum / 0x29 HGSS), 8000+ hidden items (ARM9 table, 8 bytes, item and quantity first). Map script file is the header u16 at +0x04 (Platinum) or +0x06 (HGSS); trainer battle command 0xE5 / 0xD5. Event coordinates are region-wide and a map's file can include a neighbour's objects; duplicates are kept only where the map's other objects surround them. Platinum headers: wild file u16 at +0x0E, music at +0x0A (HGSS +0x0C), location byte +0x12.
- **Evolution methods:** CFRU 16–28 are shared; later numbers differ per hack (`speciesData.evolutionScheme`). Megas list a no-item entry back to their base form (skipped).
- Headless Chrome can't go narrower than 500px: check phone layouts with 390px iframes in a page served over HTTP (file:// pages can't load them).

## Setup wizard

- `dashboard/setup.go` + `static/setup.html` (`GET /api/setup`, `POST /api/setup/game`, `POST /api/setup/fix`). The three focus games are listed in `setupGames` with their ROM SHA-1s. The choice is saved as setting `setup.game` (game "").
- Checks: ROM by SHA-1 in its system folder; GBA: `default = "lr-mgba"` in `configs/gba/emulators.cfg`, `network_cmd_enable` and `autosave_interval` (the GBA `retroarch.cfg` overrides `all/`); DS: DraStic installed, `/proc/sys/kernel/yama/ptrace_scope` absent or 0; the save (offers `.sav` → `.srm`, trimmed to 128 KB, old `.srm` kept as `.before-tallgrass-<time>`); live read (running ROM matches and the party was read).
- Config edits keep the original as `<file>.bak-tallgrass` (once) and are refused while a game runs. Flags `-retropie-configs`, `-drastic-dir`.
- Don't reuse the `.mark` class for new UI: route encounter tiles position it absolutely.

## Cloud (M4)

- **Repo:** `git@github.com:cambboyle/Tallgrass.git` (private). Commit as Cameron Boyle <cambboyle@gmail.com> (repo-local config); **never add Claude attribution** to commits or PRs.
- **Supabase project** `ifjlueqhuyaafjuzltoi` (London). Schema in `supabase/migrations/` (apply with the Supabase MCP `apply_migration`, then keep the file in the repo): `profiles` (auto-created on sign-up; `is_maintainer` set by hand), `devices` (SHA-256 of each device key), `pairing_codes` (server-only), `games` (the three focus games by ROM SHA-1), `game_data` (uploaded exports; files in the public `gamedata` bucket).
- **Edge Functions** (`supabase/functions/`, all `verify_jwt = false`, callers checked in code): `pair-start` → code + poll secret; `pair-claim` (user session) → creates the device; `pair-poll` → hands the device key over once; `device` (device key) → account info; `gamedata-upload` (device key, maintainers) → stores an export. The MCP deploy takes each function's files plus `_shared/util.ts`.
- **Agent:** `cloud/` client; dashboard Account page (`/api/cloud*`); device key in `cloud-link.json` (0600, next to the binary). Flags `-cloud`, `-site`, `-link-file`. The anon key is compiled in (public by design).
- **Website:** https://tallgrass-delta.vercel.app (Vercel, root `web/`; note `tallgrass.vercel.app` is someone else's site). `web/` (Next.js 16: `proxy.ts` not middleware, async `params`/`cookies()`, Cache Components turned off in favour of `revalidate` + empty `generateStaticParams` for ISR). Docs read the latest export from Storage (kept in memory 10 min); `GAMEDATA_DIR` serves local exports for previews. Read `web/node_modules/next/dist/docs/` before using a Next API you're unsure of.

## Journey/tracker gotcha

- The run view must be deterministic: Go map iteration is random, and Pokémon/areas recorded at the same moment tie on time. An unstable order made the SSE state "change" every poll, so the Journey section reshuffled every second (fixed with party-slot order and tie-breaks; `TestViewIsStable`).

## Save files

- RetroPie keeps saves next to the ROMs (`savefile_directory` unset), e.g. `~/RetroPie/roms/gba/<rom name>.srm`. Backups on the Pi: `/home/pi/tallgrass/backups/`.
- `autosave_interval = "60"` is set in `/opt/retropie/configs/all/retroarch.cfg` (line 957; changed 2026-10-09, original kept as `retroarch.cfg.bak-pokecompanion`). RetroArch's default (0) only writes the `.srm` when the game closes. It writes only when save data changed, and `config_save_on_exit = "false"`, so the edit sticks. Takes effect on the next game launch.
- Restoring while a game runs is refused: RetroArch would overwrite the file from memory on exit. "Game running" comes from the dashboard poller reaching RetroArch, so it's any game, not just the one being restored.

## Save file format (Radical Red)

- Standard Gen 3 frame: 32 × 4 KB sectors, two 14-sector slots written alternately; footer at `0xFF4` = section ID (u16), checksum (u16), signature `0x08012025` (u32), save counter (u32). Section 0 = SaveBlock2, 1–4 = SaveBlock1, 5–13 = PC storage. Newest slot = higher counter.
- **CFRU packs `0xFF0` bytes per sector** (vanilla `0xF80`), using the space up to the footer. Verified against RAM: sector 4 = SaveBlock1 + 3×`0xFF0`; box names land at vanilla offsets only with `0xFF0`.
- **Checksums don't verify** for sections 1, 4 and 13 (CFRU writes extra data outside the checksummed range, with cut-offs that differ from vanilla), so the profile sets `verifyChecksums: false` and slots are picked by signature and counter only.
- Party: SaveBlock1 + `0x34` (count), `+0x38` (100 bytes each). Dex: same offsets as RAM (SaveBlock1 + `0x310` seen, `+0x3B4` caught). Trainer: SaveBlock2 + 0 (name), `+0xA` (ID; the in-game ID is the low 16 bits).
- **25 boxes, CFRU compressed format (verified live with a deposited Wooloo).** Box Pokémon are **58 bytes**, unencrypted: the usual 28-byte header (personality, OT ID, nickname, OT name...), then `0x1C` species, `0x1E` item, `0x20` exp, `0x24` PP bonuses, `0x25` friendship, `0x26` ball, `0x27` four 10-bit move IDs (5 bytes), `0x2C` EVs, `0x32` Pokérus, `0x33` met location, `0x34` origins, `0x36` IV word. No PP, contest stats or ribbons. See `gen3/compressed.go`. Vanilla boxes are 80-byte encrypted records.
- Boxes 1–19 run back to back from storage + 4 (30 × 58 = 1,740 bytes each). **Boxes 20–25 live elsewhere** (verified 2026-10-10 with Pokémon in boxes 20, 21 and 25): CFRU's GetBoxedMonPtr (`0x0808BA18` → a table of 25 RAM addresses, `0x09148930` in Radical Red, `0x08A6CB2C` in Unbound, identical contents) puts boxes 20–22 at RAM `0x0203CB44`, 23–24 in SaveBlock1 + `0x1F08`, 25 in SaveBlock2 + `0xB0`. In the save, 23–25 sit at the same offsets in their blocks; 20–22 are in **sectors 30–31** (outside both slots, 0xFF0 bytes each, a straight copy of RAM from `0x0203C038`), starting at sector 30 + `0xB0C`. Profiles describe this as `saveFile.extraBoxes` / `extraSectors` / `ram.extra`.
- Box names are 9 bytes: Box1–14 at storage + `0x8344` (vanilla), Box15–25 *before* it in reverse (Box15 at `0x833B`, Box25 at `0x82E1`); `Save.BoxName` handles both.
- Levels: growth rate is byte 19 of the 28-byte base stats (`[0x080001BC]`); experience table at `[0x0802FFB8]` (`0x0915514C`) has **256 levels per row** (vanilla: 101, table at `0x08253AE4`).
- Radical Red ROM SHA-1: `964f951a0fdaf209e4ea1344883ef0d557bb3a80`.

## Gen 4 (DS) notes

- **Saves:** `.dsv` = 512 KB raw save + 122-byte DeSmuME footer. Two copies (0, 0x40000) of a general block and a storage block, each ending in a footer: ... counter (u32), size (u32), magic `0x20060623`, block ID (u16: 0 general, 1 storage), CRC16-CCITT (u16). Footers are 0x14 bytes (Platinum) or 0x10 (HeartGold/SoulSilver). Found by scanning for the magic, so resized blocks in hacks just work.
- **Platinum:** general `0xCF2C` (trainer name +0x68, IDs +0x78, party count +0x9C, party +0xA0); storage at `0xCF2C`, 18 boxes from +4 (30 × 136 bytes = `0xFF0` each), names ("BOX 1") right after at `0x11EE4`, 0x28 bytes each.
- **HeartGold/SoulSilver:** general `0xF628` (trainer +0x64, IDs +0x74, party count +0x94, party +0x98); storage at `0xF700`, boxes `0x1000` apart (padded), names 8 bytes after the last box. **HeartGold Generations** (hack): general `0xFFA0`, storage at `0x10000` (`0x1E4FC`), **30 boxes**; box count is derived from the storage block's size.
- **Pokémon:** personality (u32), flags (u16), checksum (u16), then 4 × 32-byte blocks shuffled by `((pid & 0x3E000) >> 13) % 24` and XORed with the PRNG `x*0x41C64E6D + 0x6073` seeded by the checksum; the checksum is the 16-bit sum of the decrypted blocks. The party tail (level, HP, stats) uses the same PRNG seeded by the personality. Species IDs are national numbers (hacks extend past 493). Text is 16-bit: `A`=`0x012B`, `a`=`0x0145`, `0`=`0x0121`, space `0x01DE`, end `0xFFFF`.
- **ROM data:** message archive `/msgdata/pl_msg.narc` (Platinum) or `/a/0/2/7` (HeartGold/SoulSilver); message files are encrypted per entry. Names are picked by content (entry 1 = "Bulbasaur"/"Pound"/"Master Ball"/"Stench"; locations = the list with "Twinleaf Town"/"New Bark Town"), preferring mixed case; all-caps lists are title-cased. Base stats: `pl_personal.narc` / `/a/0/0/2` (44 bytes, growth rate at 0x13); experience: `pl_growtbl.narc` / `/a/0/0/3` (8 × 101 u32). In HeartGold Generations, abilities differ from vanilla (Cyndaquil: Dry Skin/Blaze), which is correct for the hack.
- **Sprites:** sprite archive `/poketool/pokegra/pl_pokegra.narc` (Platinum) or `/a/0/0/4` (HeartGold/SoulSilver), 6 files per species: back F/M, front F/M (NCGR, empty if no gender difference), normal and shiny palettes (NCLR). Images are 160×80 (two 80×80 frames), 4bpp, line by line, after a 0x30-byte header; decrypt forward with seed = first u16, `seed = seed*0x4E6D + 0x6073` (16-bit). Palette colours at TTLP + 0x18. `ROM.ArchiveFile` reads single NARC members, so the ~50 MB archive is never loaded. Gender is block B byte 0x40, bit 1.
- **ARM9 and overlays:** `ROM.ARM9()` returns the main binary as it sits in RAM (base `0x02000000`); HeartGold/SoulSilver compress it with BLZ (backwards LZ; the compressed end is the word just before the `nitrocode` magic), HeartGold Generations ships it uncompressed. `ROM.Overlays()` decompresses overlays. The TM list (92 TMs + 8 HMs) is found in ARM9 by its HMs (Cut, Fly, Surf, Strength); species TM bits are base stats bytes 0x1C–0x2B; egg moves are `20000+species` then moves (`/a/2/2/9` in HGSS, overlay 5 in Platinum).
- **Map headers offline:** the same table as in RAM (`0x020F6BE0`, HGSS), read from the decompressed ARM9; real headers have music IDs ≥ 1000 at 0x0C/0x0E, which marks the end of the table (540 maps in vanilla).
- **Location (live):** the general block holds five 20-byte location records back to back (map, warp, x, z, facing; `+0x1424` in HeartGold Generations). Only the first is the current position and updates every step; the others are door/respawn points that change only at warps (reading one of those made the Route section lag until you entered a building). `gen4.FindLocation` finds the group of five. Map headers in RAM give the wild bank and location name.
- **Live PC (DS):** the game keeps the whole save image in RAM with the save file's layout, so the PC is at the live general block + (storage start − general start in the save file); HeartGold Generations: general `0x0227C2A8`, storage `0x0228C2A8` (+0x10000). `liveDSView` reads both, checks the general block and box 1's name against the latest save, and falls back to the save file if they don't match. Gen 4 wild Pokémon carry the player's trainer ID, so a battle's wild Pokémon in RAM looks like one of yours.
- **Pokédex (DS):** in the general block after the magic `0xBEEFCAFE`: caught flags, then seen flags `0x40` later (vanilla) or `0x3FC` later (HeartGold Generations, +0x14A8 in its general block); bit = species − 1. `gen4.FindPokedex` tries the known gaps, then searches (the seen array must contain every caught species and not be filler). Drives the Route section's caught/seen marks and the tracker's dex-flag PC catches.
- **Stale block at startup:** DraStic can hold an older copy of the save (the title screen's) that passes `CheckGeneral` at first and is reused later. The poller re-checks its block every 10 s and treats an empty party as a miss, then searches again.
- **DraStic startup:** DS RAM is mapped a moment after DraStic starts, so a game detected at launch is re-checked every poll until it's readable.
- **Live (DraStic):** DraStic maps DS main RAM (4 MB) at **`0x02000000` in its own process** (mirrors above). The general block's address varies per session; `gen4.FindGeneral` scans for it strictly (valid checksums, a real trainer name, a party OT ID matching the trainer). **The game decrypts party Pokémon in place in RAM** (several times a second, no flag set), and while holding them decrypted it edits fields and only fixes the checksum on re-encrypting; it can also be caught with some blocks decrypted and others not. So `DecodeLive` also accepts plain data with a stale checksum if the names are valid text and values are sane, and the dashboard keeps the last good party when a read fails (~10% of reads).

## Data changes made for Cam

- 2026-10-09: copied `PokemonUnbound.sav` (July 2025 save, from another emulator, with a 16-byte RTC trailer) over the blank `PokemonUnbound.srm`, so RetroArch loads it. The `.sav` is untouched; both `.srm` versions are in the backups.

## Mac gotchas

- On 2026-10-09 around 14:18, Go- and Python-built programs on the Mac started failing to send UDP to the Pi ("write: broken pipe" / "No route to host") while `nc` and ping still worked. Probably macOS Local Network privacy (System Settings → Privacy & Security → Local Network). Headless Chrome was hit too (it could no longer load the Pi's dashboard at ~15:05), while `curl` still worked. The deployed app on the Pi is unaffected.
- macOS caps outgoing UDP datagrams at 9216 bytes (`net.inet.udp.maxdgram`), which matters for fake RetroArch servers in tests (see `TestReadMemoryChunks`).

## Pi gotchas

- `/etc/apt/sources.list` was repaired to `deb http://legacy.raspbian.org/raspbian/ buster main contrib non-free rpi`, because Raspbian Buster moved to the legacy server. Stray Debian repos had broken RetroArch (`libavcodec.so.58` was missing).
- **Do not run `apt upgrade` on the Pi.** Buster is end-of-life; a reflash to a newer OS is planned eventually.

## Working with Cam

- Cam is a full-stack engineer (TypeScript/NestJS) who is new to Go.
- He's vibe-coding this project: build features directly and keep explanations brief. Explain Go concepts in more depth when he asks, and use JavaScript comparisons where they help.
- Test against the live Pi with a game running whenever a change touches the memory client or decoders.
