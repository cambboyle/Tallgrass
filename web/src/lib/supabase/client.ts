import { createBrowserClient } from "@supabase/ssr";

// Browser client: signs in, calls Edge Functions as the user.
export function createClient() {
  return createBrowserClient(process.env.NEXT_PUBLIC_SUPABASE_URL!, process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY!);
}
