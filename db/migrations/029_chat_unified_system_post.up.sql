-- 029_chat_unified_system_post.up.sql
-- Unify the boundary/log distinction into a single "system post" concept
-- ordered by an integer severity. Adds scene_id so scene jumps in the
-- Public Record sidebar can anchor on the right scene.started post.
--
-- See PUBLIC_RECORD_SIDEBAR_SPEC.md, Part 2.
--
-- Schema before:
--   kind        TEXT  ('message' | 'log' | 'boundary')
--   severity    TEXT  ('minor' | 'default' | 'important')  nullable
--   system_code TEXT  set for log/boundary
--
-- Schema after:
--   severity    INTEGER NOT NULL  (0 for player messages; 10/25/50/75/100 for system)
--   system_code TEXT  set iff author_id IS NULL
--   scene_id    BIGINT  references scenes(id), nullable

-- ── Drop the old CHECK constraint and boundary index up front. ───────────
ALTER TABLE scene_posts DROP CONSTRAINT IF EXISTS scene_posts_author_kind_chk;
DROP INDEX IF EXISTS scene_posts_boundaries;

-- ── Add the new integer-severity column and backfill from kind/severity. ─
ALTER TABLE scene_posts
  ADD COLUMN severity_int INTEGER NOT NULL DEFAULT 0;

-- Boundaries are the structural anchors → severity 100 regardless of the
-- old severity text (which was always 'important' for boundaries anyway).
UPDATE scene_posts SET severity_int = 100 WHERE kind = 'boundary';

-- Logs map their string severity to the new ordered scale.
UPDATE scene_posts SET severity_int = 25 WHERE kind = 'log' AND severity = 'minor';
UPDATE scene_posts SET severity_int = 50 WHERE kind = 'log' AND severity = 'default';
UPDATE scene_posts SET severity_int = 75 WHERE kind = 'log' AND severity = 'important';

-- Player messages stay at the default 0; the chat filter will key off
-- author_id IS NOT NULL to always show them regardless of threshold.

-- ── Drop the old columns and rename the new one into place. ──────────────
ALTER TABLE scene_posts DROP COLUMN kind;
ALTER TABLE scene_posts DROP COLUMN severity;
ALTER TABLE scene_posts RENAME COLUMN severity_int TO severity;

-- ── Add scene_id, nullable; populated only on scene-scoped posts. ────────
ALTER TABLE scene_posts
  ADD COLUMN scene_id BIGINT REFERENCES scenes(id) ON DELETE SET NULL;

-- ── New invariant: system posts have system_code, player messages don't. ─
ALTER TABLE scene_posts
  ADD CONSTRAINT scene_posts_author_system_chk
    CHECK (
      (author_id IS NOT NULL AND system_code IS NULL)
      OR
      (author_id IS NULL AND system_code IS NOT NULL)
    );

-- Anchor-lookup index: jumping to a row/plan/scene in the rail asks for
-- the first system post with the matching id, which always has severity
-- 100. A partial index on the boundaries keeps that lookup cheap.
CREATE INDEX scene_posts_anchors
  ON scene_posts(game_id, id)
  WHERE severity = 100;
