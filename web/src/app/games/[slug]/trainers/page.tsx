import Link from "next/link";
import { notFound } from "next/navigation";
import { getGame, getGameData, type Trainer } from "@/lib/gamedata";
import { slugify } from "@/lib/types";
import GameHeader from "@/components/GameHeader";
import GameNav from "@/components/GameNav";
import NoData from "@/components/NoData";

export const revalidate = 3600;

// Rendered on first visit, then cached for an hour.
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

  // Group by the first area each trainer appears in, in map order.
  const groups = new Map<string, Trainer[]>();
  for (const t of data?.trainers ?? []) {
    const area = t.areas?.[0] ?? "";
    if (!groups.has(area)) groups.set(area, []);
    groups.get(area)!.push(t);
  }
  const areas = [...groups.keys()].filter(Boolean);
  const elsewhere = groups.get("") ?? [];

  return (
    <main>
      <GameHeader game={game} uploadedAt={uploadedAt} subtitle={`${game.name} trainers`} />
      <GameNav slug={slug} current="trainers" />
      {!data ? <NoData name={game.name} /> : (
        <>
          <p className="muted">{data.trainers.length} battles, by area. Open an area for its full teams.</p>
          <div className="table-wrap">
            <table>
              <thead><tr><th>Area</th><th>Trainers</th></tr></thead>
              <tbody>
                {areas.map((a) => (
                  <tr key={a}>
                    <td style={{ whiteSpace: "nowrap" }}><Link href={`/games/${slug}/areas/${slugify(a)}`}>{a}</Link></td>
                    <td className="small">{groups.get(a)!.map((t) => `${t.class} ${t.name}`).join(", ")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {elsewhere.length > 0 && (
            <p className="small muted">{elsewhere.length} more battles aren&apos;t tied to a place (rematches, battle facilities, scripted fights).</p>
          )}
        </>
      )}
    </main>
  );
}
