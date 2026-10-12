-- Pokémon R.O.W.E. (2.1.1) joins the games the website documents. Game data
-- uploads are accepted only for ROMs listed here.
insert into public.games (slug, name, generation, rom_sha1) values
  ('rowe', 'Pokémon R.O.W.E.', 3, '{ab2196855ac47598cb2ef7ebff475d6ed17b1992}')
on conflict (slug) do nothing;
