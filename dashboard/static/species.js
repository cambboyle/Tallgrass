// Species page: a dialog with a species' types, stats, abilities,
// evolutions, where to find it and the moves it learns. Used by the Party and box viewer pages.
(() => {
  const esc = (s) => String(s ?? "").replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
  const STATS = [["hp", "HP"], ["attack", "Attack"], ["defense", "Defense"], ["spAtk", "Sp. Atk"], ["spDef", "Sp. Def"], ["speed", "Speed"]];

  const dialog = document.createElement("dialog");
  dialog.className = "species";
  document.body.appendChild(dialog);
  dialog.addEventListener("click", (e) => {
    if (e.target === dialog || e.target.closest("[data-close]")) dialog.close(); // backdrop or close button
    const evo = e.target.closest("[data-species]");
    if (evo) window.openSpecies(dialog.dataset.rom, Number(evo.dataset.species));
  });

  // found lists where the species appears in the wild, if we know.
  function found(p) {
    if (!p.foundIn) return "";
    if (!p.foundIn.length) return `<h3>Where to find it</h3><p class="sub">Not found in the wild.</p>`;
    const rows = p.foundIn.map((f) => {
      const lv = f.minLevel === f.maxLevel ? `Lv ${f.minLevel}` : `Lv ${f.minLevel}–${f.maxLevel}`;
      return `<li><span>${esc(f.area)}</span><span class="sub">${esc(f.method)}${f.when ? ` · ${esc(f.when)}` : ""} · ${lv} · ${f.chance}%</span></li>`;
    }).join("");
    return `<h3>Where to find it</h3><ul class="found">${rows}</ul>`;
  }

  // learnt lists TM/HM, tutor and egg moves, when the game's tables are known.
  function learnt(p) {
    const list = (title, items) => items && items.length
      ? `<h3>${title} <span class="sub">${items.length}</span></h3><ul class="movelist">${items.map((m) => `<li>${esc(m)}</li>`).join("")}</ul>` : "";
    const tms = (p.tmMoves || []).length
      ? `<h3>TM and HM moves <span class="sub">${p.tmMoves.length}</span></h3><ul class="levelmoves tms">${p.tmMoves.map((m) =>
        `<li><span>${esc(m.machine)}</span>${esc(m.move)}</li>`).join("")}</ul>` : "";
    const sorted = (a) => (a || []).slice().sort((x, y) => x.localeCompare(y));
    return tms + list("Move tutors", sorted(p.tutorMoves)) + list("Egg moves", sorted(p.eggMoves));
  }

  function html(p) {
    const total = STATS.reduce((n, [k]) => n + p.baseStats[k], 0);
    const bars = STATS.map(([k, label]) => {
      const v = p.baseStats[k];
      return `<div class="stat"><span>${label}</span><b>${v}</b><i style="width:${Math.min(100, v / 1.8)}%"></i></div>`;
    }).join("");
    const abilities = [...p.abilities.map(esc), ...(p.hiddenAbility ? [`${esc(p.hiddenAbility)} <span class="sub">(hidden)</span>`] : [])];
    const evos = p.evolvesTo.length
      ? p.evolvesTo.map((e) => `<li><button class="link" data-species="${e.species}">${esc(e.name)}</button> <span class="sub">${esc(e.how)}</span></li>`).join("")
      : `<li class="sub">Doesn't evolve further.</li>`;
    const moves = p.levelMoves.map((m) => `<li><span>${m.level || "Evo"}</span>${esc(m.move)}</li>`).join("");
    return `<div class="species-head">
        ${p.sprite ? `<img src="${p.sprite}" alt="">` : ""}
        <div><h2>${esc(p.name)}</h2><div class="types">${p.types.map((t) => `<span class="pill">${esc(t)}</span>`).join(" ")}</div></div>
        <button class="btn" data-close aria-label="Close">✕</button>
      </div>
      <h3>Base stats <span class="sub">${total} total</span></h3><div class="stats">${bars}</div>
      <h3>Abilities</h3><p>${abilities.join(", ") || "—"}</p>
      ${(p.evolvesFrom || []).length ? `<h3>Evolves from</h3><ul class="evos">${p.evolvesFrom.map((e) =>
        `<li><button class="link" data-species="${e.species}">${esc(e.name)}</button> <span class="sub">${esc(e.how)}</span></li>`).join("")}</ul>` : ""}
      <h3>Evolves into</h3><ul class="evos">${evos}</ul>
      ${found(p)}
      <h3>Level-up moves</h3><ul class="levelmoves">${moves}</ul>
      ${learnt(p)}`;
  }

  window.openSpecies = async (rom, species) => {
    if (!rom || !species) return;
    dialog.dataset.rom = rom;
    dialog.innerHTML = `<p class="sub">Loading…</p>`;
    if (!dialog.open) dialog.showModal();
    const res = await fetch(`/api/species?rom=${encodeURIComponent(rom)}&species=${species}`);
    dialog.innerHTML = res.ok ? html(await res.json())
      : `<p>Couldn't load this species: ${esc(await res.text())}</p><button class="btn" data-close>Close</button>`;
  };
})();
