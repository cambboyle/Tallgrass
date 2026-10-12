import type { GameData, Trainer } from "./gamedata";
import { placeholderTeams, teamKey } from "./journey";

// The story's big fights, picked out of the trainer list by class name:
// gym leaders, the Elite Four and champions, rivals, and villain bosses.
// A ROM has no "first fight" flag, and games keep several teams per boss
// (rematches, difficulty modes), so teams are grouped by name and listed
// lowest level first.

export const CATEGORIES = ["Gym leaders", "Elite Four & champions", "Rivals", "Villain teams and other bosses"] as const;
export type Category = (typeof CATEGORIES)[number];

export type BossGroup = {
  key: string;
  class: string;
  name: string;
  teams: Trainer[]; // lowest top level first
  areas: string[];
};

export function category(cls: string): Category | null {
  const c = cls.toLowerCase();
  if (/elite four|champion/.test(c)) return "Elite Four & champions";
  if (/rival/.test(c)) return "Rivals";
  if (/^(gym )?leader$/.test(c)) return "Gym leaders";
  if (/leader|admin|boss/.test(c)) return "Villain teams and other bosses";
  return null;
}

const top = (t: Trainer) => Math.max(0, ...t.party.map((p) => p.level));

// bosses groups an export's key trainers by category. Within a category,
// bosses follow the order of their lowest-level team, or, where levels are
// set at battle time (R.O.W.E.), the order of the trainer table, which
// follows the story there.
export function bosses(data: GameData): { category: Category; groups: BossGroup[] }[] {
  const fake = placeholderTeams(data.trainers);
  const byKey = new Map<string, { cat: Category; group: BossGroup }>();
  for (const t of data.trainers) {
    const cat = category(t.class);
    if (!cat || !t.name.trim() || !t.party.length || fake.has(teamKey(t.party))) continue;
    const key = `${t.class}|${t.name}`;
    let e = byKey.get(key);
    if (!e) byKey.set(key, (e = { cat, group: { key, class: t.class, name: t.name, teams: [], areas: [] } }));
    e.group.teams.push(t);
    for (const a of t.areas ?? []) if (!e.group.areas.includes(a)) e.group.areas.push(a);
  }
  const first = (g: BossGroup) => Math.min(...g.teams.map((t) => t.id));
  const low = (g: BossGroup) => {
    const levels = g.teams.map(top).filter((l) => l > 0);
    return levels.length ? Math.min(...levels) : Infinity;
  };
  return CATEGORIES.map((category) => {
    const groups = [...byKey.values()].filter((e) => e.cat === category).map((e) => e.group);
    for (const g of groups) g.teams.sort((a, b) => top(a) - top(b) || a.id - b.id);
    groups.sort((a, b) => low(a) - low(b) || first(a) - first(b));
    return { category, groups };
  }).filter((c) => c.groups.length);
}

// "Potion, Potion, Super Potion" → "Potion ×2, Super Potion"
export function itemList(items: string[]): string {
  const counts = new Map<string, number>();
  for (const i of items) counts.set(i, (counts.get(i) ?? 0) + 1);
  return [...counts].map(([name, n]) => (n > 1 ? `${name} ×${n}` : name)).join(", ");
}
