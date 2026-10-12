import { notFound } from "next/navigation";
import Link from "next/link";
import { getGame } from "@/lib/gamedata";
import { GUIDES } from "@/lib/guides";
import GameHeader from "@/components/GameHeader";
import GameNav from "@/components/GameNav";

export const revalidate = 3600;

// `code` spans in the guide text.
function Rich({ text }: { text: string }) {
  return text.split("`").map((part, i) => (i % 2 ? <code key={i}>{part}</code> : part));
}

// Rendered on first visit, then cached.
export async function generateStaticParams() {
  return [];
}

export async function generateMetadata({ params }: PageProps<"/games/[slug]/guide">) {
  const { slug } = await params;
  const game = await getGame(slug);
  return { title: `Guide · ${game?.name ?? "Game"}`, description: GUIDES[slug]?.tagline };
}

export default async function GameGuide({ params }: PageProps<"/games/[slug]/guide">) {
  const { slug } = await params;
  const game = await getGame(slug);
  const guide = GUIDES[slug];
  if (!game || !guide) notFound();
  return (
    <main className="guide">
      <GameHeader game={game} uploadedAt={null} />
      <GameNav slug={slug} current="guide" />
      <section className="card stack">
        <p className="lead">{guide.tagline}</p>
        {guide.about.map((p) => <p key={p}><Rich text={p} /></p>)}
        <dl className="facts">
          <dt>By</dt><dd>{guide.author}</dd>
          <dt>Built on</dt><dd>{guide.base}</dd>
          <dt>Version</dt><dd>{guide.version}</dd>
        </dl>
      </section>
      <section className="card stack">
        <h2>What the game adds</h2>
        <ul>{guide.highlights.map((h) => <li key={h}>{h}</li>)}</ul>
      </section>
      <section className="card stack">
        <h2>With Tallgrass</h2>
        <ul>{guide.withTallgrass.map((h) => <li key={h}><Rich text={h} /></li>)}</ul>
        <p className="small muted">Good to know:</p>
        <ul className="small muted">{guide.caveats.map((c) => <li key={c}><Rich text={c} /></li>)}</ul>
      </section>
      <section className="card stack">
        <h2>Getting started</h2>
        <ol>{guide.start.map((s) => <li key={s}><Rich text={s} /></li>)}</ol>
        <p className="small muted">
          Full install and setup steps are in the <Link href="/guide">guide</Link>.
        </p>
      </section>
      <section className="card stack">
        <h2>Official and useful links</h2>
        <ul>
          {guide.links.map((l) => (
            <li key={l.url}><a href={l.url}>{l.label}</a>: {l.note}</li>
          ))}
        </ul>
        <p className="small muted">
          Written from the hack&apos;s public information and Tallgrass&apos;s own testing, for the version above. Features
          change between releases, so the official thread is the authority.
        </p>
      </section>
    </main>
  );
}
