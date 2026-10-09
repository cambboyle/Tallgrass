// Colours for type chips (one shade per type, white text).
const colours: Record<string, string> = {
  normal: "#8a8a6c", fire: "#d8622b", water: "#3f77d6", grass: "#4c9a3a", electric: "#c99a12", ice: "#4ca9b5",
  fighting: "#b2392f", poison: "#8b3f99", ground: "#b08a3e", flying: "#7d74c8", psychic: "#d64b78", bug: "#7f9a1e",
  rock: "#9a8433", ghost: "#5d4d8f", dragon: "#5a3dd1", dark: "#5a4a3e", steel: "#7c8794", fairy: "#c9619f",
};

export function typeColour(t: string) {
  return colours[t.toLowerCase()] ?? "#6f7a72";
}

export const slugify = (s: string) =>
  s.normalize("NFKD").replace(/[̀-ͯ]/g, "").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
