// pair-claim: a signed-in user enters the code their agent shows.
// POST {code} with the user's session -> {deviceName}
import { admin, bearer, cors, error, json, normaliseCode, randomToken, sha256 } from "../_shared/util.ts";

Deno.serve(async (req) => {
  if (req.method === "OPTIONS") return new Response(null, { headers: cors });
  if (req.method !== "POST") return error("Use POST", 405);

  const db = admin();
  const { data: auth } = await db.auth.getUser(bearer(req));
  if (!auth?.user) return error("Sign in first", 401);

  const body = await req.json().catch(() => ({}));
  const code = normaliseCode(body.code);
  if (!code) return error("That isn't a pairing code. It looks like ABCD-2345.");

  const { data: pairing } = await db.from("pairing_codes").select("*").eq("code", code).maybeSingle();
  if (!pairing || new Date(pairing.expires_at) < new Date()) {
    return error("That code has expired or doesn't exist. Get a new one from your device.", 404);
  }
  if (pairing.claimed_by) return error("That code has already been used.", 409);

  const token = randomToken();
  const { data: device, error: devErr } = await db.from("devices")
    .insert({ user_id: auth.user.id, name: pairing.device_name, token_hash: await sha256(token) })
    .select("id").single();
  if (devErr) return error("Couldn't link the device", 500);

  await db.from("pairing_codes")
    .update({ claimed_by: auth.user.id, device_id: device.id, device_token: token })
    .eq("code", code);
  return json({ deviceName: pairing.device_name });
});
