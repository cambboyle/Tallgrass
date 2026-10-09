"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import TypeChips from "./TypeChips";

export type SpeciesRow = { species: number; name: string; types: string[]; total: number };

// Searchable list of a game's species.
export default function SpeciesTable({ slug, rows }: { slug: string; rows: SpeciesRow[] }) {
  const [q, setQ] = useState("");
  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    return s ? rows.filter((r) => r.name.toLowerCase().includes(s) || r.types.some((t) => t.toLowerCase() === s)) : rows;
  }, [q, rows]);
  return (
    <div className="stack">
      <input type="search" placeholder={`Search ${rows.length} Pokémon by name or type`} value={q} onChange={(e) => setQ(e.target.value)} aria-label="Search Pokémon" />
      <div className="table-wrap">
        <table>
          <thead><tr><th className="num">#</th><th>Name</th><th>Type</th><th className="num">Total</th></tr></thead>
          <tbody>
            {shown.slice(0, 400).map((r) => (
              <tr key={r.species}>
                <td className="num muted">{r.species}</td>
                <td><Link href={`/games/${slug}/pokemon/${r.species}`}>{r.name}</Link></td>
                <td><TypeChips types={r.types} /></td>
                <td className="num">{r.total}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {shown.length > 400 && <p className="small muted">Showing 400 of {shown.length}. Search to narrow it down.</p>}
    </div>
  );
}
