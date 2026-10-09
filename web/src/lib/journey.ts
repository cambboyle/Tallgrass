// Journey order: roughly when a player reaches each area, worked out from
// the game data. Nothing in a ROM lists story order, but levels rise as the
// game goes on, so an area's level is a good stand-in:
//
//   - trainers: the median of each trainer's highest level, leaving out
//     placeholder teams (see isPlaceholder)
//   - wild Pokémon: the median lowest level in the grass (caves count as
//     grass); fishing and surfing levels are flat across a game, so they
//     don't say when you get somewhere
//
// An area with trainers uses their level (wild levels in some regions are
// misleading: HeartGold keeps Kanto's grass at Johto-start levels); one
// without uses its grass. Towns with neither take the
// lowest level among the places they connect to. Anything still unknown
// goes last, alphabetically.

export type JourneyInput = {
  name: string;
  trainerLevels: number[]; // each trainer's highest party level
  wild: { method: string; minLevel: number }[];
  walksTo: string[]; // areas next to it (route edges)
  doorsTo: string[]; // areas reached through doors and warps
};

const quantile = (xs: number[], q: number) => {
  const s = [...xs].sort((a, b) => a - b);
  return s[Math.min(s.length - 1, Math.floor((s.length - 1) * q))];
};

export function journeyLevels(areas: JourneyInput[]): Map<string, number> {
  const level = new Map<string, number>();
  for (const a of areas) {
    const grass = a.wild.filter((w) => w.method === "Grass");
    if (a.trainerLevels.length) level.set(a.name, quantile(a.trainerLevels, 0.5));
    else if (grass.length) level.set(a.name, quantile(grass.map((w) => w.minLevel), 0.5));
  }
  // Fill in towns and buildings from their neighbours, a few steps out.
  for (let pass = 0; pass < 4; pass++) {
    const found = new Map<string, number>();
    for (const a of areas) {
      if (level.has(a.name)) continue;
      const known = (names: string[]) => names.map((n) => level.get(n)).filter((v): v is number => v !== undefined);
      const near = known(a.walksTo).length ? known(a.walksTo) : known(a.doorsTo);
      if (near.length) found.set(a.name, Math.min(...near));
    }
    if (!found.size) break;
    found.forEach((v, k) => level.set(k, v));
  }
  return level;
}

export function byJourney<T extends { name: string }>(items: T[], level: Map<string, number>): T[] {
  const collator = new Intl.Collator("en", { numeric: true });
  return [...items].sort((a, b) => {
    const la = level.get(a.name) ?? Infinity;
    const lb = level.get(b.name) ?? Infinity;
    return la - lb || collator.compare(a.name, b.name);
  });
}

// isPlaceholder spots unused filler trainers that hacks leave in the table
// and on maps: the exact same team under three or more different names.
export function placeholderTeams(trainers: { name: string; party: { species: number; level: number }[] }[]): Set<string> {
  const names = new Map<string, Set<string>>();
  for (const t of trainers) {
    const key = teamKey(t.party);
    if (!names.has(key)) names.set(key, new Set());
    names.get(key)!.add(t.name);
  }
  return new Set([...names].filter(([, n]) => n.size >= 3).map(([k]) => k));
}

export const teamKey = (party: { species: number; level: number }[]) => party.map((p) => `${p.species}@${p.level}`).join(",");
