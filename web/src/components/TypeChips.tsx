import { typeColour } from "@/lib/types";

export default function TypeChips({ types }: { types: string[] }) {
  return (
    <span>
      {types.map((t) => (
        <span key={t} className="type" style={{ "--type": typeColour(t) } as React.CSSProperties}>{t}</span>
      ))}
    </span>
  );
}
