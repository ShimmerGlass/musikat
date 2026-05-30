CREATE TABLE IF NOT EXISTS release_group_notifications (
    user_id TEXT,
    release_group_mb_id TEXT,

    UNIQUE (user_id, release_group_mb_id)
);