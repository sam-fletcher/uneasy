-- Recreate the legacy count-based heart declarations table (mirrors the
-- definition originally introduced in migration 019_prologue).
CREATE TABLE prologue_heart_declarations (
  id         BIGSERIAL PRIMARY KEY,
  game_id    BIGINT      NOT NULL REFERENCES games(id),
  player_id  BIGINT      NOT NULL REFERENCES players(id),
  track      TEXT        NOT NULL CHECK (track IN ('power','knowledge','esteem')),
  count      SMALLINT    NOT NULL CHECK (count >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (game_id, player_id, track)
);
