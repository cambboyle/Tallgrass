import type { GameData, Trainer } from "./gamedata";
import { journeyLevels, placeholderTeams, teamKey } from "./journey";
import { slugify } from "./types";

export type Encounter = { species: number; name: string; method: string; when?: string; minLevel: number; maxLevel: number; chance: number };
export type Area = {
  slug: string;
  name: string;
  encounters: Encounter[];
  items: { item: string; quantity?: number; hidden?: boolean }[];
  shops: string[][];
  trainers: Trainer[];
  leadsTo: string[]; // other areas reachable by walking or a door
  walksTo: string[]; // just the walking ones (route edges)
  level: number | null; // journey order (see journey.ts); null if unknown
};

// Maps the game doesn't name come out as "Map 43.1"; they sort last, and so
// do battle facilities, whose trainers' levels are set when you battle.
export const unnamed = (name: string) => /^Map \d+\.\d+$/.test(name);
const facility = (name: string) =>
  /battle (frontier|tower|factory|hall|castle|arcade|pike|palace|dome|pyramid|tent)|trainer tower|fight area/i.test(name);

// areas groups everything in an export by place name. Many maps share a
// name (a town and its buildings), so they merge.
export function areas(data: GameData): Area[] {
  const by = new Map<string, Area>();
  const get = (name: string) => {
    const slug = slugify(name);
    if (!by.has(slug)) by.set(slug, { slug, name, encounters: [], items: [], shops: [], trainers: [], leadsTo: [], walksTo: [], level: null });
    return by.get(slug)!;
  };
  for (const s of data.species) {
    for (const f of s.foundIn ?? []) {
      get(f.area).encounters.push({ species: s.species, name: s.name, method: f.method, when: f.when, minLevel: f.minLevel, maxLevel: f.maxLevel, chance: f.chance });
    }
  }
  for (const m of data.maps) {
    const a = get(m.area);
    a.items.push(...(m.items ?? []));
    for (const shop of m.shops ?? []) {
      if (!a.shops.some((s) => s.join() === shop.join())) a.shops.push(shop);
    }
    for (const l of [...(m.connections ?? []), ...(m.warps ?? [])]) {
      if (l.area !== m.area && !a.leadsTo.includes(l.area)) a.leadsTo.push(l.area);
    }
    for (const l of m.connections ?? []) {
      if (l.area !== m.area && !a.walksTo.includes(l.area)) a.walksTo.push(l.area);
    }
  }
  for (const t of data.trainers) {
    for (const name of t.areas ?? []) get(name).trainers.push(t);
  }
  for (const a of by.values()) {
    a.encounters.sort((x, y) => x.method.localeCompare(y.method) || y.chance - x.chance);
  }

  const fake = placeholderTeams(data.trainers);
  const levels = journeyLevels(
    [...by.values()].filter((a) => !unnamed(a.name)).map((a) => ({
      name: a.name,
      trainerLevels: a.trainers.filter((t) => !fake.has(teamKey(t.party))).map((t) => Math.max(...t.party.map((p) => p.level))),
      wild: a.encounters.map((e) => ({ method: e.method, minLevel: e.minLevel })),
      walksTo: a.walksTo,
      doorsTo: a.leadsTo,
    })),
  );
  for (const a of by.values()) a.level = facility(a.name) ? null : (levels.get(a.name) ?? null);
  return [...by.values()].filter((a) => a.encounters.length || a.items.length || a.trainers.length || a.shops.length);
}
