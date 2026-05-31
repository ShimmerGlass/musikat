CREATE TABLE IF NOT EXISTS artist_watches (
    artist_mb_id TEXT,
    user_id TEXT,
    source TEXT,

    UNIQUE (artist_mb_id, user_id),

    FOREIGN KEY (artist_mb_id)
        REFERENCES artists(mb_id),

    FOREIGN KEY (user_id)
        REFERENCES users(id)
);