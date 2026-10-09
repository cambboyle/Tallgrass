// Game data: exports uploaded from the Tallgrass agent (see
// docs/gamedata-export.md in the repo), stored as JSON in Supabase Storage.
import { createClient } from "@supabase/supabase-js";

export type Stats = { hp: number; attack: number; defense: number; speed: number; spAtk: number; spDef: number };
export type Evolution = { species: number; name: string; how: string };
export type Found = { area: string; method: string; when?: string; minLevel: number; maxLevel: number; chance: number };
export type Species = {
  species: number;
  name: string;
  types: string[];
  baseStats: Stats;
  abilities: string[];
  hiddenAbility?: string;
  evolvesTo: Evolution[];
  evolvesFrom: Evolution[];
  foundIn: Found[] | null;
  levelMoves: { level: number; move: string }[];
  tmMoves: { machine: string; move: string }[];
  tutorMoves: string[];
  eggMoves: string[];
};
export type TrainerMon = { species: number; speciesName: string; level: number; item?: string; moves?: string[] };
export type Trainer = {
  id: number;
  class: string;
  name: string;
  double?: boolean;
  items?: string[];
  party: TrainerMon[];
  areas?: string[];
  maps?: string[];
};
export type MapLink = { direction?: string; map: string; area: string };
export type GameMap = {
  id: string;
  area: string;
  connections?: MapLink[];
  warps?: MapLink[];
  items?: { item: string; quantity?: number; hidden?: boolean; x: number; y: number }[];
  shops?: string[][];
  trainers?: number[];
};
export type GameData = {
  schema: string;
  version: number;
  generatedAt: string;
  rom: { sha1: string; file: string; game: string; gameCode?: string; generation: number };
  species: Species[];
  trainers: Trainer[];
  maps: GameMap[];
};
export type Game = { slug: string; name: string; generation: number };

function db() {
  return createClient(process.env.NEXT_PUBLIC_SUPABASE_URL!, process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY!, {
    auth: { persistSession: false },
  });
}

export async function getGames(): Promise<Game[]> {
  const { data } = await db().from("games").select("slug, name, generation").order("generation").order("name");
  return data ?? [];
}

export async function getGame(slug: string): Promise<Game | null> {
  const { data } = await db().from("games").select("slug, name, generation").eq("slug", slug).maybeSingle();
  return data;
}

// Exports are a few MB; keep the latest of each game in memory per server
// instance, refreshed at most every 10 minutes.
const memo = new Map<string, { at: number; data: GameData | null; uploadedAt: string | null }>();

export async function getGameData(slug: string): Promise<{ data: GameData | null; uploadedAt: string | null }> {
  // Local development: GAMEDATA_DIR holds <slug>.json exports.
  if (process.env.GAMEDATA_DIR) {
    const { readFile } = await import("node:fs/promises");
    try {
      const text = await readFile(`${process.env.GAMEDATA_DIR}/${slug}.json`, "utf8");
      return { data: JSON.parse(text) as GameData, uploadedAt: null };
    } catch {
      return { data: null, uploadedAt: null };
    }
  }
  const hit = memo.get(slug);
  if (hit && Date.now() - hit.at < 10 * 60 * 1000) return hit;
  const client = db();
  const { data: row } = await client
    .from("game_data")
    .select("storage_path, created_at")
    .eq("game_slug", slug)
    .order("created_at", { ascending: false })
    .limit(1)
    .maybeSingle();
  let data: GameData | null = null;
  if (row) {
    const url = client.storage.from("gamedata").getPublicUrl(row.storage_path).data.publicUrl;
    const res = await fetch(url);
    if (res.ok) data = (await res.json()) as GameData;
  }
  const entry = { at: Date.now(), data, uploadedAt: row?.created_at ?? null };
  memo.set(slug, entry);
  return entry;
}

export const statTotal = (s: Stats) => s.hp + s.attack + s.defense + s.speed + s.spAtk + s.spDef;
