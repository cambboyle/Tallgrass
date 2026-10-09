import Link from "next/link";
import { redirect } from "next/navigation";
import { createClient } from "@/lib/supabase/server";
import UnlinkButton from "@/components/UnlinkButton";
import SignOutButton from "@/components/SignOutButton";

export const metadata = { title: "Account" };

export default async function Account() {
  const supabase = await createClient();
  const { data: { user } } = await supabase.auth.getUser();
  if (!user) redirect("/login?next=/account");

  const [{ data: profile }, { data: devices }] = await Promise.all([
    supabase.from("profiles").select("display_name, is_maintainer").eq("id", user.id).single(),
    supabase.from("devices").select("id, name, created_at, last_seen_at").order("created_at"),
  ]);
  const when = (iso: string | null) => (iso ? new Date(iso).toLocaleString("en-GB", { dateStyle: "medium", timeStyle: "short" }) : "never");

  return (
    <main style={{ maxWidth: 720 }}>
      <div className="stack">
        <h1>{profile?.display_name || "Your account"}</h1>
        <p className="muted">{user.email}{profile?.is_maintainer ? " · maintainer" : ""}</p>
      </div>
      <section className="stack">
        <h3>Devices</h3>
        {devices?.length ? (
          <div className="table-wrap">
            <table>
              <thead><tr><th>Name</th><th>Linked</th><th>Last seen</th><th /></tr></thead>
              <tbody>
                {devices.map((d) => (
                  <tr key={d.id}>
                    <td>{d.name}</td>
                    <td className="small">{when(d.created_at)}</td>
                    <td className="small">{when(d.last_seen_at)}</td>
                    <td><UnlinkButton id={d.id} name={d.name} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="card stack">
            <p>No devices yet.</p>
            <Link className="button primary" href="/link">Link a device</Link>
          </div>
        )}
      </section>
      <div><SignOutButton /></div>
    </main>
  );
}
