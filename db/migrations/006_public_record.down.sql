-- Restore the Phase 1 flat posts table.
CREATE TABLE IF NOT EXISTS posts (
  id         BIGSERIAL   PRIMARY KEY,
  game_id    BIGINT      NOT NULL REFERENCES games(id),
  author_id  BIGINT      NOT NULL REFERENCES players(id),
  body       TEXT        NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS posts_game_id_created_at ON posts(game_id, created_at);

DROP TABLE IF EXISTS scene_entries;
DROP TABLE IF EXISTS scene_posts;
DROP TABLE IF EXISTS public_record_rows;
