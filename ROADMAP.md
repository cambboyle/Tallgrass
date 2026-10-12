# Roadmap

Tallgrass (formerly pokecompanion) grows from a tool for one Raspberry Pi into a public ecosystem for playing Pokémon games and romhacks on emulators. It has four parts:

- a local **agent** that runs next to the emulator
- a public **website** with docs and accounts
- a **bank** for moving Pokémon between games
- a shared **profile registry** so any game can be supported

Phases 0–7 of `CLAUDE.md` are the first version of the agent. This file is the plan from here. Status is tracked in the checkboxes.

## Focus (2026-10-09)

Launch supports three games, done properly: **Radical Red, HeartGold Generations and Unbound**. Everything else stays "experimental" (auto-detected profiles, vanilla games) and isn't advertised. Instead of preparing every game up front, a setup wizard prepares the one game the user picks (M6).

## Principles

1. **Never ship game data.** Names, sprites, stats, encounter tables and so on are always read from the user's own ROM. We distribute code and *addresses*, never Nintendo or Game Freak assets. This keeps the project legally clean and makes every romhack work, because the data always matches the user's exact ROM version.
2. **Local first.** The agent is fully useful with no account and no internet. The cloud adds sharing, sync and docs, but never gates a local feature.
3. **Never lose a save.** Every write to a save file (restore, bank transfer, sync) backs up first, and refuses to run while that game is open.
4. **Games are identified by ROM hash**, never by filename.
5. **Profiles are data, not code.** Supporting a new game or hack means writing a profile, not shipping a release.
6. **Light on the device.** The agent must never make a game stutter on a Raspberry Pi 4.

## Architecture

```
 Device (Pi / Deck / PC)                          Cloud
┌──────────────────────────────────┐        ┌──────────────────────────────────┐
│ Agent (Go, this repo)            │        │ API (NestJS + Postgres)          │
│  ├ RetroArch UDP / process memory│        │  ├ accounts, devices (pairing)   │
│  ├ save watcher + backups        │◄──────►│  ├ profile registry              │
│  ├ event log (SQLite)            │ HTTPS  │  ├ docs: generated + curated     │
│  ├ ROM data extraction           │        │  └ optional save/bank sync       │
│  └ local web UI (phone/laptop)   │        │ Website (Next.js)                │
└──────────────▲───────────────────┘        │  └ public docs, guides, accounts │
               │ save files                 └──────────────────────────────────┘
┌──────────────┴───────────────────┐
│ Bank (desktop, OpenHome-based)   │
└──────────────────────────────────┘
```

