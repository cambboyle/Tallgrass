// backup-upload: a linked device that switched cloud sync on uploads a save
// backup or its Journey history. Authorization: Bearer <device key>.
//   X-Kind: save     X-File: gba/Game.srm (percent-encoded)   X-Version: 20261011T120000Z-ab12cd34ef56.srm
//   X-Kind: journey  (body: JSON)
// Saves go to backups/<user>/<device>/<X-File>/<X-Version>; the newest 30
// versions of each save are kept.
import { admin, cors, deviceFromRequest, error, json } from "../_shared/util.ts";

const KEEP = 30;
const MAX = 4 * 1024 * 1024;
const safe = (s: string) => /^[^/\\]+$/.test(s) && s !== "." && s !== ".." && s.length <= 200;

// Storage keys allow only some ASCII characters, so a name with anything
// else ("Pokémon …") is stored as "u-" + base64url of its UTF-8 bytes; the
// website decodes it back for display.
const plain = /^[A-Za-z0-9 ._()\-!']+$/;
function storageName(name: string): string {
  if (plain.test(name) && !name.startsWith("u-")) return name;
  const bytes = new TextEncoder().encode(name);
  return "u-" + btoa(String.fromCharCode(...bytes)).replaceAll("+", "-").replaceAll("/", "_").replaceAll("=", "");
}

Deno.serve(async (req) => {
  if (req.method === "OPTIONS") return new Response(null, { headers: cors });
  if (req.method !== "POST") return error("Use POST", 405);
  const db = admin();
  const device = await deviceFromRequest(db, req);
  if (!device) return error("This device isn't linked", 401);

  const body = new Uint8Array(await req.arrayBuffer());
  if (body.length === 0 || body.length > MAX) return error("Empty or too big", 413);
  const root = `${device.user_id}/${device.id}`;
  const kind = req.headers.get("x-kind");

  if (kind === "journey") {
    const { error: upErr } = await db.storage.from("backups").upload(`${root}/journey.json`, body, {
      contentType: "application/json",
      upsert: true,
    });
    if (upErr) return error("Couldn't store the journey", 500);
    return json({ stored: "journey.json" });
  }

  if (kind !== "save") return error("X-Kind must be save or journey");
  // X-File is percent-encoded (headers can't carry UTF-8 safely).
  let file: string[];
  try {
    file = (req.headers.get("x-file") ?? "").split("/").map(decodeURIComponent);
  } catch {
    return error("Bad file name");
  }
  const version = req.headers.get("x-version") ?? "";
  if (file.length !== 2 || !file.every(safe) || !safe(version)) return error("Bad file or version");
  const folder = `${root}/${file.map(storageName).join("/")}`;
  const { error: upErr } = await db.storage.from("backups").upload(`${folder}/${version}`, body, {
    contentType: "application/octet-stream",
    upsert: true,
  });
  if (upErr) return error("Couldn't store the save", 500);

  // Versions are named by UTC time first, so name order is age order.
  const { data: list } = await db.storage.from("backups").list(folder, { limit: 1000, sortBy: { column: "name", order: "desc" } });
  const old = (list ?? []).slice(KEEP).map((o) => `${folder}/${o.name}`);
  if (old.length) await db.storage.from("backups").remove(old);
  return json({ stored: `${file.join("/")}/${version}`, pruned: old.length });
});
