import type { Game } from "@/lib/gamedata";

export default function GameHeader({ game, uploadedAt, subtitle }: { game: Game; uploadedAt: string | null; subtitle?: string }) {
  return (
    <div className="stack">
      <p className="small muted">Generation {game.generation}{uploadedAt ? ` · data from ${new Date(uploadedAt).toLocaleDateString("en-GB", { dateStyle: "medium" })}` : ""}</p>
      <h1>{subtitle ?? game.name}</h1>
    </div>
  );
}
