// gamedata-upload: a maintainer's agent uploads a game data export
// (docs/gamedata-export.md) for one of Tallgrass's supported games.
// POST the export JSON with Authorization: Bearer <device key> -> {game, path}
import { admin, cors, deviceFromRequest, error, json } from "../_shared/util.ts";

const SUPPORTED_EXPORT_VERSIONS = [1];

Deno.serve(async (req) => {
  if (req.method === "OPTIONS") return new Response(null, { headers: cors });
  if (req.method !== "POST") return error("Use POST", 405);
  const db = admin();
  const device = await deviceFromRequest(db, req);
  if (!device) return error("This device isn't linked", 401);
  const { data: profile } = await db.from("profiles").select("is_maintainer").eq("id", device.user_id).single();
  if (!profile?.is_maintainer) return error("Only maintainers can upload game data", 403);

  const text = await req.text();
  let data;
  try {
    data = JSON.parse(text);
  } catch {
    return error("That isn't JSON");
  }
  if (data?.schema !== "tallgrass.gamedata" || !SUPPORTED_EXPORT_VERSIONS.includes(data?.version)) {
    return error("That isn't a Tallgrass game data export this server understands");
  }
  const sha1 = String(data?.rom?.sha1 ?? "").toLowerCase();
  const { data: game } = await db.from("games").select("slug").contains("rom_sha1", [sha1]).maybeSingle();
  if (!game) return error("That ROM isn't one of Tallgrass's supported games", 422);

  const path = `${game.slug}/${sha1}/${new Date().toISOString().replace(/[:.]/g, "-")}.json`;
  const { error: upErr } = await db.storage.from("gamedata").upload(path, text, { contentType: "application/json" });
  if (upErr) return error("Couldn't store the file", 500);

  const { error: rowErr } = await db.from("game_data").insert({
    game_slug: game.slug,
    rom_sha1: sha1,
    export_version: data.version,
    storage_path: path,
    species_count: data.species?.length ?? 0,
    trainer_count: data.trainers?.length ?? 0,
    map_count: data.maps?.length ?? 0,
    uploaded_by: device.user_id,
  });
  if (rowErr) return error("Couldn't record the upload", 500);
  return json({ game: game.slug, path });
});
