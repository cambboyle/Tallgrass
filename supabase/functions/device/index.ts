// device: the agent checks its link. GET with Authorization: Bearer <device key>
// -> {deviceId, name, account: {displayName, isMaintainer}}
import { admin, cors, deviceFromRequest, error, json } from "../_shared/util.ts";

Deno.serve(async (req) => {
  if (req.method === "OPTIONS") return new Response(null, { headers: cors });
  const db = admin();
  const device = await deviceFromRequest(db, req);
  if (!device) return error("This device isn't linked", 401);
  const { data: profile } = await db.from("profiles").select("display_name, is_maintainer").eq("id", device.user_id).single();
  return json({
    deviceId: device.id,
    name: device.name,
    account: { displayName: profile?.display_name ?? "", isMaintainer: profile?.is_maintainer ?? false },
  });
});
