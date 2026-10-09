"use client";

import Link from "next/link";
import { useSyncExternalStore } from "react";

export type AreaRow = {
  name: string;
  href: string;
  level: number | null;
  unnamed: boolean;
  cells: React.ReactNode[];
};

type Order = "journey" | "az";
const collator = new Intl.Collator("en", { numeric: true });

function sortRows(rows: AreaRow[], order: Order) {
  return [...rows].sort((a, b) => {
    if (a.unnamed !== b.unnamed) return a.unnamed ? 1 : -1;
    if (order === "journey") {
      const d = (a.level ?? Infinity) - (b.level ?? Infinity);
      if (d) return d;
    }
    return collator.compare(a.name, b.name);
  });
}

// The chosen order lives in localStorage; components re-read it when it
// changes (here or in another tab).
const KEY = "area-order";
const listeners = new Set<() => void>();
function subscribe(l: () => void) {
  listeners.add(l);
  window.addEventListener("storage", l);
  return () => {
    listeners.delete(l);
    window.removeEventListener("storage", l);
  };
}
function readOrder(): Order {
  try {
    return localStorage.getItem(KEY) === "az" ? "az" : "journey";
  } catch {
    return "journey";
  }
}

// A table of areas that sorts by journey (default) or name. The choice is
// remembered in this browser.
export default function AreaSort({ head, rows }: { head: string[]; rows: AreaRow[] }) {
  const order = useSyncExternalStore(subscribe, readOrder, () => "journey" as Order);
  const choose = (o: Order) => {
    try {
      localStorage.setItem(KEY, o);
    } catch {}
    listeners.forEach((l) => l());
  };
  return (
    <div className="stack">
      <div className="sort" role="group" aria-label="Sort areas">
        <span className="small muted">Sort</span>
        <button className={order === "journey" ? "on" : ""} onClick={() => choose("journey")} aria-pressed={order === "journey"}>Journey</button>
        <button className={order === "az" ? "on" : ""} onClick={() => choose("az")} aria-pressed={order === "az"}>A–Z</button>
        {order === "journey" && <span className="small muted">Roughly when you get there, from trainer and wild levels.</span>}
      </div>
      <div className="table-wrap">
        <table>
          <thead><tr>{head.map((h, i) => <th key={h} className={i > 0 && i < head.length - 1 ? "num" : undefined}>{h}</th>)}</tr></thead>
          <tbody>
            {sortRows(rows, order).map((r) => (
              <tr key={r.href}>
                <td style={{ whiteSpace: "nowrap" }}><Link href={r.href}>{r.name}</Link></td>
                {r.cells.map((c, i) => <td key={i} className={i < r.cells.length - 1 ? "num" : "small"}>{c}</td>)}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
