CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    name TEXT,
    password TEXT,
    subsonic_user TEXT,
    subsonic_pass TEXT,
    xmpp_jid TEXT
);