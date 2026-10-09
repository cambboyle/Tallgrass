"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { createClient } from "@/lib/supabase/client";

// Enter the code a Tallgrass device shows to link it to this account.
export default function LinkForm() {
  const params = useSearchParams();
  const [code, setCode] = useState(params.get("code") ?? "");
  const [signedIn, setSignedIn] = useState<boolean | null>(null);
  const [state, setState] = useState<{ busy?: boolean; done?: string; error?: string }>({});

  useEffect(() => {
    createClient().auth.getSession().then(({ data }) => setSignedIn(!!data.session));
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setState({ busy: true });
    const supabase = createClient();
    const { data: session } = await supabase.auth.getSession();
    const res = await fetch(`${process.env.NEXT_PUBLIC_SUPABASE_URL}/functions/v1/pair-claim`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${session.session?.access_token ?? ""}` },
      body: JSON.stringify({ code }),
    });
    const body = await res.json().catch(() => ({}));
    if (res.ok) setState({ done: body.deviceName });
    else setState({ error: body.error ?? "Something went wrong. Try again." });
  }

  if (signedIn === false) {
    const next = `/link${code ? `?code=${encodeURIComponent(code)}` : ""}`;
    return (
      <div className="card stack">
        <p>Sign in first, then enter your device&apos;s code.</p>
        <Link className="button primary" href={`/login?next=${encodeURIComponent(next)}`}>Sign in</Link>
      </div>
    );
  }
  if (state.done) {
    return (
      <div className="card stack">
        <h2>Linked</h2>
        <p><strong>{state.done}</strong> is now linked to your account. It will pick this up within a few seconds.</p>
        <Link className="button" href="/account">See your devices</Link>
      </div>
    );
  }
  return (
    <form className="card stack" onSubmit={submit}>
      <label htmlFor="code" className="small muted">Code shown on your device</label>
      <input id="code" className="code-input" type="text" autoComplete="off" spellCheck={false} maxLength={9}
        placeholder="ABCD-2345" value={code} onChange={(e) => setCode(e.target.value)} required />
      <button className="button primary" type="submit" disabled={state.busy || signedIn === null}>
        {state.busy ? "Linking…" : "Link device"}
      </button>
      {state.error && <p className="error">{state.error}</p>}
    </form>
  );
}
