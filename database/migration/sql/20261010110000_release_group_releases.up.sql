CREATE TABLE release_group_releases (
    release_group_mb_id TEXT NOT NULL,
    release_mb_id TEXT NOT NULL,
    recording_ids TEXT NOT NULL,
    PRIMARY KEY (release_group_mb_id, release_mb_id)
);

ALTER TABLE release_groups
ADD COLUMN releases_fetched_at INTEGER;
