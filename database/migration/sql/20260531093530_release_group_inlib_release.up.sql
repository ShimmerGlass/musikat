ALTER TABLE release_groups
ADD COLUMN in_library_release_mb_id TEXT;

UPDATE release_groups
SET in_library_release_mb_id = "";