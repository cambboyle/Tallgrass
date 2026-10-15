# Tallgrass handbook

How Tallgrass fits together and how to do every routine job: developing, testing, releasing, running the website and backend, adding games, and finding data in a ROM. The other docs go deeper on single topics:

- [Releasing](releasing.md): the release workflow and the signing key
- [Writing a game profile](writing-a-profile.md): adding a game (profile fields and how to find addresses)
- [Game data export format](gamedata-export.md): the game data export format the website reads
- [Roadmap](../ROADMAP.md): what's planned, and the principles every feature follows
- [Technical notes](../CLAUDE.md): the detailed technical notes (every address, format and gotcha found so far)

## Contents

1. [The pieces](#1-the-pieces)
2. [Rules that never bend](#2-rules-that-never-bend)
3. [The repository](#3-the-repository)
4. [Developing the agent](#4-developing-the-agent)
5. [Testing](#5-testing)
6. [Trying a change on the Pi](#6-trying-a-change-on-the-pi)
7. [Releasing](#7-releasing)
8. [How people install it](#8-how-people-install-it)
9. [The website](#9-the-website)
10. [The backend (Supabase)](#10-the-backend-supabase)
11. [Accounts, devices and cloud features](#11-accounts-devices-and-cloud-features)
12. [Game profiles and the registry](#12-game-profiles-and-the-registry)
13. [Adding a game to the website](#13-adding-a-game-to-the-website)
14. [Finding data in a game](#14-finding-data-in-a-game)
15. [Editing saves on a computer (OpenHome)](#15-editing-saves-on-a-computer-openhome)
16. [Secrets and where they live](#16-secrets-and-where-they-live)
17. [Troubleshooting](#17-troubleshooting)
18. [Checklists](#18-checklists)
19. [Reflashing the Pi](#19-reflashing-the-pi)

## 1. The pieces

```
 Raspberry Pi (RetroPie)                        Internet
 +------------------------------+              +-------------------------------+
 | RetroArch (UDP 55355) ---+   |              | GitHub: code, CI, releases    |
 | DraStic (process memory) +   |              | Vercel: the website           |
 | save files --------------+   |   <------>   | Supabase: accounts, devices,  |
 | ROMs (read only) --------+   |              |   profile registry, game      |
 |                          v   |              |   data, cloud backups         |
 |           tallgrass agent    |              +-------------------------------+
 |           dashboard :8080    |
 +--------------+---------------+
                |
   phone or laptop browser; a computer running `tallgrass sync` (OpenHome)
```

| Piece | What it is | Where |
|---|---|---|
| **Agent** | One Go binary. It reads the game live, backs up saves, keeps the Journey, and serves the dashboard on port 8080. It also has the `sync` and `-export` commands. | Repository root (`main.go` plus packages) |
| **Dashboard** | Plain HTML/JS pages built into the binary | `dashboard/static/` |
| **Website** | Next.js: guide, game docs, sign-in, linking devices, cloud backups | `web/` on Vercel |
| **Backend** | Supabase: Postgres tables, storage buckets, Edge Functions | `supabase/` |
| **Profiles** | JSON describing where each game keeps its data | `profiles/`, plus the registry |
| **Installers** | `install.sh` (one line), the RetroPie-Setup module | `web/public/` |

## 2. Rules that never bend

1. **Never ship game data.** No ROMs, sprites, names or text in the repository, the releases or the website's own files. Everything is read from the user's own ROM. Profiles only say *where* data is. The website's game docs come from exports that a maintainer uploads from their own ROM.
2. **Never lose a save.** Anything that writes a save (restore, upload, sync, setup fixes) backs up the current file first. It refuses while that game is running, because the emulator would overwrite the file on exit.
3. **Stay light on the Pi.** The service runs at `Nice=10` with idle I/O, and games come first.
4. **No surprise changes to the Pi's system.** Config edits keep a `.bak-tallgrass` copy, and the setup wizard refuses to edit while a game runs. The Pi is on Raspberry Pi OS 12 (Bookworm), so updates are fine, but play a game afterwards to check the live read still works.
5. **Nothing leaves the device unless the user switches it on.** Accounts, cloud backups and game data uploads are all opt-in.
6. Commits are by Cameron Boyle, with no AI attribution lines.

## 3. The repository

| Path | What's there |
|---|---|
| `main.go` | Flags, start-up, and the service's wiring |
| `synccmd.go`, `savesync/` | `tallgrass sync` (desktop save sync for OpenHome) |
| `retroarch/` | UDP memory reads (`READ_CORE_MEMORY`) |
| `emulator/` | Finds the running game (`/proc`), reads DraStic's memory |
| `gen3/` | GBA: Pokémon formats (vanilla, CFRU, R.O.W.E.), saves, names, sprites, encounters, trainers, maps |
| `gen4/` | DS: ROM file system, saves, live RAM, names, sprites, trainers, shops |
| `profile/`, `profiles/` | Game profile format, and the bundled profiles |
| `autoprofile/` | Generates profiles for unknown FireRed-based hacks |
| `dashboard/` | HTTP server, the poller, and every page and API |
| `tracker/` | The Journey / Nuzlocke event engine and SQLite store |
| `saves/` | Save watcher and versioned backups |
| `patch/` | IPS/UPS/BPS/xdelta patching |
| `gamelist/` | EmulationStation `gamelist.xml` names |
| `cloud/` | Client for the Supabase Edge Functions |
| `update/` | Self-update (signature-checked) |
| `cmd/ramdump` | Dumps game RAM, for reverse engineering |
| `cmd/sign` | Signs release checksums |
| `web/` | The website |
| `supabase/` | Database migrations and Edge Functions |
| `deploy/` | The systemd unit used by `make install-service` |
| `.github/workflows/` | `ci.yml`, `release.yml`, `profiles.yml` |

## 4. Developing the agent

Requirements: Go (the version in `go.mod`) and SSH access to the Pi (`pi@192.168.0.227`, key auth).

```bash
make run            # run the dashboard on the Mac, reading RetroArch on the Pi over the LAN
open http://localhost:8080
```

- Running on the Mac, it can't see the Pi's processes, ROMs or saves. Use `-profile radical-red` to choose the game, and expect save/library features to be off. For everything else, test on the Pi (section 6).
- `go run . -print` prints the party once.
- `go run . -export path/to/rom.gba > game.json` writes a ROM's game data with no emulator running.
- The dashboard UI is plain files in `dashboard/static/`. There's no build step, but the files are embedded, so restart after editing them.
- Each game-specific number goes in a profile, not in code (section 12).

## 5. Testing

```bash
go test ./...       # everything that doesn't need a ROM (CI runs this)
go vet ./...
```

ROM-backed tests skip unless you point them at files. CI has no ROMs, so these can break unnoticed: run them after touching boxes, saves or encounters.

```bash
scp "pi@192.168.0.227:RetroPie/roms/gba/1636 - Pokemon Fire Red (U)(Squirrels) (patched).gba" ~/roms/rr.gba
scp "pi@192.168.0.227:RetroPie/roms/gba/1636 - Pokemon Fire Red (U)(Squirrels) (patched).srm" ~/roms/rr.srm
TALLGRASS_ROM=~/roms/rr.gba TALLGRASS_SAVE=~/roms/rr.srm go test ./...
```

Keep ROMs and saves outside the repository.

Website: `cd web && npm run lint && npm run build`. CI runs both.

**Live check:** anything touching memory reads or decoders also gets tested on the Pi with the game running (section 6).

## 6. Trying a change on the Pi

Two ways:

| | `make deploy` | Release |
|---|---|---|
| What runs | `dev-<commit>` build | `X.Y.Z` |
| Self-updates afterwards | **No** (dev builds never update) | Yes |
| Use for | Quick experiments | Anything finished |

```bash
make deploy         # tests, cross-compiles for armv7, copies, restarts the service
make logs           # follow the service log
make status
```

After a `make deploy`, get the Pi back onto releases by tagging a release (section 7) and re-running the installer. Or run it from the Pi:

```bash
curl -fsSL https://tallgrass-delta.vercel.app/install.sh | bash
```

Useful while testing (on the Pi):

```bash
curl -s localhost:8080/api/party | head -c 500
curl -s localhost:8080/api/update          # running version
journalctl -u tallgrass -n 50 --no-pager
```

## 7. Releasing

1. Commit and push to `main`.
2. Wait for CI (`ci.yml`) to pass: `gh run list --workflow ci.yml --limit 1`.
3. Tag the commit and push the tag:
   ```bash
   git tag v0.1.24 && git push origin v0.1.24
   ```
4. `release.yml` tests again, builds the release files, writes `SHA256SUMS`, signs it (`SHA256SUMS.sig`) and publishes the GitHub release:
   - Linux: armv7, arm64, amd64
   - macOS: arm64, amd64
   - Windows: amd64
   - `install.sh`
   - the systemd unit
5. Update the Pi: dashboard → **Account → Updates → Install**. Or use the API:
   ```bash
   ssh pi@192.168.0.227 'curl -s "localhost:8080/api/update?check=1"; curl -s -X POST -H "Content-Type: application/json" -d "{}" localhost:8080/api/update/apply'
   ```
6. Every install checks for updates every 6 hours. It installs one only after the user clicks Install, and only if the signature and checksum match.

Version numbers: `X.Y.Z`. Agents offer any release higher than their own. If a release fails (for example a flaky test), fix it and tag the next number; don't reuse a tag.

## 8. How people install it

| Route | Command | Notes |
|---|---|---|
| One line (Pi / Linux) | `curl -fsSL https://tallgrass-delta.vercel.app/install.sh \| bash` | Installs to `~/tallgrass` and sets up the `tallgrass` systemd service. Asks for sudo once. |
| RetroPie-Setup | See below | Runs the same installer as root for the RetroPie user |
| Desktop sync | Download `tallgrass-darwin-*` / `tallgrass-windows-amd64.exe` | Only for `tallgrass sync` (section 15) |

RetroPie-Setup module (`web/public/retropie/tallgrass.sh`, served by the website):

```bash
mkdir -p ~/RetroPie-Setup/ext/tallgrass/scriptmodules/supplementary
curl -fsSLo ~/RetroPie-Setup/ext/tallgrass/scriptmodules/supplementary/tallgrass.sh \
  https://tallgrass-delta.vercel.app/retropie/tallgrass.sh
```

Then go to RetroPie-Setup → Manage packages → Manage experimental packages → **tallgrass**. Or use the command line:

```bash
sudo ~/RetroPie-Setup/retropie_packages.sh tallgrass depends
sudo ~/RetroPie-Setup/retropie_packages.sh tallgrass install_bin
```

- Removing the package stops the service and deletes the binary. Backups, the Journey and settings stay in `~/tallgrass`.
- Installer options (environment variables):
  - `TALLGRASS_VERSION=0.1.20` installs a specific release;
  - `TALLGRASS_DIR=` installs somewhere else;
  - `TALLGRASS_USER=` installs for that user when run as root.

What lives in `~/tallgrass` on a device:

| File | What |
|---|---|
| `tallgrass` | The binary (replaced by updates) |
| `tallgrass.db` | Journey / Nuzlocke history and settings (SQLite) |
| `backups/` | Every save version, gzipped: `backups/<system>/<save>/<time>-<hash>.ext.gz` |
| `auth.json` | Dashboard PIN (hashed) and sign-ins. Delete it to remove a forgotten PIN. |
| `cloud-link.json` | The device's account key (after linking) |
| `cloud-sync.json` | What cloud backup has already sent |
| `registry-profiles.json` | Cached profiles from the registry |
| `rom-hashes.json` | Cached ROM SHA-1s |
| `profiles/` | The user's own profiles (override everything else) |

## 9. The website

- **Hosting:** Vercel project with root directory `web/`. Every push to `main` deploys. URL: https://tallgrass-delta.vercel.app. (`tallgrass.vercel.app` is someone else's site.)
- **Stack:** Next.js 16 (App Router), Supabase Auth.
  - Next 16 differs from older versions: `proxy.ts` instead of middleware, async `params`/`cookies()`, and ISR via `revalidate`.
  - When unsure about an API, read `web/node_modules/next/dist/docs/`.
- **Environment variables (Vercel and `web/.env.local`):** `NEXT_PUBLIC_SUPABASE_URL` and `NEXT_PUBLIC_SUPABASE_ANON_KEY`. Both are public by design.
- **Analytics:** Vercel Web Analytics (`<Analytics />` in `layout.tsx`, no cookies). It has to be enabled in the Vercel project's Analytics tab.

Run it locally:

```bash
cd web
cp .env.example .env.local     # fill in the two values
npm install
npm run dev                    # http://localhost:3000
GAMEDATA_DIR=/path/to/exports npm run dev   # game docs from local exports (<slug>.json)
```

| Page | What |
|---|---|
| `/` | Landing page |
| `/guide` | The user guide: install, setup, dashboard, PIN, editing saves, account, updates, troubleshooting |
| `/games`, `/games/[slug]/…` | Game docs: Pokémon, areas, trainers (from uploaded exports) |
| `/login`, `/account`, `/link` | Sign-in, devices, cloud backups, entering a pairing code |
| `/install.sh`, `/retropie/tallgrass.sh` | Installers (`web/public/`) |

## 10. The backend (Supabase)

Project `ifjlueqhuyaafjuzltoi` (London), managed through the Supabase dashboard or the Supabase MCP tools.

**Tables**

| Table | What |
|---|---|
| `profiles` | One per user, created at sign-up. Maintainers have `is_maintainer = true`, set by hand. |
| `devices` | Linked devices (only a SHA-256 of each device key is stored) |
| `pairing_codes` | Short-lived codes for linking (server-only) |
| `games` | Games the website documents (slug, name, generation, ROM SHA-1s) |
| `game_data` | Uploaded exports (files in the public `gamedata` bucket) |
| `game_profiles` | The profile registry (public read, written only by `profiles-sync`) |

**Storage buckets:** `gamedata` (public, game exports) and `backups` (private, cloud backups, readable only by their owner).

**Edge Functions** (`supabase/functions/`; all have `verify_jwt = false` and check callers in code):

| Function | Caller | Does |
|---|---|---|
| `pair-start` | Device | Makes a pairing code and a poll secret |
| `pair-claim` | Signed-in user | Links the code's device to the account |
| `pair-poll` | Device | Picks up its device key, once |
| `device` | Device key | Account info for the dashboard |
| `gamedata-upload` | Maintainer device | Stores a game data export |
| `backup-upload` | Device key | Stores a cloud backup (save version or Journey) |
| `profiles-sync` | CI (`X-Sync-Secret`) | Publishes `profiles/` to the registry |

**Changing the schema**

1. Write a new file `supabase/migrations/<YYYYMMDDHHMMSS>_<name>.sql`. Never edit an applied one.
2. Apply it (Supabase MCP `apply_migration`, or the SQL editor).
3. Commit the file.

**Deploying a function:** deploy its `index.ts` together with `_shared/util.ts` (MCP `deploy_edge_function`, `verify_jwt: false`). Then commit. The repository is the source of truth, so keep the deployed version and the file in step.

**Function secrets** (Supabase → Edge Functions → Secrets): `PROFILES_SYNC_SECRET`. The Supabase keys are provided automatically.

## 11. Accounts, devices and cloud features

**Linking a device**

1. Dashboard → **Account → Link this device**. The agent calls `pair-start` and shows a code.
2. The user enters the code at `/link` while signed in (`pair-claim`).
3. The agent polls (`pair-poll`), receives its device key once, and keeps it in `cloud-link.json`.
4. Unlink from the website's account page, or from the dashboard.

**Cloud backups** (opt-in)

- Switched on from the dashboard's Account page (setting `cloud.sync`).
- Every 10 minutes the agent sends save versions it hasn't sent yet, plus the Journey export when it changed, to `backup-upload`.
- Files are stored as `backups/<user>/<device>/<system>/<save>/<version>`, and the newest 30 versions per save are kept. Names with characters that storage keys don't allow are stored as `u-` + base64url, and the website decodes them.
- Users download them from `/account`.

**Game data for the website** (maintainers only)

1. The maintainer's device must be linked, and their account needs `is_maintainer`.
2. Dashboard → **Library** → **Upload to Tallgrass** on the game's ROM. The agent exports the ROM's data and sends it to `gamedata-upload`.
3. The upload is accepted only if the ROM's SHA-1 is listed in the `games` table (section 13).

## 12. Game profiles and the registry

A profile is JSON that tells the agent where a game keeps its data (`profiles/<slug>.json`). Gen 4 (DS) games need none. The full guide is [writing-a-profile.md](writing-a-profile.md).

Where profiles come from, highest priority first:

1. **Local:** `~/tallgrass/profiles/*.json` on the device. Use this for testing, or for users' own hacks.
2. **Registry:** downloaded from Supabase at start-up and every 6 hours, and applied without a restart.
3. **Bundled:** built into the binary.
4. **Auto:** generated for unknown FireRed-based hacks (`autoprofile/`).

**Shipping a profile change to everyone, without a release**

1. Edit or add `profiles/<slug>.json`. Run `go test ./profile/ ./dashboard/`.
2. Push to `main`. `profiles.yml` sends every profile to `profiles-sync`, which bumps the version of any that changed. Run it by hand with `gh workflow run profiles.yml`.
3. Devices pick it up within 6 hours, or at their next restart.
4. If a profile needs a newer agent, set `"minTallgrass": "0.1.20"`. Older agents then skip it.

Community submissions come in as pull requests to `profiles/`, and get reviewed like code.

## 13. Adding a game to the website

1. Add a row to `games`, in a new migration:
   ```sql
   insert into public.games (slug, name, generation, rom_sha1)
   values ('rowe', 'Pokémon R.O.W.E.', 3, '{ab2196855ac47598cb2ef7ebff475d6ed17b1992}');
   ```
2. Apply it and commit it.
3. From a maintainer device with that ROM: **Library → Upload to Tallgrass**.
4. The pages appear under `/games/<slug>` within 10 minutes (the game list within an hour).

To change what's exported, edit `dashboard/export.go` and keep `docs/gamedata-export.md` in step. Bump `ExportVersion` only when a field changes meaning.

### The hack catalogue

`catalog/hacks.json` (CC0) lists every hack Tallgrass supports: author, version, the base game it needs, a link to its official page (we link, never host ROMs or patches), and the SHA-1 of the build Tallgrass is tested against. It's embedded in the agent (the Setup wizard uses it when your ROM isn't found: it says who made the game, what to patch and links to the page) and read from GitHub by the website's `/hacks` page, so a change on `main` shows within 10 minutes.

- **Add a hack:** add an entry under `hacks` (and its base game under `bases` if new). `go test ./catalog/ ./dashboard/` checks the file hangs together and that the Setup wizard and the catalogue agree on each game's SHA-1.
- **Only list a checksum you checked** against a real copy (`shasum -a 1 file`). A base game with no checked SHA-1 simply has none, and its `detail` says where to find the right version. A wrong hash can mislead people, a missing one can't.
- **Links** come from the hack's official thread. Some forums block automated reads, so a person should click each new link once.
- Slugs match the game's slug in `setupGames`, `profiles/` and the `games` table.

## 14. Finding data in a game

The methods used so far, roughly in the order to try them. CLAUDE.md records every result.

1. **Start from the ROM header.** Game Freak's header at `0x08000100` points at the species and move names, items, sprites and base stats. Hacks often keep these pointers up to date (Radical Red's move-name pointer is a known stale one).
2. **Search the ROM for known content.** Encode a name ("BULBASAUR") in the Gen 3 character set and search for it. Then search for little-endian pointers to where it was found. Tables of names and the code that uses them turn up this way.
3. **Follow vanilla code pointers.** Code in vanilla FireRed/Emerald loads tables through literal pools. A ref like `[0x08082990]` reads the pointer stored there, so it keeps working when a hack repoints the table. Profiles use this form wherever possible.
4. **Diff RAM dumps.** `cmd/ramdump` (cross-compile it, copy it to the Pi and run it there) saves EWRAM/IWRAM through RetroArch, or with `ramdump ds`, DraStic's main RAM. Dump, do one thing in the game (catch a Pokémon, deposit it, change a skill), dump again and diff. This is how R.O.W.E.'s Pokédex flags, PC layout and Trainer Skills were found.
5. **Disassemble the game's own code.** Python `capstone` in Thumb mode. Find the function (for example `GetMonData`, the wild encounter selector or the IV getter) through a literal pool or a known caller. Switch tables give field numbers. This is how R.O.W.E.'s packed Pokémon format, IV rule and time-of-day logic were confirmed.
6. **Read the hack's own docs.** R.O.W.E.'s spreadsheet explained that Trainer Skills replace IVs, which the code then confirmed.
7. **Verify live.** Every finding gets checked against the running game (party, route, dex marks, PC) before the profile says `"verified": "live"`.

For DS games, the agent reads the ROM's file system (`gen4/nds.go`): NARC archives, encrypted message files and ARM9/overlays. It finds data by content (for example, the list of names that starts "Bulbasaur") rather than by fixed offsets, so hacks mostly just work.

## 15. Editing saves on a computer (OpenHome)

- `tallgrass sync <device address>` runs on a Mac, Windows or Linux computer.
  - It mirrors the device's saves into `~/Tallgrass Saves`.
  - The folder is laid out for OpenHome's **Suggested** saves: `.sav` for GBA, `.dsv` for DS, all at the top level.
  - Edits saved there are uploaded back through `/api/saves/upload`, which checks the file, backs up first and waits while the game runs.
  - If a save changed on both sides, the device's copy wins and the computer's copy goes into `Conflicts/`.
  - It asks once for the dashboard PIN, if one is set.
- OpenHome supports Radical Red and Unbound saves, but not R.O.W.E. or HeartGold Generations.
- The simpler manual route: Saves → download, edit, then **Upload edited save**.
- OpenHome plugins can only supply sprites. A built-in "Tallgrass device" save source would have to be an upstream contribution to OpenHome.

## 16. Secrets and where they live

Never print, paste or commit any of these.

| Secret | Where | Used by | If it leaks |
|---|---|---|---|
| Release signing key (Ed25519) | `~/.config/tallgrass/release.key` on the Mac; GitHub secret `RELEASE_SIGNING_KEY`; keep a copy in a password manager | `release.yml` signs `SHA256SUMS` | Make a new key (`go run ./cmd/sign -genkey`), put its public half in `update.PublicKey`, release. Installs on the old key must be reinstalled. |
| Profiles sync secret | `~/.config/tallgrass/profiles-sync-secret`; GitHub secret and Supabase function secret `PROFILES_SYNC_SECRET` | `profiles.yml` → `profiles-sync` | Generate a new one and set it in all three places |
| Device keys | Each device's `cloud-link.json` (only hashes in the database) | Agent ↔ Edge Functions | Unlink the device on the website |
| Supabase service keys | Supabase only (provided to functions automatically) | Edge Functions | Rotate them in Supabase |

The Supabase URL and anon key are public on purpose: they're compiled into the agent and the website. Row level security protects the data.

## 17. Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| Dashboard shows no party | No game running, RetroArch network commands off, or the core isn't `lr-mgba` | Setup page checks and fixes all three |
| Game "not supported" | Its ROM's SHA-1 matches no profile | Hover "not supported yet" on the Library page to see the SHA-1. Write a profile (section 12). |
| Timeouts from RetroArch | Unknown commands get **no** reply, and nothing answers while no game is running | Check with `VERSION` first |
| Save didn't update | RetroArch writes `.srm` only on exit, unless `autosave_interval` is set | Setup sets `autosave_interval = 60` |
| Restore/upload refused | The game is running | Quit to EmulationStation first |
| Forgotten PIN | | Delete `~/tallgrass/auth.json` and run `sudo systemctl restart tallgrass` |
| Pi doesn't offer updates | It's running a `dev-` build from `make deploy` | Re-run the installer |
| Game names not in EmulationStation | The names are only written while EmulationStation isn't running | Restart the Pi once |
| Cloud backup error | See `curl localhost:8080/api/cloud/sync` on the device, and the Supabase function and storage logs | |
| Release failed | Usually a test | `gh run view --log-failed`, fix, tag the next version |
| Go programs on the Mac can't reach the Pi over UDP | macOS Local Network privacy | System Settings → Privacy & Security → Local Network |
| Bluetooth controller won't pair after a reflash (`ConnectionAttemptFailed`, "Page Timeout") | The controller still holds a pairing with the Pi's old Bluetooth address, and the new system lost the key | Back up `/var/lib/bluetooth` before reflashing. Otherwise give the Pi a new address (section 19) |
| Screen is blurry or at the wrong size after an OS change | The old firmware `hdmi_*` settings don't apply under KMS | Use `video=HDMI-A-1:<w>x<h>M@59` in `/boot/firmware/cmdline.txt` (section 19) |
| An `ssh` wait loop never ends | `pgrep -f <name>` run over `ssh` matches the ssh shell itself | Wait on a marker file instead |

## 18. Checklists

**New feature**
1. Check it against the rules (section 2) and the roadmap's principles.
2. Put game-specific values in profiles.
3. Write tests (with a fake RetroArch or RAM where possible), then test live on the Pi.
4. Update the user guide (`web/src/app/guide/page.tsx`) if users will see it, plus `CLAUDE.md` and `ROADMAP.md`.
5. Release (section 7).

**New game**
1. Get its ROM SHA-1. Profile it (writing-a-profile.md) and verify it live.
2. Add it to `profiles/` (the registry ships it).
3. Add it to `catalog/hacks.json` (see below), and optionally to `setupGames` for the setup wizard, to `games` for the website, and upload its data.

**Schema or function change**
1. Add a migration file, apply it, and commit it.
2. Deploy the function, then commit it.
3. Check the logs after the first real call.

## 19. Reflashing the Pi

Done once, on 2026-10-12 (Buster to Bookworm); this is what worked, including what to do differently. About 20 minutes to back up, then 1–2 hours.

**Which OS.** RetroPie's own image is still 4.8 on Buster (2022), so flashing it gets you the same old system. Instead use **Raspberry Pi OS Lite, 32-bit, Bookworm** (Debian 12) and RetroPie's install script on top. DraStic is 32-bit only, and the script has prebuilt binaries for Pi OS 10–12 only (Debian 13 would build everything from source).

**1. Back up to the Mac** (nothing is changed on the Pi):

```bash
B=~/Backups/retropie-$(date +%F); mkdir -p $B; chmod 700 $B
rsync -a --no-owner --no-group -e ssh pi@192.168.0.227:/home/pi/RetroPie/ $B/RetroPie/
rsync -a --no-owner --no-group --rsync-path="sudo rsync" -e ssh pi@192.168.0.227:/opt/retropie/configs/ $B/opt-retropie-configs/
rsync -a --no-owner --no-group -e ssh pi@192.168.0.227:/home/pi/tallgrass/ $B/tallgrass/
rsync -a --no-owner --no-group --rsync-path="sudo rsync" -e ssh pi@192.168.0.227:/var/lib/bluetooth/ $B/bluetooth/   # keeps controller pairings
ssh pi@192.168.0.227 'sudo cat /boot/config.txt' > $B/boot-config.txt
```

Also save a list of installed packages (`ls /opt/retropie/{emulators,libretrocores,supplementary}`). Check the copy with `rsync -anc --itemize-changes` (no output means identical). Close the game first, so saves are consistent.

**2. Flash** with Raspberry Pi Imager: Pi 4, Raspberry Pi OS Lite (32-bit) on Bookworm (the imager may call it "Legacy"). Customise: hostname `retropie`, user `pi`, Wi-Fi, SSH on with a public key. **Paste the key carefully**: a stray space made it useless this time. If key login fails, serve the key from the Mac and fetch it on the Pi's keyboard:

```bash
# Mac: serve one file for a minute
D=$(mktemp -d); cp ~/.ssh/id_ed25519.pub $D/key.pub; cd $D && python3 -m http.server 8000 --bind $(ipconfig getifaddr en0)
# Pi (at its keyboard): replace authorized_keys with it
curl -s http://<mac ip>:8000/key.pub > ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys
```

Then on the Mac `ssh-keygen -R 192.168.0.227` (new host key) and connect.

**3. Install RetroPie** (about 15 minutes, binaries):

```bash
sudo apt update && sudo apt -y full-upgrade && sudo apt -y install git dialog unzip xmlstarlet rsync
git clone --depth=1 https://github.com/RetroPie/RetroPie-Setup.git && cd RetroPie-Setup
sudo ./retropie_packages.sh setup basic_install       # run in the background; it logs a lot
for p in drastic bluetooth usbromservice xpad; do sudo ./retropie_packages.sh $p; done
sudo ./retropie_packages.sh autostart enable           # boot straight into EmulationStation
```

**4. Display.** Under KMS the old `hdmi_cvt` lines don't apply. Check the panel's real mode (`edid-decode /sys/class/drm/card*-HDMI-A-1/edid`), then add to the single line in `/boot/firmware/cmdline.txt`: `video=HDMI-A-1:1024x600M@59` (use your panel's size). `M` asks for a calculated mode and `@59` stops the kernel picking an EDID mode that the Pi 4 driver rejects (odd horizontal timings; see `dmesg | grep "not supported"`). Fan: add `dtoverlay=gpio-fan,gpiopin=14,temp=80000` to `/boot/firmware/config.txt`. Reboot and check `cat /sys/class/graphics/fb0/virtual_size`.

**5. Restore** (don't copy whole config folders over the fresh ones: RetroArch is newer): ROMs and saves, DraStic's folder, EmulationStation's folder (settings, gamelists, `es_input.cfg`), controller autoconfigs, `emulators.cfg`, `runcommand.cfg`. For `retroarch.cfg` and the core options, apply only the keys that differ from the fresh file (the keyboard bindings, `network_cmd_enable`, `autosave_interval`). Then `sudo chown -R pi:pi /opt/retropie/configs`.

**6. Tallgrass.** Install it (module or `install.sh`), `sudo systemctl stop tallgrass`, move the fresh database aside, copy the saved data in (not the binary), start it. Check `/api/cloud`, `/api/saves` and the Setup page.

**7. Bluetooth controller.** If `/var/lib/bluetooth` was restored (as root, with the same adapter address) pairings may survive. If not, and the controller won't pair: it remembers the Pi's old address. Either clear the controller's pairings (the method depends on the model), or give the Pi a new fixed address with a small service (see `CLAUDE.md`, "The Pi's system": `bt-address.service`, which sets it after `bluetooth.service` with `btmgmt power off` + `public-addr`, verifies with `hciconfig hci0`, and restarts bluetooth once). Pair with `bluetoothctl` (agent on, scan on, pair, trust, connect) while the controller is in pairing mode.

**8. Test live.** Start a RetroArch game and a DraStic game, and check `/api/party` shows the party, `/api/boxes` says `live`, and the Journey continued. That's the real proof; a green Setup page isn't.
