import { notFound } from "next/navigation";
import Link from "next/link";
import { getGame, getGameData } from "@/lib/gamedata";
import { bosses, itemList } from "@/lib/bosses";
import { slugify } from "@/lib/types";
import GameHeader from "@/components/GameHeader";
import GameNav from "@/components/GameNav";
import NoData from "@/components/NoData";
import { Team } from "@/components/TrainerCard";

export const revalidate = 600; // new uploads show within 10 minutes

// Rendered on first visit, then cached.
export async function generateStaticParams() {
  return [];
}

export async function generateMetadata({ params }: PageProps<"/games/[slug]/bosses">) {
  const game = await getGame((await params).slug);
  return { title: `Bosses · ${game?.name ?? "Game"}` };
}

export default async function Bosses({ params }: PageProps<"/games/[slug]/bosses">) {
  const { slug } = await params;
  const game = await getGame(slug);
  if (!game) notFound();
  const { data, uploadedAt } = await getGameData(slug);
  const sections = data ? bosses(data) : [];
  return (
    <main className="guide">
      <GameHeader game={game} uploadedAt={uploadedAt} subtitle={`${game.name} bosses`} />
      <GameNav slug={slug} current="bosses" />
      {!data ? <NoData name={game.name} /> : (
        <>
          <p className="muted">
            Gym leaders, the Elite Four and champions, rivals and villain bosses, with their full teams, read from the
            game itself. A game keeps several teams per boss (rematches, difficulty modes, a different starter), and its
            data doesn&apos;t say which is the first fight, so each boss lists their teams from the lowest level up.
          </p>
          <nav className="chips" aria-label="Jump to">
            {sections.map((s) => <a key={s.category} className="chip" href={`#${slugify(s.category)}`}>{s.category}</a>)}
          </nav>
          {sections.map((s) => (
            <section key={s.category} className="stack" id={slugify(s.category)} style={{ scrollMarginTop: 16 }}>
              <h2>{s.category}</h2>
              {s.groups.map((g) => (
                <article key={g.key} className="card stack">
                  <h3>{g.class} {g.name}</h3>
                  {g.areas.length > 0 && (
                    <p className="small muted">
                      Fought in{" "}
                      {g.areas.map((a, i) => (
                        <span key={a}>{i > 0 && ", "}<Link href={`/games/${slug}/areas/${slugify(a)}`}>{a}</Link></span>
                      ))}
                    </p>
                  )}
                  {g.teams.map((t, i) => {
                    const top = Math.max(0, ...t.party.map((p) => p.level));
                    return (
                      <div key={t.id} className="stack">
                        {(g.teams.length > 1 || t.items?.length || t.double) && (
                          <p className="small muted">
                            {g.teams.length > 1 ? `Team ${i + 1}${top ? ` · up to Lv ${top}` : ""}` : ""}
                            {t.double ? `${g.teams.length > 1 ? " · " : ""}Double battle` : ""}
                            {t.items?.length ? `${g.teams.length > 1 || t.double ? " · " : ""}Uses ${itemList(t.items)}` : ""}
                          </p>
                        )}
                        <Team slug={slug} party={t.party} />
                        {t.otherParty?.length ? (
                          <>
                            <p className="small muted">Other team (used in another mode)</p>
                            <Team slug={slug} party={t.otherParty} />
                          </>
                        ) : null}
                      </div>
                    );
                  })}
                </article>
              ))}
            </section>
          ))}
          {sections.length === 0 && <p className="muted">This game&apos;s data has no trainers we could pick out as bosses.</p>}
        </>
      )}
    </main>
  );
}
