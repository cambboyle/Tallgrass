import type { Metadata } from "next";
import Link from "next/link";
import { getCatalog, systemName } from "@/lib/catalog";

export const revalidate = 600; // changes on main show within 10 minutes

export const metadata: Metadata = {
  title: "Hacks",
  description: "Where to get each romhack Tallgrass supports, which game it needs, and how to check your patched ROM.",
};

export default async function Hacks() {
  const catalog = await getCatalog();
  const baseOf = (slug: string) => catalog?.bases.find((b) => b.slug === slug);
  return (
    <main className="guide">
      <section className="stack" style={{ paddingTop: 24 }}>
        <h1>Hacks</h1>
        <p className="lead">
          Where to get each romhack Tallgrass supports. Tallgrass never downloads or shares ROMs or patches, and neither
          does this site: you bring your own copy of the base game, apply the hack&apos;s official patch, and check the result
          against the checksum below.
        </p>
      </section>

      {!catalog ? (
        <section className="card">
          <p>The list couldn&apos;t be loaded just now. Try again in a minute, or see the <Link href="/games">game pages</Link>.</p>
        </section>
      ) : (
        <>
          {catalog.hacks.map((h) => {
            const base = baseOf(h.base);
            return (
              <section key={h.slug} className="card stack" id={h.slug}>
                <h2>{h.name}</h2>
                <p className="small muted">
                  By {h.author} · {systemName(h.system)}
                  {h.version ? ` · version ${h.version}` : ""}
                </p>
                <dl className="facts">
                  <dt>Base game</dt>
                  <dd>
                    {base?.name ?? h.base}
                    {h.baseNote ? <span className="muted"> · {h.baseNote}</span> : null}
                  </dd>
                  <dt>Official page</dt>
                  <dd><a href={h.source.url}>{h.source.label}</a></dd>
                  <dt>Patched ROM SHA-1</dt>
                  <dd>
                    {h.result.sha1.map((s) => <code key={s} style={{ overflowWrap: "anywhere" }}>{s}</code>)}
                    <span className="muted small"> (the build Tallgrass is tested against)</span>
                  </dd>
                </dl>
                <p>{h.support}</p>
                <p className="small">
                  <Link href={`/games/${h.slug}/guide`}>Guide</Link> · <Link href={`/games/${h.slug}`}>Game data</Link>
                </p>
              </section>
            );
          })}

          <section className="card stack" id="bases">
            <h2>Base games</h2>
            <p className="small muted">
              The original games these hacks are made from. A checksum is listed only where it was checked against a real
              copy; otherwise the hack&apos;s own page says which version it needs.
            </p>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr><th>Game</th><th>System</th><th>Checksums</th></tr>
                </thead>
                <tbody>
                  {catalog.bases.map((b) => (
                    <tr key={b.slug}>
                      <td>
                        {b.name}
                        {b.gameCode ? <span className="muted small"> · {b.gameCode}</span> : null}
                        {b.detail ? <div className="muted small">{b.detail}</div> : null}
                      </td>
                      <td>{systemName(b.system)}</td>
                      <td className="small">
                        {b.sha1?.map((s) => <div key={s}>SHA-1 <code style={{ overflowWrap: "anywhere" }}>{s}</code></div>)}
                        {b.crc32 ? <div>CRC32 <code>{b.crc32}</code></div> : null}
                        {!b.sha1?.length && !b.crc32 ? <span className="muted">See the hack&apos;s page</span> : null}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>

          <section className="card stack" id="check">
            <h2>Checking your ROM</h2>
            <p>After patching, compare your ROM&apos;s SHA-1 with the one above. If it matches, Tallgrass will recognise it.</p>
            <ul>
              <li>On a Mac or Linux: <code>shasum -a 1 &quot;your-rom.gba&quot;</code></li>
              <li>On Windows: <code>certutil -hashfile &quot;your-rom.gba&quot; SHA1</code></li>
              <li>On the device: the Library page shows a ROM&apos;s SHA-1 when you hover &quot;not supported yet&quot;.</li>
            </ul>
            <p className="small muted">
              A different checksum usually means a different version of the hack, or a different copy of the base game.
              Tallgrass may still read it if the hack is close, but it&apos;s only tested against the builds above.
            </p>
          </section>
        </>
      )}
    </main>
  );
}
