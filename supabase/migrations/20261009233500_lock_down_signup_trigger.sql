-- The sign-up trigger runs as its owner; nobody should call it over the API.
revoke execute on function public.handle_new_user() from public, anon, authenticated;

comment on table public.pairing_codes is
  'Server-side only: read and written by the pairing Edge Functions with the service role. No client policies on purpose.';
