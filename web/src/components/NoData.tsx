export default function NoData({ name }: { name: string }) {
  return (
    <div className="card stack">
      <p>Game data for {name} hasn&apos;t been uploaded yet.</p>
      <p className="muted small">A maintainer uploads it from the Tallgrass agent (Library → Upload to Tallgrass).</p>
    </div>
  );
}
