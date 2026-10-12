import Link from "next/link";
import { hasGuide } from "@/lib/guides";

export default function GameNav({ slug, current }: { slug: string; current: "guide" | "pokemon" | "trainers" | "bosses" | "areas" }) {
  const tabs = [
    ...(hasGuide(slug) ? [{ key: "guide", href: `/games/${slug}/guide`, label: "Guide" }] : []),
    { key: "pokemon", href: `/games/${slug}`, label: "Pokémon" },
    { key: "trainers", href: `/games/${slug}/trainers`, label: "Trainers" },
    { key: "bosses", href: `/games/${slug}/bosses`, label: "Bosses" },
    { key: "areas", href: `/games/${slug}/areas`, label: "Areas" },
  ];
  return (
    <nav className="tabs">
      {tabs.map((t) => (
        <Link key={t.key} href={t.href} aria-current={t.key === current ? "page" : undefined}>{t.label}</Link>
      ))}
    </nav>
  );
}
