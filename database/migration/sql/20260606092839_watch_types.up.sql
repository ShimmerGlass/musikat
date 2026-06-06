ALTER TABLE artist_watches
ADD COLUMN primary_types TEXT;

ALTER TABLE artist_watches
ADD COLUMN secondary_types TEXT;

UPDATE artist_watches SET
    primary_types = 'Album,EP,Single',
    secondary_types = '';