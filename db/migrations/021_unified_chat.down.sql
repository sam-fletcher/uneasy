-- 021_unified_chat.down.sql

DROP INDEX IF EXISTS scene_posts_boundaries;
DROP INDEX IF EXISTS scene_posts_game_id;
CREATE INDEX IF NOT EXISTS scene_posts_game_row
  ON scene_posts(game_id, row_number, created_at);

-- Rolling back is only safe if no system-authored rows exist.
DELETE FROM scene_posts WHERE author_id IS NULL;

ALTER TABLE scene_posts DROP CONSTRAINT IF EXISTS scene_posts_author_kind_chk;

ALTER TABLE scene_posts
  ALTER COLUMN author_id SET NOT NULL;

ALTER TABLE scene_posts
  DROP COLUMN IF EXISTS system_data,
  DROP COLUMN IF EXISTS system_code,
  DROP COLUMN IF EXISTS severity,
  DROP COLUMN IF EXISTS kind;
