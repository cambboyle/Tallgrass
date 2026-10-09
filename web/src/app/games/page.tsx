import Link from "next/link";
import { getGames } from "@/lib/gamedata";

export const revalidate = 3600;
export const metadata = { title: "Games" };

export default async function Games() {
  const games = await getGames();
  return (
    <main>
      <div className="stack">
        <h1>Games</h1>
        <p className="lead">Docs generated from each game itself: every species, move list, trainer and wild encounter.</p>
      </div>
      <div className="grid">
        {games.map((g) => (
          <Link key={g.slug} href={`/games/${g.slug}`} className="card" style={{ textDecoration: "none" }}>
            <h2>{g.name}</h2>
            <p className="muted small">Generation {g.generation}</p>
          </Link>
        ))}
      </div>
    </main>
  );
}
