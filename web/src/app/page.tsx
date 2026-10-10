import Link from "next/link";
import { getGames } from "@/lib/gamedata";

export const revalidate = 3600;

export default async function Home() {
  const games = await getGames();
  return (
    <main>
      <section className="stack" style={{ paddingTop: 24 }}>
        <h1>Your playthrough, read straight from the game.</h1>
        <p className="lead">
          Tallgrass runs next to your emulator on a Raspberry Pi or PC. It shows your party live, what you can catch where
          you&apos;re standing, the trainers ahead, and keeps every save safe. It reads everything from your own copy of the
          game, so romhacks just work.
        </p>
        <div className="chips" style={{ marginTop: 8 }}>
          <Link className="button primary" href="/games">Browse game docs</Link>
          <Link className="button" href="/link">Link a device</Link>
        </div>
      </section>

      <section className="stack">
        <h3>Supported games</h3>
        <div className="grid">
          {games.map((g) => (
            <Link key={g.slug} href={`/games/${g.slug}`} className="card" style={{ textDecoration: "none" }}>
              <h2>{g.name}</h2>
              <p className="muted small">Generation {g.generation} · species, moves, trainers, wild encounters and items</p>
            </Link>
          ))}
        </div>
      </section>

      <section className="card stack" id="install">
        <h3>Install</h3>
        <p>On your Raspberry Pi (RetroPie) or Linux PC, with your game already playable:</p>
        <pre className="install"><code>curl -fsSL https://tallgrass-delta.vercel.app/install.sh | bash</code></pre>
        <p className="muted small">Then open the address it prints and pick your game; the setup page checks everything and fixes what it can. Tallgrass keeps itself up to date. <Link href="/guide">Read the guide</Link>.</p>
      </section>

      <section className="grid">
        <div className="card stack">
          <h3>Live</h3>
          <p>Party with stats, IVs, EVs and moves; the route you&apos;re on with what&apos;s catchable by time of day; trainers and items on the map.</p>
        </div>
        <div className="card stack">
          <h3>Your run</h3>
          <p>Catches, faints, evolutions and new areas logged automatically. Nuzlocke mode when you want it.</p>
        </div>
        <div className="card stack">
          <h3>Saves</h3>
          <p>Every save change is backed up and versioned. Browse your boxes from any save, and restore any version.</p>
        </div>
      </section>
    </main>
  );
}
