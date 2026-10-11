import type { SupabaseClient } from "@supabase/supabase-js";

// The signed-in user's cloud backups (storage bucket "backups", folder per
// user and device): each save with its versions, and the Journey history.
// Links are signed for an hour.

type Device = { id: string; name: string };
type Version = { name: string; at: Date | null; url: string | null };
type Save = { file: string; versions: Version[] };

// Versions are named "<UTC time>-<hash prefix><ext>", e.g. 20261011T120000Z-ab12cd34ef56.srm.
function versionTime(name: string): Date | null {
  const m = /^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z/.exec(name);
  return m ? new Date(Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4], +m[5], +m[6])) : null;
}

const when = (d: Date | null) => (d ? d.toLocaleString("en-GB", { dateStyle: "medium", timeStyle: "short", timeZone: "UTC" }) + " UTC" : "");

async function deviceBackups(supabase: SupabaseClient, userId: string, device: Device) {
  const bucket = supabase.storage.from("backups");
  const root = `${userId}/${device.id}`;
  const { data: top } = await bucket.list(root, { limit: 100 });
  const saves: Save[] = [];
  let journey: string | null = null;
  for (const entry of top ?? []) {
    if (entry.name === "journey.json") {
      const { data } = await bucket.createSignedUrl(`${root}/journey.json`, 3600, { download: "tallgrass-journey.json" });
      journey = data?.signedUrl ?? null;
      continue;
    }
    // A system folder ("gba", "nds") of save folders.
    const { data: files } = await bucket.list(`${root}/${entry.name}`, { limit: 100 });
    for (const f of files ?? []) {
      const folder = `${root}/${entry.name}/${f.name}`;
      const { data: versions } = await bucket.list(folder, { limit: 100, sortBy: { column: "name", order: "desc" } });
      const names = (versions ?? []).map((v) => v.name);
      const { data: signed } = names.length ? await bucket.createSignedUrls(names.map((n) => `${folder}/${n}`), 3600) : { data: [] };
      saves.push({
        file: `${entry.name}/${f.name}`,
        versions: names.map((n, i) => ({ name: n, at: versionTime(n), url: signed?.[i]?.signedUrl ?? null })),
      });
    }
  }
  return { device, saves, journey };
}

export default async function CloudBackups({ supabase, userId, devices }: { supabase: SupabaseClient; userId: string; devices: Device[] }) {
  const all = await Promise.all(devices.map((d) => deviceBackups(supabase, userId, d)));
  const withData = all.filter((d) => d.saves.length || d.journey);
  return (
    <section className="stack">
      <h3>Cloud backups</h3>
      {withData.length === 0 ? (
        <p className="muted small">
          Nothing here yet. Switch on <b>Cloud backups</b> on your device&apos;s Account page (in its dashboard) and its save
          backups and Journey history are copied here every 10 minutes.
        </p>
      ) : (
        withData.map(({ device, saves, journey }) => (
          <div key={device.id} className="card stack">
            <h2 style={{ fontSize: 18 }}>{device.name}</h2>
            {journey && (
              <p className="small">
                <a href={journey}>Download Journey history</a> <span className="muted">(JSON)</span>
              </p>
            )}
            {saves.map((s) => {
              const name = s.file.split("/").pop();
              const [latest, ...older] = s.versions;
              return (
                <details key={s.file}>
                  <summary>
                    <b>{name}</b>{" "}
                    <span className="muted small">
                      {s.versions.length} version{s.versions.length === 1 ? "" : "s"}
                      {latest?.at ? `, latest ${when(latest.at)}` : ""}
                    </span>
                    {latest?.url && (
                      <>
                        {" · "}
                        <a href={latest.url} download={name}>Download latest</a>
                      </>
                    )}
                  </summary>
                  <ul className="small" style={{ marginTop: 8 }}>
                    {older.map((v) => (
                      <li key={v.name}>
                        {when(v.at) || v.name} {v.url && <a href={v.url} download={name}>Download</a>}
                      </li>
                    ))}
                  </ul>
                </details>
              );
            })}
            <p className="muted small">
              To put a save back on your device, download it and use <b>Upload edited save</b> on its dashboard&apos;s Saves
              page.
            </p>
          </div>
        ))
      )}
    </section>
  );
}
