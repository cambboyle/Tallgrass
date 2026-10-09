// pair-poll: the agent checks whether its code has been entered, and
// collects its device key once (the code is deleted afterwards).
// POST {code, pollSecret} -> {status: "pending" | "expired" | "linked", token?, deviceId?}
import { admin, cors, error, json, normaliseCode, sha256 } from "../_shared/util.ts";

Deno.serve(async (req) => {
  if (req.method === "OPTIONS") return new Response(null, { headers: cors });
  if (req.method !== "POST") return error("Use POST", 405);
  const body = await req.json().catch(() => ({}));
  const code = normaliseCode(body.code);
  const secret = String(body.pollSecret ?? "");

  const db = admin();
  const { data: pairing } = await db.from("pairing_codes").select("*").eq("code", code).maybeSingle();
  if (!pairing || pairing.poll_secret_hash !== await sha256(secret)) return json({ status: "expired" });
  if (pairing.device_token) {
    await db.from("pairing_codes").delete().eq("code", code);
    return json({ status: "linked", token: pairing.device_token, deviceId: pairing.device_id });
  }
  if (new Date(pairing.expires_at) < new Date()) return json({ status: "expired" });
  return json({ status: "pending" });
});
