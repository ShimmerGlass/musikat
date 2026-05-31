CREATE TABLE IF NOT EXISTS release_group_artists (
    artist_mb_id TEXT,
    release_group_mb_id TEXT,

    UNIQUE (artist_mb_id, release_group_mb_id)
);