CREATE INDEX IF NOT EXISTS idx_release_group_artists_release_group ON release_group_artists (release_group_mb_id);
CREATE INDEX IF NOT EXISTS idx_artist_watches_user ON artist_watches (user_id);
CREATE INDEX IF NOT EXISTS idx_release_group_notifications_release_group ON release_group_notifications (release_group_mb_id);
CREATE INDEX IF NOT EXISTS idx_release_groups_release_date ON release_groups (release_date);
