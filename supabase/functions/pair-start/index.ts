// pair-start: the agent asks for a pairing code to show its user.
// POST {deviceName} -> {code, pollSecret, expiresAt}
import { admin, cors, error, json, pairingCode, randomToken, sha256 } from "../_shared/util.ts";

Deno.serve(async (req) => {
  if (req.method === "OPTIONS") return new Response(null, { headers: cors });
  if (req.method !== "POST") return error("Use POST", 405);
  const body = await req.json().catch(() => ({}));
  const deviceName = String(body.deviceName ?? "").trim().slice(0, 64) || "Tallgrass device";

  const db = admin();
  await db.from("pairing_codes").delete().lt("expires_at", new Date().toISOString());

  const pollSecret = randomToken();
  for (let attempt = 0; attempt < 5; attempt++) {
    const code = pairingCode();
    const { data, error: err } = await db.from("pairing_codes")
      .insert({ code, device_name: deviceName, poll_secret_hash: await sha256(pollSecret) })
      .select("expires_at").single();
    if (!err) return json({ code, pollSecret, expiresAt: data.expires_at });
    if (err.code !== "23505") return error("Couldn't create a pairing code", 500); // 23505: code taken
  }
  return error("Couldn't create a pairing code", 500);
});
