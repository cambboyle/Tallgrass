# Game data export (format version 1)

The agent can export what it reads from a ROM as one JSON file: species, their moves and evolutions, where to find them, and every trainer battle. The docs site (ROADMAP M4) will build its per-game pages from these files.

Only facts read from the user's own ROM are included: numbers, names and lists. Sprites, music and other assets are never exported (ROADMAP principle 1).

## Getting a file

- **Dashboard:** Library → a game's **Export data** link, or `GET /api/export?rom=<first 12 hex digits of the ROM's SHA-1>`.
- **Command line:** `tallgrass -export path/to/game.gba > game.json`. This needs no emulator and works on any machine.

## Shape

```jsonc
{
  "schema": "tallgrass.gamedata",
  "version": 1,
  "generatedAt": "2026-10-09T20:02:00Z",
  "rom": {
    "sha1": "964f951a…",          // the key: one file per exact ROM
    "file": "Radical Red.gba",
    "game": "Pokémon Radical Red", // profile name, or the DS game family
    "gameCode": "IPKE",            // DS only
    "generation": 3
  },
  "species": [ /* Species */ ],
  "trainers": [ /* Trainer */ ],
  "maps": [ /* Map */ ]
}
```

### Species

| Field | Meaning |
|---|---|
| `species` | The game's own species ID. Hacks renumber, so IDs only mean something within one `rom.sha1`. |
| `name`, `types`, `abilities`, `hiddenAbility` | As the game names them. |
| `baseStats` | `hp`, `attack`, `defense`, `speed`, `spAtk`, `spDef`. |
| `evolvesTo`, `evolvesFrom` | `{species, name, how}`; `how` is readable text ("Level 16", "Use Fire Stone"). |
| `foundIn` | `{area, method, when, minLevel, maxLevel, chance}`. `when` is a time of day, or empty for all day. `chance` is 0 where the game's odds aren't known (method "Hidden (DexNav)"). **Missing (null)** means this game's maps can't be read yet (Platinum); an empty list means not found in the wild. |
| `levelMoves` | `{level, move}`; level 0 means "on evolution". |
| `tmMoves` | `{machine, move}`, e.g. `{"machine": "HM03", "move": "Surf"}`. |
| `tutorMoves`, `eggMoves` | Move names. |

Unused species slots (blank or placeholder names) are left out.

### Trainer

| Field | Meaning |
|---|---|
| `id` | The game's trainer ID. Rematches and alternate teams are separate trainers with the same class and name. |
| `class`, `name` | E.g. "Leader", "Brock". |
| `double` | A double battle. |
| `items` | Items the trainer uses in battle. |
| `party` | `{species, speciesName, level, item, moves}`. Empty `moves` means the Pokémon uses its level-up moves. `level` 0 means the game sets it when the battle starts (R.O.W.E. scales trainers to the player); there `moves` is a pool of up to six the game picks from. |
| `otherParty` | Optional: a second team the hack gives this trainer (R.O.W.E.), same shape as `party`; when the game uses it isn't known. |
| `areas`, `maps` | Where the trainer is battled, as area names and map IDs. Absent when unknown (battle facilities, some scripted fights, rematch-only teams). |

### Map

| Field | Meaning |
|---|---|
| `id` | `"group.num"` in Gen 3; `"0.<map ID>"` in DS games, which number maps in one list. |
| `area` | The location name shown in game. Several maps (a town and its buildings) share one. |
| `connections` | Gen 3 only: `{direction, map, area}` for walking off an edge (`north`, `south`, `east`, `west`, `dive`, `emerge`). DS maps join through the region grid instead, which isn't read. |
| `warps` | `{map, area}`: where doors, stairs and cave entrances lead, one per destination. |
| `items` | `{item, quantity, hidden, x, y}`: item balls and hidden items. Quantity is left out when it's 1. |
| `shops` | Each shop's stock (item names), in the clerk's order. |
| `shopBadges` | DS only, when a map has a standard Poké Mart: per shop, per item, the badges needed before the clerk sells it (`null` for shops whose stock never changes). The game counts Johto and Kanto badges together. |
| `trainers` | Trainer IDs battled on this map. |

## Versioning

- `version` changes only when a field changes meaning or is removed. New fields can appear in any version; readers should ignore fields they don't know.
- The same ROM and agent version always produce the same data, apart from `generatedAt`.
