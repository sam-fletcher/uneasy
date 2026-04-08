-- Phase 2: Public record rows, scene posts (replacing Phase 1 flat posts),
-- and scene entries (one-line summaries on the public record).

CREATE TABLE public_record_rows (
  game_id     BIGINT   NOT NULL REFERENCES games(id),
  row_number  SMALLINT NOT NULL CHECK (row_number BETWEEN 1 AND 13),
  PRIMARY KEY (game_id, row_number)
);

CREATE TABLE scene_posts (
  id          BIGSERIAL   PRIMARY KEY,
  game_id     BIGINT      NOT NULL REFERENCES games(id),
  row_number  SMALLINT,                          -- NULL for prologue / lobby scenes
  plan_id     BIGINT,                            -- NULL for open scenes
  author_id   BIGINT      NOT NULL REFERENCES players(id),
  body        TEXT        NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (game_id, row_number)
    REFERENCES public_record_rows(game_id, row_number)
);

CREATE INDEX scene_posts_game_row ON scene_posts(game_id, row_number, created_at);

CREATE TABLE scene_entries (
  id          BIGSERIAL   PRIMARY KEY,
  game_id     BIGINT      NOT NULL REFERENCES games(id),
  row_number  SMALLINT    NOT NULL,
  author_id   BIGINT      NOT NULL REFERENCES players(id),
  body        TEXT        NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (game_id, row_number)
    REFERENCES public_record_rows(game_id, row_number)
);

-- Drop the Phase 1 flat posts table — replaced by scene_posts.
DROP TABLE IF EXISTS posts;
