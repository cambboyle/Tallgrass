// profiles-sync: the repo's CI sends every profile (profiles/*.json) after a
// push to main; this upserts them into game_profiles, bumping a profile's
// version when its content changed, and removes profiles no longer in the
// repo. POST [{slug, profile}] with X-Sync-Secret: <PROFILES_SYNC_SECRET>.
import { admin, cors, error, json } from "../_shared/util.ts";

function sameJSON(a: unknown, b: unknown): boolean {
  const canon = (v: unknown): string =>
    JSON.stringify(v, (_k, x) =>
      x && typeof x === "object" && !Array.isArray(x)
        ? Object.fromEntries(Object.entries(x).sort(([p], [q]) => p.localeCompare(q)))
        : x);
  return canon(a) === canon(b);
}

Deno.serve(async (req) => {
  if (req.method === "OPTIONS") return new Response(null, { headers: cors });
  if (req.method !== "POST") return error("Use POST", 405);
  const secret = Deno.env.get("PROFILES_SYNC_SECRET");
  if (!secret || req.headers.get("x-sync-secret") !== secret) return error("Not allowed", 401);

  let items: { slug: string; profile: Record<string, unknown> }[];
  try {
    items = await req.json();
  } catch {
    return error("That isn't JSON");
  }
  if (!Array.isArray(items) || items.length === 0) return error("Expected a list of profiles");
  for (const it of items) {
    if (!/^[a-z0-9-]+$/.test(String(it?.slug)) || typeof it?.profile !== "object" || !it.profile?.name) {
      return error(`Bad profile entry: ${JSON.stringify(it?.slug)}`);
    }
  }

  const db = admin();
  const { data: existing, error: readErr } = await db.from("game_profiles").select("slug, version, profile");
  if (readErr) return error("Couldn't read the registry", 500);
  const bySlug = new Map((existing ?? []).map((r) => [r.slug, r]));

  const changed: string[] = [];
  for (const { slug, profile } of items) {
    const old = bySlug.get(slug);
    if (old && sameJSON(old.profile, profile)) continue;
    const row = {
      slug,
      name: String(profile.name),
      rom_sha1: Array.isArray(profile.romSha1) ? profile.romSha1.map(String) : [],
      version: (old?.version ?? 0) + 1,
      profile,
      updated_at: new Date().toISOString(),
    };
    const { error: upErr } = await db.from("game_profiles").upsert(row);
    if (upErr) return error(`Couldn't store ${slug}`, 500);
    changed.push(slug);
  }

  const keep = new Set(items.map((i) => i.slug));
  const removed = (existing ?? []).map((r) => r.slug).filter((s) => !keep.has(s));
  if (removed.length) {
    const { error: delErr } = await db.from("game_profiles").delete().in("slug", removed);
    if (delErr) return error("Couldn't remove old profiles", 500);
  }
  return json({ changed, removed, total: items.length });
});
