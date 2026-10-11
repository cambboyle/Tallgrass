# Tallgrass

A companion for Pokémon romhacks played on your own hardware. It runs next to your emulator on a Raspberry Pi (RetroPie) or Linux PC and gives you, on your phone or computer:

- your party live: stats, IVs, EVs, moves
- what you can catch where you're standing, by time of day, with caught/seen marks
- trainers, items and shops on the current map
- an automatic journey log (or Nuzlocke tracker)
- versioned backups of every save, and a box viewer
- docs for each supported game, generated from the game itself: https://tallgrass-delta.vercel.app

Supported games: **Pokémon Radical Red**, **Pokémon Unbound** (RetroArch with mGBA) and **HeartGold Generations** (DraStic). Tallgrass never ships game data; it reads everything from your own copy of the game.

## Install

On the Pi (or Linux PC), with your games already set up:

```bash
curl -fsSL https://tallgrass-delta.vercel.app/install.sh | bash
```

Then open `http://<your-pi>:8080/setup.html` and pick your game. Tallgrass keeps itself up to date.

## Development

- `go test ./...`, `make deploy` (to the Pi in `Makefile`), `make run` (on a Mac, talking to the Pi).
- `CLAUDE.md` has the detailed notes on game formats and the codebase; `ROADMAP.md` has the plan.
- Releases: `docs/releasing.md`. Website: `web/README.md`.
- Adding a game: [docs/writing-a-profile.md](docs/writing-a-profile.md). Try a profile without rebuilding by putting it in `~/tallgrass/profiles/`.

## Licence

- The Tallgrass code (agent, website, server functions) is licensed under the **GNU General Public License v3.0**: see [LICENSE](LICENSE).
- Game profiles (`profiles/*.json`: memory addresses and table locations) are dedicated to the public domain under **CC0 1.0**: see [profiles/LICENSE](profiles/LICENSE).
- Tallgrass is a fan project, not affiliated with Nintendo, Game Freak, The Pokémon Company or the authors of the romhacks it supports. It contains no game data; everything it shows is read from the player's own copy of the game.
