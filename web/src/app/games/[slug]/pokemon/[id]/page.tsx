import Link from "next/link";
import { notFound } from "next/navigation";
import { getGame, getGameData, statTotal, type Species } from "@/lib/gamedata";
import { slugify } from "@/lib/types";
import GameNav from "@/components/GameNav";
import TypeChips from "@/components/TypeChips";

export const revalidate = 3600;

// Rendered on first visit, then cached for an hour.
export async function generateStaticParams() {
  return [];
}

async function load(slug: string, id: string) {
  const game = await getGame(slug);
  if (!game) return null;
  const { data } = await getGameData(slug);
  const sp = data?.species.find((s) => s.species === Number(id));
  return game && sp ? { game, sp } : null;
}

export async function generateMetadata({ params }: PageProps<"/games/[slug]/pokemon/[id]">) {
  const { slug, id } = await params;
  const r = await load(slug, id);
  return { title: r ? `${r.sp.name} · ${r.game.name}` : "Pokémon" };
}

const STATS: [keyof Species["baseStats"], string][] = [
  ["hp", "HP"], ["attack", "Attack"], ["defense", "Defense"], ["spAtk", "Sp. Atk"], ["spDef", "Sp. Def"], ["speed", "Speed"],
];

function MoveList({ title, moves }: { title: string; moves: string[] }) {
  if (!moves.length) return null;
  return (
    <section className="stack">
      <h3>{title} · {moves.length}</h3>
      <div className="chips">{[...moves].sort().map((m) => <span key={m} className="chip">{m}</span>)}</div>
    </section>
  );
}

export default async function SpeciesPage({ params }: PageProps<"/games/[slug]/pokemon/[id]">) {
  const { slug, id } = await params;
  const r = await load(slug, id);
  if (!r) notFound();
  const { game, sp } = r;
  const evoLink = (e: { species: number; name: string; how: string }) => (
    <li key={`${e.species}-${e.how}`}><Link href={`/games/${slug}/pokemon/${e.species}`}>{e.name}</Link> <span className="muted small">{e.how}</span></li>
  );

  return (
    <main>
      <div className="stack">
        <p className="small muted"><Link href={`/games/${slug}`}>{game.name}</Link> · #{sp.species}</p>
        <h1>{sp.name}</h1>
        <TypeChips types={sp.types} />
      </div>
      <GameNav slug={slug} current="pokemon" />

      <div className="grid">
        <section className="card stack">
          <h3>Base stats · {statTotal(sp.baseStats)}</h3>
          {STATS.map(([k, label]) => (
            <div key={k} className="statbar">
              <span className="small muted">{label}</span>
              <span className="mono">{sp.baseStats[k]}</span>
              <i style={{ width: `${Math.min(100, sp.baseStats[k] / 1.8)}%` }} />
            </div>
          ))}
        </section>
        <section className="card stack">
          <h3>Abilities</h3>
          <p>{sp.abilities.join(", ") || "None"}</p>
          {sp.hiddenAbility && <p><span className="muted small">Hidden:</span> {sp.hiddenAbility}</p>}
          <h3>Evolution</h3>
          <ul className="small" style={{ margin: 0, paddingLeft: 18 }}>
            {sp.evolvesFrom.map(evoLink)}
            {sp.evolvesTo.map(evoLink)}
            {!sp.evolvesFrom.length && !sp.evolvesTo.length && <li className="muted">Doesn&apos;t evolve.</li>}
          </ul>
        </section>
      </div>

      {sp.foundIn && (
        <section className="stack">
          <h3>Where to find it</h3>
          {sp.foundIn.length ? (
            <div className="table-wrap">
              <table>
                <thead><tr><th>Area</th><th>Method</th><th>When</th><th className="num">Level</th><th className="num">Chance</th></tr></thead>
                <tbody>
                  {sp.foundIn.map((f, i) => (
                    <tr key={i}>
                      <td><Link href={`/games/${slug}/areas/${slugify(f.area)}`}>{f.area}</Link></td>
                      <td>{f.method}</td>
                      <td className="small">{f.when || "Any time"}</td>
                      <td className="num">{f.minLevel === f.maxLevel ? f.minLevel : `${f.minLevel}–${f.maxLevel}`}</td>
                      <td className="num">{f.chance}%</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : <p className="muted">Not found in the wild: a gift, trade, evolution or event.</p>}
        </section>
      )}

      {sp.levelMoves.length > 0 && (
        <section className="stack">
          <h3>Level-up moves</h3>
          <div className="table-wrap">
            <table>
              <thead><tr><th className="num" style={{ width: 80 }}>Level</th><th>Move</th></tr></thead>
              <tbody>
                {sp.levelMoves.map((m, i) => (
                  <tr key={i}><td className="num">{m.level || "Evo"}</td><td>{m.move}</td></tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {sp.tmMoves.length > 0 && (
        <section className="stack">
          <h3>TM and HM moves · {sp.tmMoves.length}</h3>
          <div className="chips">{sp.tmMoves.map((m) => <span key={m.machine} className="chip"><span className="mono muted">{m.machine}</span> {m.move}</span>)}</div>
        </section>
      )}
      <MoveList title="Move tutors" moves={sp.tutorMoves} />
      <MoveList title="Egg moves" moves={sp.eggMoves} />
    </main>
  );
}
