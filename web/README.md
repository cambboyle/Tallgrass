# Tallgrass website

Next.js (App Router) site: sign-in (Supabase Auth: Discord, GitHub, email links), device linking, account, and game docs generated from uploaded game data exports.

```bash
cp .env.example .env.local   # fill in the Supabase URL and publishable/anon key
npm install
npm run dev                  # http://localhost:3000
```

To preview docs pages without uploading anything, point `GAMEDATA_DIR` at a folder of exports named `<game-slug>.json` (made with `tallgrass -export <rom>`):

```bash
GAMEDATA_DIR=/path/to/exports npm run dev
```

Deployed on Vercel with **Root Directory = `web`** and the two `NEXT_PUBLIC_*` variables from `.env.example`.
