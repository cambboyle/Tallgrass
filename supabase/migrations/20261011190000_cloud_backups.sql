-- Optional cloud sync: devices that opted in upload their save backups and
-- Journey history here (through the backup-upload function, which checks
-- the device key). Private: each account can read and delete only its own
-- folder, <user id>/<device id>/...
insert into storage.buckets (id, name, public, file_size_limit)
values ('backups', 'backups', false, 4194304);

create policy "Owners can read their backups"
  on storage.objects for select
  to authenticated
  using (bucket_id = 'backups' and (storage.foldername(name))[1] = (select auth.uid())::text);

create policy "Owners can delete their backups"
  on storage.objects for delete
  to authenticated
  using (bucket_id = 'backups' and (storage.foldername(name))[1] = (select auth.uid())::text);
