import type { Metadata } from "next";
import Link from "next/link";
import { DOCS } from "@/lib/docs";

export const metadata: Metadata = {
  title: "Docs",
  description: "Tallgrass's project docs: how it's built, how to add a game, and how releases work.",
};

export default function Docs() {
  return (
    <main className="guide">
      <section className="stack" style={{ paddingTop: 24 }}>
        <h1>Docs</h1>
        <p className="lead">
          Tallgrass is open source (GPL-3.0; game profiles are CC0). These are the project&apos;s own docs, straight from{" "}
          <a href="https://github.com/cambboyle/Tallgrass">the repository</a>. To use Tallgrass, start with the{" "}
          <Link href="/guide">guide</Link>.
        </p>
      </section>
      {DOCS.map((d) => (
        <section key={d.slug} className="card stack">
          <h2><Link href={`/docs/${d.slug}`}>{d.title}</Link></h2>
          <p>{d.summary}</p>
        </section>
      ))}
    </main>
  );
}
