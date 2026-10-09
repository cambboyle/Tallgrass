"use client";

import { useState } from "react";
import { useSearchParams } from "next/navigation";
import { createClient } from "@/lib/supabase/client";

export default function LoginForm() {
  const next = useSearchParams().get("next") ?? "/account";
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [error, setError] = useState("");
  const redirectTo = () => `${window.location.origin}/auth/callback?next=${encodeURIComponent(next)}`;

  async function oauth(provider: "discord" | "github") {
    setError("");
    const { error } = await createClient().auth.signInWithOAuth({ provider, options: { redirectTo: redirectTo() } });
    if (error) setError(error.message);
  }

  async function magicLink(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    const { error } = await createClient().auth.signInWithOtp({ email, options: { emailRedirectTo: redirectTo() } });
    if (error) setError(error.message);
    else setSent(true);
  }

  return (
    <main style={{ maxWidth: 440 }}>
      <div className="stack">
        <h1>Sign in</h1>
        <p className="muted">One account for your devices, backups and anything you contribute to the docs.</p>
      </div>
      <div className="card stack">
        <button className="button primary" onClick={() => oauth("discord")}>Continue with Discord</button>
        <button className="button" onClick={() => oauth("github")}>Continue with GitHub</button>
      </div>
      <form className="card stack" onSubmit={magicLink}>
        <label htmlFor="email" className="small muted">Or get a sign-in link by email</label>
        <input id="email" type="email" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@example.com" />
        <button className="button" type="submit" disabled={sent}>{sent ? "Link sent: check your email" : "Email me a link"}</button>
      </form>
      {error && <p className="error">{error}</p>}
    </main>
  );
}
