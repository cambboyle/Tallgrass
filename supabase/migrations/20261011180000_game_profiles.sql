-- The profile registry: game profiles (profiles/*.json in the repo) that
-- agents download, so a new or fixed profile reaches players without a new
-- release. The repo is the source of truth: the profiles-sync function
-- copies it here on every push to main. Anyone can read; nobody writes
-- directly.
create table public.game_profiles (
  slug        text primary key,                 -- file name without .json
  name        text not null,
  rom_sha1    text[] not null default '{}',
  version     integer not null default 1,       -- bumped whenever the profile changes
  profile     jsonb not null,
  updated_at  timestamptz not null default now()
);

alter table public.game_profiles enable row level security;

create policy "Anyone can read game profiles"
  on public.game_profiles for select
  to anon, authenticated
  using (true);
