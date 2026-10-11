import { Marked, type Tokens } from "marked";

// The project's docs (docs/*.md in the repository), rendered from GitHub so
// the site always shows what's on main.
const REPO = "https://github.com/cambboyle/Tallgrass";
const RAW = "https://raw.githubusercontent.com/cambboyle/Tallgrass/main/docs/";

export const DOCS = [
  { slug: "handbook", title: "Handbook", summary: "How Tallgrass fits together, and how to develop, test, release and run every part of it." },
  { slug: "writing-a-profile", title: "Writing a game profile", summary: "Add support for a new romhack: profile fields and how to find addresses." },
  { slug: "gamedata-export", title: "Game data export format", summary: "The JSON the agent exports from a ROM, which the game docs on this site read." },
  { slug: "releasing", title: "Releasing", summary: "The release workflow, versions and the signing key." },
] as const;

export type Doc = (typeof DOCS)[number];

export const findDoc = (slug: string) => DOCS.find((d) => d.slug === slug);

// GitHub's heading anchors: lower case, punctuation dropped, spaces to dashes.
function slugify(text: string): string {
  return text.toLowerCase().trim().replace(/<[^>]+>/g, "").replace(/[^\p{L}\p{N}\s_-]/gu, "").replace(/\s/g, "-");
}

// Links to other docs stay on this site; anything else in the repository
// goes to GitHub.
function rewriteHref(href: string): string {
  if (/^[a-z]+:|^#/i.test(href)) return href;
  const [path, hash] = href.split("#");
  const doc = DOCS.find((d) => path === `${d.slug}.md` || path === `./${d.slug}.md`);
  if (doc) return `/docs/${doc.slug}${hash ? `#${hash}` : ""}`;
  return new URL(href, `${REPO}/blob/main/docs/`).toString();
}

function renderer() {
  const marked = new Marked({ gfm: true });
  marked.use({
    renderer: {
      heading(this: { parser: { parseInline: (t: Tokens.Generic[]) => string } }, { tokens, depth, text }: Tokens.Heading) {
        return `<h${depth} id="${slugify(text)}">${this.parser.parseInline(tokens)}</h${depth}>\n`;
      },
    },
    walkTokens(token) {
      if (token.type === "link") token.href = rewriteHref(token.href);
    },
  });
  return marked;
}

export async function getDoc(doc: Doc): Promise<{ html: string; source: string } | null> {
  const res = await fetch(RAW + doc.slug + ".md", { next: { revalidate: 600 } });
  if (!res.ok) return null;
  const md = await res.text();
  // The first heading is the page title, shown by the page itself.
  const body = md.replace(/^# .*\n/, "");
  return { html: await renderer().parse(body), source: `${REPO}/blob/main/docs/${doc.slug}.md` };
}
