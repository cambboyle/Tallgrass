-- Tallgrass cloud schema, version 1: accounts, paired devices, supported
-- games and uploaded game data (facts read from ROMs; no game assets).

-- Profiles: one per account, created on sign-up.
create table public.profiles (
  id uuid primary key references auth.users (id) on delete cascade,
  display_name text,
  is_maintainer boolean not null default false, -- may upload game data
  created_at timestamptz not null default now()
);
alter table public.profiles enable row level security;

create policy "Profiles are visible to their owner"
  on public.profiles for select to authenticated
  using ((select auth.uid()) = id);

create policy "Owners can rename themselves"
  on public.profiles for update to authenticated
  using ((select auth.uid()) = id)
  with check ((select auth.uid()) = id);

-- Only display_name may change from the client; is_maintainer is set by hand.
revoke update on public.profiles from authenticated;
grant update (display_name) on public.profiles to authenticated;

create function public.handle_new_user()
returns trigger
language plpgsql
security definer
set search_path = ''
as $$
begin
  insert into public.profiles (id, display_name)
  values (
    new.id,
    coalesce(new.raw_user_meta_data ->> 'full_name', new.raw_user_meta_data ->> 'name', split_part(new.email, '@', 1))
  );
  return new;
end;
$$;

create trigger on_auth_user_created
  after insert on auth.users
  for each row execute function public.handle_new_user();

-- Devices: agents linked to an account. The agent holds a random key; we
-- keep only its SHA-256.
create table public.devices (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references auth.users (id) on delete cascade,
  name text not null check (char_length(name) between 1 and 64),
  token_hash text not null unique,
  created_at timestamptz not null default now(),
  last_seen_at timestamptz
);
create index devices_user_id_idx on public.devices (user_id);
alter table public.devices enable row level security;

create policy "Devices are visible to their owner"
  on public.devices for select to authenticated
  using ((select auth.uid()) = user_id);

create policy "Owners can unlink devices"
  on public.devices for delete to authenticated
  using ((select auth.uid()) = user_id);

-- Pairing codes: the agent asks for a code, the user enters it on the
-- website, the agent collects its key. Server-side only (no policies).
create table public.pairing_codes (
  code text primary key,
  device_name text not null,
  poll_secret_hash text not null,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null default now() + interval '10 minutes',
  claimed_by uuid references auth.users (id) on delete cascade,
  device_id uuid references public.devices (id) on delete cascade,
  device_token text -- held only until the agent collects it
);
alter table public.pairing_codes enable row level security;

-- Games Tallgrass supports, identified by ROM SHA-1.
create table public.games (
  slug text primary key,
  name text not null,
  generation int not null,
  rom_sha1 text[] not null default '{}',
  created_at timestamptz not null default now()
);
alter table public.games enable row level security;

create policy "Games are public"
  on public.games for select to anon, authenticated
  using (true);

insert into public.games (slug, name, generation, rom_sha1) values
  ('radical-red', 'Pokémon Radical Red', 3, '{964f951a0fdaf209e4ea1344883ef0d557bb3a80}'),
  ('unbound', 'Pokémon Unbound', 3, '{b4776b82a4c7915d0fadeaa27e013523f99dfd94}'),
  ('heartgold-generations', 'HeartGold Generations', 4, '{d8fcc9de74ecc7e38caed8be883bd602653c3736}');

-- Game data: exports uploaded by maintainers (docs/gamedata-export.md).
create table public.game_data (
  id bigint generated always as identity primary key,
  game_slug text not null references public.games (slug),
  rom_sha1 text not null,
  export_version int not null,
  storage_path text not null,
  species_count int not null,
  trainer_count int not null,
  map_count int not null,
  uploaded_by uuid references auth.users (id) on delete set null,
  created_at timestamptz not null default now()
);
create index game_data_game_slug_idx on public.game_data (game_slug, created_at desc);
create index game_data_uploaded_by_idx on public.game_data (uploaded_by);
alter table public.game_data enable row level security;

create policy "Game data is public"
  on public.game_data for select to anon, authenticated
  using (true);

-- Export files: public to read; written only by the upload function.
insert into storage.buckets (id, name, public, file_size_limit, allowed_mime_types)
values ('gamedata', 'gamedata', true, 20971520, array['application/json']);
