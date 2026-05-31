CREATE TABLE IF NOT EXISTS release_groups (
    mb_id TEXT PRIMARY KEY,
    name TEXT,
    release_type TEXT,
    release_date TEXT,
    in_library INTEGER
);