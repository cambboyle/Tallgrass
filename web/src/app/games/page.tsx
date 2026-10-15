import Link from "next/link";
import { getGames } from "@/lib/gamedata";
import { GUIDES } from "@/lib/guides";

export const revalidate = 3600;
export const metadata = { title: "Games" };

export default async function Games() {
  const games = await getGames();
  return (
    <main>
      <div className="stack">
        <h1>Games</h1>
        <p className="lead">A guide to each game, and docs generated from the game itself: every species, move list, trainer and wild encounter.</p>
        <p className="small muted">Looking for the game itself? See <Link href="/hacks">where to get each hack</Link>.</p>
      </div>
      <div className="grid">
        {games.map((g) => (
          <Link key={g.slug} href={`/games/${g.slug}`} className="card" style={{ textDecoration: "none" }}>
            <h2>{g.name}</h2>
            <p className="muted small">Generation {g.generation}</p>
            {GUIDES[g.slug] && <p className="small">{GUIDES[g.slug].tagline}</p>}
          </Link>
        ))}
      </div>
    </main>
  );
}
