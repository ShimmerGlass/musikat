ALTER TABLE release_groups
RENAME COLUMN release_type TO primary_type;

ALTER TABLE release_groups
ADD COLUMN secondary_type TEXT;

UPDATE release_groups
SET secondary_type = "";