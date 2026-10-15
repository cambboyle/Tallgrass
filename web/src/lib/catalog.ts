// The known-hacks catalogue (catalog/hacks.json in the repository): who made
// each romhack, which base game it needs, the official page, and the SHA-1 of
// the build Tallgrass supports. Read from GitHub, like the docs pages, so a
// change on main shows within 10 minutes.

const RAW = "https://raw.githubusercontent.com/cambboyle/Tallgrass/main/catalog/hacks.json";

export type Base = {
  slug: string;
  name: string;
  system: "gba" | "nds";
  gameCode?: string;
  detail?: string;
  sha1?: string[];
  crc32?: string;
};

export type Hack = {
  slug: string;
  name: string;
  version?: string;
  author: string;
  system: "gba" | "nds";
  base: string;
  baseNote?: string;
  source: { label: string; url: string };
  result: { sha1: string[] };
  support: string;
};

export type Catalog = { bases: Base[]; hacks: Hack[] };

export const systemName = (s: string) => (s === "nds" ? "Nintendo DS" : "Game Boy Advance");

export async function getCatalog(): Promise<Catalog | null> {
  try {
    const res = await fetch(RAW, { next: { revalidate: 600 } });
    if (!res.ok) return null;
    const data = await res.json();
    if (data?.schema !== "tallgrass.catalog" || data?.version !== 1) return null;
    return { bases: data.bases ?? [], hacks: data.hacks ?? [] };
  } catch {
    return null;
  }
}
