import Link from "next/link";
import { notFound } from "next/navigation";
import { getGame, getGameData } from "@/lib/gamedata";
import { areas } from "@/lib/areas";
import GameHeader from "@/components/GameHeader";
import GameNav from "@/components/GameNav";
import NoData from "@/components/NoData";

export const revalidate = 3600;

// Rendered on first visit, then cached for an hour.
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
  const list = data ? areas(data).sort((a, b) => a.name.localeCompare(b.name, "en", { numeric: true })) : [];
  return (
    <main>
      <GameHeader game={game} uploadedAt={uploadedAt} subtitle={`${game.name} areas`} />
      <GameNav slug={slug} current="areas" />
      {!data ? <NoData name={game.name} /> : (
        <div className="table-wrap">
          <table>
            <thead><tr><th>Area</th><th className="num">Wild</th><th className="num">Items</th><th className="num">Trainers</th><th>Shop</th></tr></thead>
            <tbody>
              {list.map((a) => (
                <tr key={a.slug}>
                  <td><Link href={`/games/${slug}/areas/${a.slug}`}>{a.name}</Link></td>
                  <td className="num">{new Set(a.encounters.map((e) => e.species)).size || ""}</td>
                  <td className="num">{a.items.length || ""}</td>
                  <td className="num">{a.trainers.length || ""}</td>
                  <td className="small">{a.shops.length ? "Yes" : ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}
