ALTER TABLE artist_watches
ADD COLUMN added_at INTEGER;

UPDATE artist_watches
SET added_at = unixepoch();