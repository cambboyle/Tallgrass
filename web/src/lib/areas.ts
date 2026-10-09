import type { GameData, Trainer } from "./gamedata";
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
};

// areas groups everything in an export by place name. Many maps share a
// name (a town and its buildings), so they merge.
export function areas(data: GameData): Area[] {
  const by = new Map<string, Area>();
  const get = (name: string) => {
    const slug = slugify(name);
    if (!by.has(slug)) by.set(slug, { slug, name, encounters: [], items: [], shops: [], trainers: [], leadsTo: [] });
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
  }
  for (const t of data.trainers) {
    for (const name of t.areas ?? []) get(name).trainers.push(t);
  }
  for (const a of by.values()) {
    a.encounters.sort((x, y) => x.method.localeCompare(y.method) || y.chance - x.chance);
  }
  return [...by.values()].filter((a) => a.encounters.length || a.items.length || a.trainers.length || a.shops.length);
}
