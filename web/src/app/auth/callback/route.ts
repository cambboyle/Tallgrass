import { NextResponse, type NextRequest } from "next/server";
import { createClient } from "@/lib/supabase/server";

// Finishes OAuth and email sign-in: swaps the one-time code for a session.
export async function GET(request: NextRequest) {
  const url = new URL(request.url);
  const code = url.searchParams.get("code");
  const next = url.searchParams.get("next") ?? "/account";
  // Only ever send people back to this site: resolve the target and check
  // where it actually points ("/\\evil.com" looks local but isn't).
  let dest = new URL("/account", url.origin);
  if (!/[\\\u0000-\u001f]/.test(next)) {
    try {
      const candidate = new URL(next, url.origin);
      if (candidate.origin === url.origin) dest = candidate;
    } catch {
      // keep the default
    }
  }
  if (code) {
    const supabase = await createClient();
    const { error } = await supabase.auth.exchangeCodeForSession(code);
    if (!error) return NextResponse.redirect(dest);
  }
  return NextResponse.redirect(new URL("/login?error=1", url.origin));
}