- **Agent:** what exists today, generalised. One binary per platform.
- **API and website:** TypeScript (NestJS, Next.js), Cam's home turf. Postgres for accounts, the profile registry, curated docs and generated game data keyed by ROM hash.
- **Bank:** builds on [OpenHome](https://github.com/andrewbenington/OpenHome). It's GPL-3.0, built with Tauri, React and TypeScript plus a Rust core (`pkm_rs`, `openhome_core`), and already supports every official save from Gen 1 to Gen 9 plus Radical Red, Unbound, Luminescent Platinum and Compass. See "Bank strategy" below.

## Milestones

### M1: Multi-game foundation (agent) ✅ (2026-10-09)
- [x] **Auto-detect the running game** from emulator command lines in `/proc` (RetroArch and DraStic), hash the ROM, switch profile. `-profile` is now only the fallback where `/proc` doesn't exist (the Mac).
- [x] **Unsupported-game screen** on the Party page, with the ROM's SHA-1; a DS game in DraStic gets its own message.
- [x] **Per-game settings** in SQLite (migration 2). For now the only setting is the Nuzlocke toggle; a full settings page will come with more settings.
- [x] **Nuzlocke mode toggle,** per game, off by default. Off: a "Journey" log. On: Dead badges, faint-as-death, "Routes".
- [x] **Library page:** every ROM, identified in the background (hashes cached in `rom-hashes.json`), with profile status, saves, and what's playing.
- [x] **Extra save locations:** DraStic's `.dsv` files are backed up.
- [x] **Repeat PC catches:** the tracker reads the PC from RAM every 10 s; new Pokémon in boxes are logged as PC catches. Not yet seen live (needs a RetroArch game and a catch to the PC).
- Restores are now refused only while *that save's* game is open (was: any game).

### M2: More games
- [x] **Unbound profile** (2026-10-09): verified against its ROM, Cam's real save and the running game (party, items, location, live PC, Pokédex 20/69). Its long names are pointers (`gen3.ReadName`). Time-of-day periods verified from its code (2026-10-10).
- [x] **Focus-game polish** (2026-10-10): Radical Red and Unbound boxes 20–25 (CFRU box table; sectors 30–31, SaveBlock1, SaveBlock2); HeartGold Generations live PC from RAM, caught/seen marks from the Gen 4 Pokédex, recovery when DraStic's save data moves; Boxes tab refreshes every 5 s.
- [x] **Emerald (US) profile** (2026-10-09): ROM tables verified; RAM addresses confirmed by code-reference counts, not yet live.
- [x] **Automatic profiles** (`autoprofile/`) for FireRed-engine hacks (CFRU and classic): match the hand-made Radical Red/Unbound profiles exactly; generated for Odyssey and Sword & Shield Ultimate Plus. Library shows "auto-detected" / "not yet tested live".
- [ ] Vanilla FireRed: still unverified (no ROM on the Pi).
- [ ] **Decomp-based hacks** (Emerald Rogue, Heart and Soul, SoulGold, R.O.W.E.): layouts change per build (e.g. 13-byte names); need per-build research. Library explains this.
- [x] **DS sprites** (2026-10-09): live party, Boxes tab and save viewer; normal, shiny and female variants, read on demand from the ROM's sprite archive.
- [x] **DS route encounters** (HeartGold/SoulSilver family, 2026-10-09): current map from the live general block (the first of five location records, which updates every step; `+0x1424` in HeartGold Generations), map headers from DS RAM (`0x020F6BE0`, byte 0 wild file, byte 0x12 location name), encounter files `/a/0/3/7` (196 bytes; morning/day/night). Verified live while walking Route 30 ↔ Cherrygrove City. Platinum still to do.
- [x] **Polish pass, part 1** (2026-10-09): phone layout checked at 390px on every page (header wraps, Saves rows wrap, empty party slots fold into one line); the Boxes tab shows a DS game's latest save instead of an error.
- [x] **Gen 4 save files (DS)** (2026-10-09): Platinum and HeartGold/SoulSilver families, including expanded hacks (HeartGold Generations: 30 boxes, 1,393 species). Backups, box viewer and Library. Names and level tables come from the ROM's file system (message files found by content). Diamond/Pearl isn't wired up (no ROM to verify against).
- [x] **DraStic live memory** (2026-10-09): works. DraStic maps the DS's main RAM at `0x02000000` in its own process; the agent reads it through `/proc/<pid>/mem`, finds the general save block by scanning, and follows the party live (Party page and journey log). Still to do for DS: sprites, map/encounters, live PC.
- [ ] **Gen 5 (Black/White and hacks),** after Gen 4.
- [ ] **Game Boy / Game Boy Color (Gen 1–2).** RetroArch cores (Gambatte) support memory reads; Gen 1–2 structures are simpler.

### M3: Game data from the ROM
- [x] **Species pages** (2026-10-09): types, base stats, abilities (incl. CFRU hidden), evolutions (all of Eevee's), level-up moves. Gen 3 (vanilla packed and CFRU 3-byte learnsets, 5 or 16 evolution slots, found via FireRed/Emerald code literals; auto-detected for generated profiles) and Gen 4 (personal, learnset and evolution archives; HeartGold Generations' (move, level) learnsets). Opened from party cards, route encounters and the box viewer.
- [x] **Evolves from** (Gen 3 and DS) and **where to find it** (Gen 3: every map in the main and time-of-day tables, named via `gMapGroups` → section byte 0x14; Radical Red/Emerald via code literals, Unbound's table found by content at `0x083526A4`). Whole-ROM indexes are built on first use and cached. Vanilla profiles set `speciesCount` (412) because their move names follow the species names.
- [x] **TM/HM, tutor and egg moves** (2026-10-09). Gen 3: tables via FireRed/Emerald code literals (TM list `[0x08125A8C]`, TM bits `[0x08043C68]`, tutors `[0x08120BE4]`/`[0x08120C30]`, egg moves `[0x08045C50]`); bitfield sizes per game (FireRed 8/2, Emerald 8/4, Radical Red 16/16, Unbound 16/20 for 146 tutors), measured automatically for generated profiles. DS: TM list from the decompressed ARM9, TM bits in base stats 0x1C, egg moves in `/a/2/2/9` (HGSS) or overlay 5 (Platinum). DS tutors not yet.
- [x] **DS where-to-find** (HeartGold/SoulSilver family, 2026-10-09): the ARM9 binary is BLZ-decompressed so map headers are read offline (540 maps in vanilla; the table ends where music IDs stop). Platinum still to do.
- [x] **DS move tutors** (2026-10-09): tutor list found in an overlay by its first moves (HGSS 4-byte entries + `waza_oshie.bin`; Platinum 12-byte entries with compatibility right after).
- [x] **Trainers and their teams** (2026-10-09): Trainers page (search, grouped by area in the game's map order, rematches folded), "Trainers here" in the Route section, Library links. Gen 3: `gTrainers` via FireRed/Emerald code literals (`[0x08044028]`, classes `[0x080D80A0]`), placed on maps by scanning people's scripts for `trainerbattle` (Radical Red 449, Emerald 496 placed; Unbound's map events use another layout, so its trainers aren't placed yet). DS: `trdata`/`trpoke` archives, names in 9-bit compressed text; not placed on maps yet.
- [x] **Maps: items, shops, connections, warps** (2026-10-09). Gen 3: one reader for map headers and events (`gen3.ReadMaps`): item balls (finditem scripts), hidden items (background events, kind 7), shops (`pokemart`), connections and warps. DS: zone events (people, signs, warps); item balls via the item-ball script file, hidden items via a table in the ARM9, trainers via trainer script IDs (3000+) and the map's own scripts (battle command 0xE5 Platinum / 0xD5 HGSS). Route section shows the current map's items, shop and trainers. In the export as `maps`.
- [x] **Trainer locations for Unbound and DS** (2026-10-09): Unbound uses CFRU battle type 13; every gym leader placed in HGSS and Platinum.
- [x] **Platinum encounters** (2026-10-09): map headers at `0x020E601C` (wild file u16 at 0x0E), 424-byte encounter files; where-to-find and (untested live) the Route section.
- [x] **DS shop stock** (2026-10-11, HeartGold/SoulSilver family): the standard Poké Mart's (item, tier) table and the specialty-shop list table, found through the script command table (0x113/0x114); clerks call them directly or via `setvar 0x8004` + `callstd 0x800/0x804`. Badge tiers in `shopBadges`. Platinum not done yet (different commands).
- [x] **Hack-specific evolution methods** (2026-10-09): CFRU 16–28 (rain, move type, party type, location, gender, day/night, held item, move, party member, time range), Mega/Gigantamax, and each hack's later numbering (`evolutionScheme`: radical-red, unbound); HeartGold Generations' 27–36. Radical Red's Fairy type name comes from its second type table (`[0x080D8510]`).
- [x] **Versioned JSON export** (2026-10-09): `tallgrass -export game.gba` or Library → Export data (`GET /api/export?rom=`). Species (stats, moves, evolutions, where to find) and trainers, keyed by ROM SHA-1; format in `docs/gamedata-export.md` (version 1).

### M4: Cloud platform
- [x] **API and database** (2026-10-09): Supabase (project `ifjlueqhuyaafjuzltoi`, London): schema in `supabase/migrations/`, Edge Functions in `supabase/functions/`. No separate API server for now.
- [x] **Accounts** (2026-10-09): Discord, GitHub and email sign-in links on https://tallgrass-delta.vercel.app. Verified with Cam's Discord sign-in.
- [x] **Device pairing** (2026-10-09, verified with Cam's Pi): dashboard Account page shows a code (XXXX-XXXX, 10 minutes); entering it at /link gives the agent its own key (only its hash is stored).
- [x] **Public docs website, first version** (2026-10-09): live at https://tallgrass-delta.vercel.app with Radical Red, Unbound and HeartGold Generations uploaded from Cam's Pi. Still to come below:
  - Generated pages per game/hack version, built from M3 data uploaded by agents or maintainers. Only extracted *facts* (stats, learnsets), never sprites or other assets.
  - [x] User guide at `/guide` (2026-10-11): install, setup wizard, dashboard tour, PIN, account linking, updates, troubleshooting, uninstall.
  - [x] **Game guides** (2026-10-12): `/games/<slug>/guide` for Radical Red, Unbound and HeartGold Generations: what the hack adds, what Tallgrass does with it, getting started, and links to the official thread and docs. Written in our own words from the hacks' public information (no author outreach, nothing copied); content lives in `web/src/lib/guides.ts`.
  - [x] R.O.W.E. guide and page (2026-10-12): `games` row added (migration `20261012100000_game_rowe.sql`), game data uploaded from Cam's Pi (1,381 species, 944 trainers, 769 maps), guide at `/games/rowe/guide`.
  - [x] **Bosses pages** (2026-10-12): `/games/<slug>/bosses` lists gym leaders, the Elite Four and champions, rivals and villain bosses with full teams, from the game data (grouped by name, placeholder teams dropped, teams lowest level first; the ROM doesn't say which fight comes first). Step-by-step walkthroughs are *not* written: the hacks' own walkthroughs are the authority and we can't verify routes from the ROM.
  - Walkthroughs: link to community ones where an official or well-known one exists (not done: the official threads block automated reads, so links need a human check). Links to a hack's official docs are in the guides; ask the author first before copying any of their content.
- [x] **Profile registry** (2026-10-11): `profiles/` on main is published to Supabase (`game_profiles`) by CI; agents download it every 6 h and apply without a restart; community submissions are reviewed as pull requests. `minTallgrass` keeps older agents from profiles they can't read.
- [x] **Optional sync** (2026-10-11): save backups (newest 30 versions per save) and Journey history to the account, switched on per device; downloads on the website's Account page.

### M5: Bank
- [x] **Bridge first** (2026-10-11): download a save from the agent → edit it in OpenHome → upload it back (Saves → Upload edited save, `POST /api/saves/upload`). The agent checks it's a save of the same kind, backs up first and refuses while the game runs; bare 512 KB DS saves get DraStic's footer back.
- [x] **Sync folder** (2026-10-11): `tallgrass sync <device>` on a Mac/Windows/Linux computer keeps a folder in step with the device's saves, laid out so OpenHome's Suggested saves finds them; edits saved there are uploaded back (backup first, waits while the game runs). Needs no OpenHome changes (its plugins only supply sprites).
- [ ] **Deeper integration:** OpenHome talks to the agent directly (a "Tallgrass device" save source). Ideally contributed upstream; otherwise in a fork.
- [ ] **Account-linked bank:** an OpenHome bank stored in the user's cloud account, so it follows them between devices.

### M6: Consumer packaging
- [x] **Local web UI security** (2026-10-10): optional dashboard PIN (Account page, and a step in the setup wizard); salted hash on the device, 30-day sign-ins, slowed guessing.
- [x] **Setup wizard, per game** (2026-10-10; fixes tested against a fake RetroPie, checks verified on Cam's Pi for all three games): the user picks their game (Radical Red, HeartGold Generations or Unbound); the wizard finds that ROM by hash (or says which version it needs), finds its save, turns on what that emulator needs (RetroArch network commands and autosave, or checks DraStic), then asks them to load the game and confirms a live read. Only the chosen game is prepared.
- [x] **Installer, first version** (2026-10-11): `curl … /install.sh | bash` for Raspberry Pi and Linux PCs (armv7, arm64, amd64), systemd service. Since then: macOS/Windows builds (for `tallgrass sync`) and a RetroPie-Setup module (`/retropie/tallgrass.sh`, verified on Cam's Pi). Batocera dropped (2026-10-11, not needed).
- [x] **Self-update** (2026-10-11): Ed25519-signed checksums; Account → Updates installs and restarts. Public since v0.1.0 (2026-10-11); Cam's Pi installed it with the one-line installer.
- [x] **Docs** (2026-10-11): install and troubleshooting on the website's `/guide`; [writing a game profile](docs/writing-a-profile.md) with field reference and the techniques used for R.O.W.E.; local profiles folder (`-profiles-dir`, default `profiles`) to try one without rebuilding.
- [x] **CI** (2026-10-11): GitHub Actions for tests and tagged releases. Still to do: a private test corpus of saves and ROM-derived fixtures, never published.
- [x] **Reflash Cam's Pi** (2026-10-12): from Raspbian Buster to Raspberry Pi OS 12 (Bookworm, 32-bit userland) with RetroPie 4.8.12 installed by the setup script; data restored from a Mac backup; the live read verified for R.O.W.E. (RetroArch 1.19.1) and HeartGold Generations (DraStic). Gotchas and the procedure are in docs/handbook.md ("Reflashing the Pi").

### M7: Romhack library manager (the original Phase 8)
- [x] Apply `.ips`/`.bps`/`.ups`/`.xdelta` patches (2026-10-11): Library → Patch a ROM, `POST /api/library/patch`. BPS/UPS find their base by size + CRC32 and verify the result; xdelta (VCDIFF, no secondary compression) checks each window's Adler-32; IPS checks nothing. Verified with a real HeartGold → HGG xdelta (SHA-1 matches).
- [ ] A known-hacks catalogue in the registry: name, base ROM hash, patch source link (we link, never host), result hash.
- [x] EmulationStation `gamelist.xml` (2026-10-11): proper names and a short description for recognised games, keeping everything else (Library page). Artwork: not done (needs what users supply or hack authors permit).

## Decisions to make

Researched 2026-10-09. Prices and policies change; re-check before committing money.

### Product name
- **Rules:** nothing Nintendo or The Pokémon Company owns. No "Pokémon"/"Poké", no species, region or item names (Rotom, Kanto, Poké Ball), no "-dex" (it echoes Pokédex, and that space is crowded: PartyDex and PocketDex already exist). Nintendo has taken down fan projects that use its marks and assets (e.g. Pokémon Essentials in 2018, Pokémon Uranium); tools that only work on user-owned data (PKHeX, Showdown) have survived for years.
- **Shortlist** (no existing software found under these names in a quick search; still needs domain, GitHub org, app store and trademark checks): **Boxkeep**, **Routebook**, Fieldkit-style names. "TrainerKit" is taken (a cycling app).
- **Leaning:** something about *your* games and journey rather than the franchise; decide before the website goes public.

### Licence
- **Ecosystem norms:** PKHeX is GPL-3.0, OpenHome is GPL-3.0, the Pokémon Showdown server is MIT.
- **Leaning:**
  - **Agent:** GPL-3.0. Lets us reuse OpenHome's code and Rust crates, and keeps forks open. It fits the save-tool community.
  - **Server/website:** AGPL-3.0 if open-sourced (GPL's "network use" version, so hosted forks must share changes). It can also stay private at first; nothing forces it open.
  - **Profiles and generated data:** CC0. Addresses and table layouts are facts; CC0 makes community contributions and reuse frictionless.
  - Use a DCO (sign-off on commits) from the first outside contribution, to keep relicensing options simple.

### Cloud hosting
- **Supabase:** Postgres + auth (incl. Discord/GitHub OAuth) + file storage in one. Free tier: 2 projects, 500 MB DB, 1 GB storage, 50k monthly users, but **pauses after a week of inactivity**. Pro is $25/mo (8 GB DB, no pausing).
- **Railway:** cheapest for a small always-on NestJS API (~$5/mo hobby plan, usage-based).
- **Render:** free tier sleeps after 15 min; $7/mo starter; Postgres from ~$6/mo plus storage.
- **Fly.io:** no free allowance for new organisations since Oct 2024; managed Postgres from $38/mo, so a poor fit here.
- **A €4 Hetzner VPS** is cheapest of all but means running servers ourselves.
- **Leaning:** **Supabase** (database, auth, save storage) + **Railway** (NestJS API) + **Vercel or Cloudflare Pages** (Next.js site; check Vercel's free tier is non-commercial only). About $0 while building, ~$30–35/mo at launch. A Supabase connector is already set up in Claude Code, which helps.

| Decision | Leaning | When |
|---|---|---|
| Product name | Undecided (2026-10-09); shortlist above | Before the website goes public |
| Licence | **Decided 2026-10-11:** GPL-3.0 for all code (agent, website, functions), CC0 for profiles | Done (v0.1.1) |
| Cloud hosting | **Decided 2026-10-09: Supabase** (Cam knows it) for database, auth and storage; API and site hosts still open (Railway, Vercel/Cloudflare) | Start of M4 |
| Bank | Bridge first (M5), then upstream contributions; fork only if upstream won't take it | M5 |
| Generated docs | Maintainers upload first; opt-in community uploads later | M4 |

## Risks

- **Legal:** distributing ROMs, patched ROMs, sprites or text dumps would put the project at risk. Principle 1 covers this; every new feature gets checked against it. Hack authors' docs need their permission.
- **GPL-3.0:** code copied or linked from OpenHome makes that component GPL-3.0. Talking to OpenHome over files or an API doesn't.
- **DraStic:** process memory reading may break between DraStic versions; signature scanning instead of fixed offsets reduces that.
- **Romhack churn:** a new hack version means new hashes and sometimes new addresses. The profile registry and the unsupported-game screen keep that manageable.
- **Pi performance:** every live feature stays within the 1-second poll budget; anything heavy runs on demand or in the cloud.
