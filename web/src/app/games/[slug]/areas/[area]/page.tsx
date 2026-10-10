import Link from "next/link";
import { notFound } from "next/navigation";
import { getGame, getGameData } from "@/lib/gamedata";
import { areas } from "@/lib/areas";
import { slugify } from "@/lib/types";
import GameNav from "@/components/GameNav";
import TrainerCard from "@/components/TrainerCard";

export const revalidate = 600; // new uploads show within 10 minutes

// Rendered on first visit, then cached.
export async function generateStaticParams() {
  return [];
}

async function load(slug: string, areaSlug: string) {
  const game = await getGame(slug);
  const { data } = await getGameData(slug);
  const area = data ? areas(data).find((a) => a.slug === areaSlug) : undefined;
  return game && area ? { game, area } : null;
}

export async function generateMetadata({ params }: PageProps<"/games/[slug]/areas/[area]">) {
  const { slug, area } = await params;
  const r = await load(slug, area);
  return { title: r ? `${r.area.name} · ${r.game.name}` : "Area" };
}

export default async function AreaPage({ params }: PageProps<"/games/[slug]/areas/[area]">) {
  const { slug, area: areaSlug } = await params;
  const r = await load(slug, areaSlug);
  if (!r) notFound();
  const { game, area } = r;
  const order = ["Grass", "Surfing", "Rock Smash", "Old Rod", "Good Rod", "Super Rod"];
  const rank = (m: string) => (order.indexOf(m) + 1 || order.length + 1);
  const methods = [...new Set(area.encounters.map((e) => e.method))].sort((a, b) => rank(a) - rank(b));

  return (
    <main>
      <div className="stack">
        <p className="small muted"><Link href={`/games/${slug}/areas`}>{game.name} areas</Link></p>
        <h1>{area.name}</h1>
        {area.leadsTo.length > 0 && (
          <p className="small muted">Leads to {area.leadsTo.map((a, i) => (
            <span key={a}>{i > 0 && ", "}<Link href={`/games/${slug}/areas/${slugify(a)}`}>{a}</Link></span>
          ))}</p>
        )}
      </div>
      <GameNav slug={slug} current="areas" />

      {methods.map((m) => (
        <section key={m} className="stack">
          <h3>{m}</h3>
          <div className="table-wrap">
            <table>
              <thead><tr><th>Pokémon</th><th>When</th><th className="num">Level</th><th className="num">Chance</th></tr></thead>
              <tbody>
                {area.encounters.filter((e) => e.method === m).map((e, i) => (
                  <tr key={i}>
                    <td><Link href={`/games/${slug}/pokemon/${e.species}`}>{e.name}</Link></td>
                    <td className="small">{e.when || "Any time"}</td>
                    <td className="num">{e.minLevel === e.maxLevel ? e.minLevel : `${e.minLevel}–${e.maxLevel}`}</td>
                    <td className="num">{e.chance}%</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      ))}

      {area.items.length > 0 && (
        <section className="stack">
          <h3>Items</h3>
          <div className="chips">
            {area.items.map((it, i) => (
              <span key={i} className="chip">{it.item}{it.quantity ? ` ×${it.quantity}` : ""}{it.hidden ? <span className="muted"> · hidden</span> : null}</span>
            ))}
          </div>
        </section>
      )}

      {area.shops.map((shop, i) => (
        <section key={i} className="stack">
          <h3>{shop.badges ? "Poké Mart" : "Shop"}{area.shops.length > 1 && !shop.badges ? ` ${i + 1}` : ""}</h3>
          <div className="chips">
            {shop.items.map((it, j) => (
              <span key={it} className="chip">
                {it}
                {shop.badges?.[j] ? <span className="muted"> · {shop.badges[j]} badge{shop.badges[j] > 1 ? "s" : ""}</span> : null}
              </span>
            ))}
          </div>
          {shop.badges && <p className="muted small">Stock grows with your badge count (Johto and Kanto badges both count).</p>}
        </section>
      ))}

      {area.trainers.length > 0 && (
        <section className="stack">
          <h3>Trainers · {area.trainers.length}</h3>
          {area.trainers.map((t) => <TrainerCard key={t.id} slug={slug} t={t} />)}
        </section>
      )}
    </main>
  );
}
