import { notFound } from "next/navigation";
import { getGame, getGameData, statTotal } from "@/lib/gamedata";
import GameHeader from "@/components/GameHeader";
import GameNav from "@/components/GameNav";
import NoData from "@/components/NoData";
import SpeciesTable from "@/components/SpeciesTable";

export const revalidate = 600; // new uploads show within 10 minutes

// Rendered on first visit, then cached.
export async function generateStaticParams() {
  return [];
}

export async function generateMetadata({ params }: PageProps<"/games/[slug]">) {
  const game = await getGame((await params).slug);
  return { title: game?.name ?? "Game" };
}

export default async function GamePage({ params }: PageProps<"/games/[slug]">) {
  const { slug } = await params;
  const game = await getGame(slug);
  if (!game) notFound();
  const { data, uploadedAt } = await getGameData(slug);
  return (
    <main>
      <GameHeader game={game} uploadedAt={uploadedAt} />
      <GameNav slug={slug} current="pokemon" />
      {data ? (
        <SpeciesTable slug={slug} rows={data.species.map((s) => ({ species: s.species, name: s.name, types: s.types, total: statTotal(s.baseStats) }))} />
      ) : (
        <NoData name={game.name} />
      )}
    </main>
  );
}
