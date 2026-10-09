// Shared helpers for Tallgrass Edge Functions.
import { createClient, type SupabaseClient } from "npm:@supabase/supabase-js@2";

export const cors = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Headers": "authorization, apikey, content-type, x-client-info",
  "Access-Control-Allow-Methods": "POST, GET, OPTIONS",
};

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { ...cors, "Content-Type": "application/json" },
  });
}

export function error(message: string, status = 400): Response {
  return json({ error: message }, status);
}

// admin bypasses row level security; only use it after checking the caller.
export function admin(): SupabaseClient {
  const secrets = Deno.env.get("SUPABASE_SECRET_KEYS");
  const key = secrets ? JSON.parse(secrets)["default"] : Deno.env.get("SUPABASE_SERVICE_ROLE_KEY")!;
  return createClient(Deno.env.get("SUPABASE_URL")!, key, { auth: { persistSession: false } });
}

export async function sha256(text: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, "0")).join("");
}

// randomToken returns n random bytes as base64url text.
export function randomToken(n = 32): string {
  const bytes = crypto.getRandomValues(new Uint8Array(n));
  return btoa(String.fromCharCode(...bytes)).replaceAll("+", "-").replaceAll("/", "_").replaceAll("=", "");
}

// Pairing codes avoid look-alike characters (0/O, 1/I/L) and read as XXXX-XXXX.
const codeChars = "ABCDEFGHJKMNPQRSTUVWXYZ23456789";
export function pairingCode(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(8));
  const chars = Array.from(bytes, (b) => codeChars[b % codeChars.length]).join("");
  return `${chars.slice(0, 4)}-${chars.slice(4)}`;
}

export function normaliseCode(input: unknown): string {
  const s = String(input ?? "").toUpperCase().replace(/[^A-Z0-9]/g, "");
  return s.length === 8 ? `${s.slice(0, 4)}-${s.slice(4)}` : "";
}

export function bearer(req: Request): string {
  const h = req.headers.get("authorization") ?? "";
  return h.toLowerCase().startsWith("bearer ") ? h.slice(7).trim() : "";
}

// deviceFromRequest checks an agent's device key (Authorization: Bearer).
export async function deviceFromRequest(db: SupabaseClient, req: Request) {
  const token = bearer(req);
  if (!token) return null;
  const { data } = await db.from("devices").select("id, name, user_id").eq("token_hash", await sha256(token)).maybeSingle();
  if (data) {
    await db.from("devices").update({ last_seen_at: new Date().toISOString() }).eq("id", data.id);
  }
  return data;
}
