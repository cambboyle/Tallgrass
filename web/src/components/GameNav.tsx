import Link from "next/link";

export default function GameNav({ slug, current }: { slug: string; current: "pokemon" | "trainers" | "areas" }) {
  const tabs = [
    { key: "pokemon", href: `/games/${slug}`, label: "Pokémon" },
    { key: "trainers", href: `/games/${slug}/trainers`, label: "Trainers" },
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
