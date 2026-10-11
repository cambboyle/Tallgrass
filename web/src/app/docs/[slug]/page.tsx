import { notFound } from "next/navigation";
import Link from "next/link";
import { findDoc, getDoc } from "@/lib/docs";

export const revalidate = 600; // changes on main show within 10 minutes

// Rendered on first visit, then cached.
export async function generateStaticParams() {
  return [];
}

export async function generateMetadata({ params }: PageProps<"/docs/[slug]">) {
  const doc = findDoc((await params).slug);
  return { title: doc?.title ?? "Docs", description: doc?.summary };
}

export default async function DocPage({ params }: PageProps<"/docs/[slug]">) {
  const doc = findDoc((await params).slug);
  if (!doc) notFound();
  const page = await getDoc(doc);
  if (!page) notFound();
  return (
    <main className="guide">
      <section className="stack" style={{ paddingTop: 24 }}>
        <p className="small muted"><Link href="/docs">Docs</Link></p>
        <h1>{doc.title}</h1>
        <p className="small muted">
          From <a href={page.source}>docs/{doc.slug}.md</a> on GitHub. Spotted a mistake? Edit it there.
        </p>
      </section>
      {/* Our own repository's Markdown, rendered on the server. */}
      <article className="card doc" dangerouslySetInnerHTML={{ __html: page.html }} />
    </main>
  );
}
