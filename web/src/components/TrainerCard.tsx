import Link from "next/link";
import type { Trainer } from "@/lib/gamedata";

export default function TrainerCard({ slug, t }: { slug: string; t: Trainer }) {
  return (
    <article className="card stack" id={`t${t.id}`}>
      <div style={{ display: "flex", justifyContent: "space-between", gap: 12, flexWrap: "wrap" }}>
        <h2 style={{ fontSize: 18 }}>{t.class} {t.name}{t.double && <span className="chip small" style={{ marginLeft: 8 }}>Double</span>}</h2>
        <span className="mono small muted">#{t.id}</span>
      </div>
      {t.items?.length ? <p className="small muted">Uses {t.items.join(", ")}</p> : null}
      <div className="table-wrap">
        <table>
          <tbody>
            {t.party.map((m, i) => (
              <tr key={i}>
                <td style={{ width: "30%" }}><Link href={`/games/${slug}/pokemon/${m.species}`}>{m.speciesName}</Link></td>
                <td className="num" style={{ width: 70 }}>Lv {m.level}</td>
                <td className="small">{m.item && <span className="muted">@ {m.item}</span>}{m.item && m.moves?.length ? " · " : ""}{m.moves?.join(", ")}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </article>
  );
}
