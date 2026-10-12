// Player guides for the games Tallgrass focuses on. They're written from each
// hack's public information (linked below; the hacks' own threads and docs
// are the authority) plus what Tallgrass has verified by reading the games.
// Prose only: no game files, text or sprites are copied here.

export type GuideLink = { label: string; url: string; note: string };
export type Guide = {
  tagline: string;
  author: string;
  base: string; // what it's built on
  version: string; // as of when the guide was written
  about: string[];
  highlights: string[];
  withTallgrass: string[];
  caveats: string[];
  start: string[]; // numbered steps
  links: GuideLink[];
};

export const GUIDES: Record<string, Guide> = {
  "radical-red": {
    tagline: "A difficulty hack of FireRed with modern mechanics and Pokémon up to Gen 9.",
    author: "soupercell",
    base: "Pokémon FireRed (GBA), on the Custom FireRed Upgrade (CFRU) engine",
    version: "4.1 (March 2024)",
    about: [
      "Radical Red is built for players who want FireRed to fight back. Trainers are far tougher, and you get modern tools to match: later-generation mechanics, a much bigger Pokémon pool, and optional modes that raise or lower the challenge.",
    ],
    highlights: [
      "Pokémon, moves and abilities from later generations, up to Gen 9, with updated sprites",
      "Mega Evolutions and Z-Moves, and Dynamax in Wish Piece raid battles",
      "Physical/special split and the Fairy type",
      "Reusable TMs and an expanded TM list",
      "DexNav, which lets you search for Pokémon with hidden abilities",
      "A built-in randomizer, and optional difficulty modes (see the official thread for the current list)",
    ],
    withTallgrass: [
      "Live party: species, level, HP, nature, IVs, EVs and moves, read from the running game",
      "Route encounters for the map you're on, by time of day (day, and evening & night), with what you've seen and caught marked",
      "The Journey: catches, faints and new routes logged automatically, with an optional Nuzlocke view",
      "All 25 PC boxes, live and from save files",
      "Every save backed up as a new version, so a bad save can be rolled back",
      "Trainers, items and maps documented on this site",
    ],
    caveats: [
      "Hidden abilities show the regular ability, because the game's hidden-ability flag isn't decoded yet.",
      "Encounters changed by a story script aren't shown.",
    ],
    start: [
      "Get the patch from the official thread. Tallgrass doesn't download or share ROMs: apply the patch to your own copy of FireRed. The patcher asks for a specific FireRed (USA) dump; the thread says which.",
      "Put the patched ROM in your RetroPie `gba` folder.",
      "Open Tallgrass → Setup, choose Radical Red, and follow the checks. Tallgrass recognises the ROM by its checksum; if the Library page says \"not supported yet\", you have a different version.",
      "Play. Your party shows up on the dashboard, and your saves are backed up as you go.",
    ],
    links: [
      { label: "Radical Red on PokéCommunity", url: "https://www.pokecommunity.com/threads/pok%C3%A9mon-radical-red-version-4-1-released-gen-9-dlc-pokemon-character-customization-now-available.437688/", note: "The official thread: releases and updates. The hack also has an official online patcher, documentation of every change, and a Discord server." },
      { label: "OpenHome", url: "https://github.com/andrewbenington/OpenHome", note: "A save editor and Pokémon bank that supports Radical Red saves. See the guide's editing section." },
    ],
  },

  unbound: {
    tagline: "A FireRed hack with four difficulty settings, raids, missions and plenty of quality-of-life.",
    author: "Skeli",
    base: "Pokémon FireRed (GBA), on the CFRU engine, which Skeli also made",
    version: "Completed in December 2020, with updates since",
    about: [
      "Pokémon Unbound began development in 2016 and is a full game with its own region, story and postgame. It runs on the same engine as Radical Red, so the two share most mechanics, and Tallgrass handles them alike.",
    ],
    highlights: [
      "Four difficulty options, from Vanilla (close to a traditional game) up to Insane",
      "Mechanics from later generations, up to Gen 8: the physical/special split, the Fairy type, Mega Evolutions, Z-Moves and Dynamax",
      "Unlimited bag space and auto-run",
      "DexNav and daily raids",
      "Over 80 missions to complete",
    ],
    withTallgrass: [
      "Live party with IVs, EVs, nature and moves",
      "Route encounters by time of day (morning, day and night, matching the game's own schedule), with seen and caught marks",
      "The Journey with an optional Nuzlocke view",
      "All 25 PC boxes, live and from save files",
      "Versioned save backups",
      "Trainers, items and maps documented on this site",
    ],
    caveats: [
      "Hidden abilities show the regular ability, because the hidden-ability flag isn't decoded yet.",
      "Only some trainers are placed on maps so far; all are listed.",
    ],
    start: [
      "Get the game from the official thread. Tallgrass doesn't download or share ROMs: use a patch on your own copy of FireRed, as the thread describes.",
      "Put the ROM in your RetroPie `gba` folder.",
      "Open Tallgrass → Setup and choose Unbound.",
      "Bringing a save from another emulator? Setup can convert a `.sav` to the `.srm` RetroArch reads. It keeps your old files and trims the copy to the right size.",
    ],
    links: [
      { label: "Pokémon Unbound on PokéCommunity", url: "https://www.pokecommunity.com/threads/pok%C3%A9mon-unbound-completed.382178/", note: "The official thread, with downloads and updates." },
      { label: "Unbound community wiki", url: "https://pokemonunbound.fandom.com/wiki/", note: "Fan-maintained pages on locations, Pokémon, missions and difficulty." },
      { label: "OpenHome", url: "https://github.com/andrewbenington/OpenHome", note: "A save editor and Pokémon bank that supports Unbound saves. See the guide's editing section." },
    ],
  },

  "heartgold-generations": {
    tagline: "HeartGold rebuilt with every generation's Pokémon and modern mechanics.",
    author: "spearmintz",
    base: "Pokémon HeartGold (DS), on BluRosie's hg-engine",
    version: "2.0 (September 2025)",
    about: [
      "HeartGold Generations keeps Johto and Kanto, and rebuilds the game around a much larger Pokémon roster and modern battle rules. hg-engine, the engine under it, is a community overhaul of English HeartGold: more Pokémon, abilities, moves and items, Mega Evolutions, the Fairy type, hidden abilities and 30 PC boxes.",
    ],
    highlights: [
      "Around 950 Pokémon from over 500 families, across all nine generations, with trade and cross-generation evolutions available",
      "Mega Evolution, and a level cap to stop over-levelling",
      "Infinite Rare Candy and Pocket Heal for grinding, and a portable PC on the L button",
      "Buyable nature mints and ability capsules",
      "Type changes for underused Pokémon",
      "Free Heart Scales after 7 gyms and free Master Balls after 8",
      "An unlocked 60 fps frame rate, and the Kanto gym leaders as an endgame boss rush",
    ],
    withTallgrass: [
      "Live party, read straight from DraStic's memory while you play (DS games need no RetroArch commands)",
      "Your location and the route's wild encounters, with seen and caught marks from the Pokédex",
      "All 30 PC boxes, live",
      "The Journey, including PC catches with real names and levels",
      "Versioned backups of your `.dsv` save",
      "Trainers, shops and items documented on this site, including this hack's own shop stock",
    ],
    caveats: [
      "DS memory moves while the game runs, so roughly one live read in ten fails. Tallgrass keeps the last good party, so you shouldn't notice.",
      "OpenHome doesn't support this hack's saves.",
    ],
    start: [
      "Get the hack from the official thread. Tallgrass doesn't download or share ROMs: patch your own copy of English HeartGold, as the thread describes.",
      "Put the ROM in your RetroPie `nds` folder, and play it in DraStic.",
      "Open Tallgrass → Setup and choose HeartGold Generations. It checks that DraStic can be read by Tallgrass (the `ptrace_scope` check) and finds your save in DraStic's backup folder.",
      "Start the game and load your save. The party appears on the dashboard within a few seconds.",
    ],
    links: [
      { label: "HeartGold Generations on PokéCommunity", url: "https://www.pokecommunity.com/threads/pok%C3%A9mon-heartgold-generations-v2-0.537551/", note: "The official thread, with downloads and updates." },
      { label: "hg-engine", url: "https://github.com/BluRosie/hg-engine", note: "The engine the hack is built on." },
    ],
  },

  rowe: {
    tagline: "An open-world Hoenn with 16 badges, every Pokémon from Gen 1 to 9, and a pile of game modes.",
    author: "Rioluwott",
    base: "Pokémon Emerald (GBA), built from the pokeemerald decomp",
    version: "The Mega Update, 2.1.1",
    about: [
      "R.O.W.E. (a wide-open sandbox) turns Hoenn, and the Sevii Islands, into a game you can tackle in any order. Pokémon from all nine generations are available from the start, and the game scales levels so no area is out of reach, so building a team doesn't need breeding or grinding.",
      "It's built from the decomp rather than on CFRU, so it stores Pokémon in its own format. Tallgrass reads that directly, which is why it has its own profile.",
    ],
    highlights: [
      "16 badges, and the Sevii Islands, which you need to finish the game",
      "Gen 1–9 Pokémon from the start, with regional forms, Mega Evolution, DexNav, reusable TMs and nature mints",
      "Difficulty options (Easy, Normal and Hard; once started you can only lower it) with level caps tied to gym progress",
      "Grindless Mode: levels are replaced by stats scaled to your badge count, and you set EVs yourself on the Summary screen",
      "Trainer Skills: spend points on upgrades such as Stat Boosts, which give every Pokémon you own IVs, and bonus-battle and XP skills",
      "Delta Boost (from the Mega Update): Pokémon that can't Mega Evolve can boost with a Delta Orb",
      "Many optional modes, such as Double Battle, No EVs, Inverse, Monotype, Little Cup, randomizers and Gym Shuffle (the hack's docs list them all)",
      "New Game Plus, which restarts you at level 1 but keeps stats, natures and items",
    ],
    withTallgrass: [
      "Live party with levels, moves, nature and EVs. IVs are shown with a star, because the game gives every Pokémon the same IVs from your Stat Boost skills (1 at skill level 0, up to 31 at level 10), and Tallgrass reads those skills",
      "Forms are shown as their own species (Alolan Meowth, not Meowth), and ability names appear",
      "Route encounters by time of day, including DexNav's hidden encounters and Headbutt trees, with seen and caught marks",
      "Grindless Mode levels are worked out the way the game does it, for your party and boxes",
      "All 21 PC boxes, live and from save files",
      "The Journey, including PC catches",
      "Species, learnsets, evolutions and about 940 trainers documented on this site, including each trainer's second team where the game has one",
    ],
    caveats: [
      "Trainer levels scale during battle, so the trainers pages show \"scales\" instead of a level.",
      "Gym leaders aren't placed on the maps yet (their battles start differently), though they're listed.",
      "Tallgrass can't tell when the game is using its alternate team for a trainer, and shows both.",
      "OpenHome doesn't support R.O.W.E. saves.",
    ],
    start: [
      "Get the game from the official thread. Tallgrass doesn't download or share ROMs: use the patch or build the thread points to, on your own copy of Emerald.",
      "Put the ROM in your RetroPie `gba` folder. Tallgrass recognises version 2.1.1 by its checksum, so another version may show as \"not supported yet\" on the Library page.",
      "Open Tallgrass → Setup, choose Pokémon R.O.W.E., and follow the checks. It also sets up RetroArch's network commands and autosave if they're off.",
      "Play. Your party shows up on the dashboard, and the game's saves are backed up as they change.",
    ],
    links: [
      { label: "R.O.W.E. 2.0 on PokéCommunity", url: "https://www.pokecommunity.com/threads/pokemon-r-o-w-e-2-0-an-open-world-version-of-pok%C3%A9mon-emerald-gen-9-16-badges-sevii-islands-following-pok%C3%A9mon-costumes-and-much-more.442592/", note: "The official thread: releases and updates. The hack's own FAQ, game modes and Trainer Skills pages are in a spreadsheet linked from it, and the game has an Information option in the Start menu." },
    ],
  },
};

export const hasGuide = (slug: string) => slug in GUIDES;
