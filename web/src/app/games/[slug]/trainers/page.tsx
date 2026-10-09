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

export async function generateMetadata({ params }: PageProps<"/games/[slug]/trainers">) {
  const game = await getGame((await params).slug);
  return { title: `Trainers · ${game?.name ?? "Game"}` };
}

export default async function Trainers({ params }: PageProps<"/games/[slug]/trainers">) {
  const { slug } = await params;
  const game = await getGame(slug);
  if (!game) notFound();
  const { data, uploadedAt } = await getGameData(slug);
  const withTrainers = data ? areas(data).filter((a) => a.trainers.length) : [];
  const placed = new Set(withTrainers.flatMap((a) => a.trainers.map((t) => t.id)));
  const elsewhere = (data?.trainers ?? []).filter((t) => !placed.has(t.id)).length;

  return (
    <main>
      <GameHeader game={game} uploadedAt={uploadedAt} subtitle={`${game.name} trainers`} />
      <GameNav slug={slug} current="trainers" />
      {!data ? <NoData name={game.name} /> : (
        <>
          <p className="muted">{data.trainers.length} battles, by area. Open an area for its full teams.</p>
          <AreaSort
            head={["Area", "Highest level", "Trainers"]}
            rows={withTrainers.map((a) => ({
              name: a.name,
              href: `/games/${slug}/areas/${a.slug}`,
              level: a.level,
              unnamed: unnamed(a.name),
              cells: [
                Math.max(...a.trainers.map((t) => Math.max(...t.party.map((p) => p.level)))),
                a.trainers.map((t) => `${t.class} ${t.name}`).join(", "),
              ],
            }))}
          />
          {elsewhere > 0 && (
            <p className="small muted">{elsewhere} more battles aren&apos;t tied to a place (rematches, battle facilities, scripted fights).</p>
          )}
        </>
      )}
    </main>
  );
}
