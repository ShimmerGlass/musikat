ALTER TABLE release_groups
DROP COLUMN releases_fetched_at;

DROP TABLE IF EXISTS release_group_releases;
