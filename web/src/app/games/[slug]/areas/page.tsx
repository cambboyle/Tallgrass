import { notFound } from "next/navigation";
import { getGame, getGameData } from "@/lib/gamedata";
import { areas, unnamed } from "@/lib/areas";
import AreaSort from "@/components/AreaSort";
import GameHeader from "@/components/GameHeader";
import GameNav from "@/components/GameNav";
import NoData from "@/components/NoData";

export const revalidate = 600; // new uploads show within 10 minutes

// Rendered on first visit, then cached.
export async function generateStaticParams() {
  return [];
}

export async function generateMetadata({ params }: PageProps<"/games/[slug]/areas">) {
  const game = await getGame((await params).slug);
  return { title: `Areas · ${game?.name ?? "Game"}` };
}

export default async function Areas({ params }: PageProps<"/games/[slug]/areas">) {
  const { slug } = await params;
  const game = await getGame(slug);
  if (!game) notFound();
  const { data, uploadedAt } = await getGameData(slug);
  const rows = data
    ? areas(data).map((a) => ({
        name: a.name,
        href: `/games/${slug}/areas/${a.slug}`,
        level: a.level,
        unnamed: unnamed(a.name),
        cells: [
          new Set(a.encounters.map((e) => e.species)).size || "",
          a.items.length || "",
          a.trainers.length || "",
          a.shops.length ? "Yes" : "",
        ],
      }))
    : [];
  return (
    <main>
      <GameHeader game={game} uploadedAt={uploadedAt} subtitle={`${game.name} areas`} />
      <GameNav slug={slug} current="areas" />
      {!data ? <NoData name={game.name} /> : <AreaSort head={["Area", "Wild", "Items", "Trainers", "Shop"]} rows={rows} />}
    </main>
  );
}
