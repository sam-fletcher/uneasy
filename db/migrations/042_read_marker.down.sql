-- 042_read_marker.down.sql
ALTER TABLE players DROP COLUMN IF EXISTS last_read_post_id;
