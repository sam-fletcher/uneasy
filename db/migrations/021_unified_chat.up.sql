-- 021_unified_chat.up.sql
-- Transition scene_posts into a unified, game-wide chat feed that carries
-- three kinds of entries: free-text player messages, system-emitted boundary
-- markers (phase/row/plan/scene transitions), and (later) action-log entries.
--
-- See project_chat_ux memory and the chat redesign discussion. Per-row /
-- per-plan filter reads are retired in favor of a single chronological feed
-- ordered by id; row_number and plan_id remain on each row as metadata so
-- "jump to row N" / "jump to plan X" can be implemented client-side without
-- schema changes.

ALTER TABLE scene_posts
  ADD COLUMN kind        TEXT  NOT NULL DEFAULT 'message'
    CHECK (kind IN ('message', 'log', 'boundary')),
  ADD COLUMN severity    TEXT
    CHECK (severity IN ('minor', 'default', 'important')),
  ADD COLUMN system_code TEXT,
  ADD COLUMN system_data JSONB;

-- System-authored entries (boundaries, log) have no player author.
ALTER TABLE scene_posts
  ALTER COLUMN author_id DROP NOT NULL;

-- Author must be set iff the entry came from a player.
ALTER TABLE scene_posts
  ADD CONSTRAINT scene_posts_author_kind_chk
    CHECK (
      (kind = 'message' AND author_id IS NOT NULL AND system_code IS NULL)
      OR
      (kind IN ('log', 'boundary') AND system_code IS NOT NULL)
    );

-- Game-wide chronological feed: the new primary read pattern.
CREATE INDEX scene_posts_game_id ON scene_posts(game_id, id);

-- Boundary index for the future "jump to" table-of-contents query.
CREATE INDEX scene_posts_boundaries
  ON scene_posts(game_id, id)
  WHERE kind = 'boundary';

-- The old per-row composite index is no longer the hot path; drop it to keep
-- the table lean. Plan-scoped lookups, if ever needed, can use a partial
-- index added at the time.
DROP INDEX IF EXISTS scene_posts_game_row;
