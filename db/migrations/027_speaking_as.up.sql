-- 027_speaking_as.up.sql
-- Per-message character attribution for chat. NULL means OOC (or system).
-- The application validates that the asset is one the author is allowed to
-- speak as in the currently-active scene.

ALTER TABLE scene_posts
  ADD COLUMN speaking_as_asset_id BIGINT REFERENCES assets(id);
