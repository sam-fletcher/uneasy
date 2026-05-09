-- 029_chat_unified_system_post.down.sql
-- Reverses 029_chat_unified_system_post.up.sql.

ALTER TABLE scene_posts DROP CONSTRAINT IF EXISTS scene_posts_author_system_chk;
DROP INDEX IF EXISTS scene_posts_anchors;
ALTER TABLE scene_posts DROP COLUMN scene_id;

ALTER TABLE scene_posts RENAME COLUMN severity TO severity_int;

ALTER TABLE scene_posts
  ADD COLUMN kind TEXT NOT NULL DEFAULT 'message'
    CHECK (kind IN ('message', 'log', 'boundary'));
ALTER TABLE scene_posts
  ADD COLUMN severity TEXT
    CHECK (severity IN ('minor', 'default', 'important'));

-- Reconstitute kind/severity from the integer scale.
-- author_id IS NOT NULL ⇒ player message; severity stays NULL.
-- severity_int = 100 ⇒ boundary; severity = 'important'.
-- otherwise system post ⇒ log; severity from the integer scale.
UPDATE scene_posts SET kind = 'boundary', severity = 'important'
  WHERE author_id IS NULL AND severity_int = 100;
UPDATE scene_posts SET kind = 'log', severity = 'minor'
  WHERE author_id IS NULL AND severity_int < 50 AND severity_int <> 100 AND severity_int > 0;
UPDATE scene_posts SET kind = 'log', severity = 'default'
  WHERE author_id IS NULL AND severity_int >= 50 AND severity_int < 75;
UPDATE scene_posts SET kind = 'log', severity = 'important'
  WHERE author_id IS NULL AND severity_int >= 75 AND severity_int < 100;

ALTER TABLE scene_posts DROP COLUMN severity_int;

ALTER TABLE scene_posts
  ADD CONSTRAINT scene_posts_author_kind_chk
    CHECK (
      (kind = 'message' AND author_id IS NOT NULL AND system_code IS NULL)
      OR
      (kind IN ('log', 'boundary') AND system_code IS NOT NULL)
    );

CREATE INDEX scene_posts_boundaries
  ON scene_posts(game_id, id)
  WHERE kind = 'boundary';
