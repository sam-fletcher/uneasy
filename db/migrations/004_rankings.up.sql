-- Phase 2: Rankings — three tracks (power, knowledge, esteem), positions 1–5.

CREATE TABLE rankings (
  id          BIGSERIAL PRIMARY KEY,
  game_id     BIGINT   NOT NULL REFERENCES games(id),
  player_id   BIGINT   REFERENCES players(id),   -- NULL = dummy token
  category    TEXT     NOT NULL CHECK (category IN ('power','knowledge','esteem')),
  rank        SMALLINT NOT NULL CHECK (rank BETWEEN 1 AND 5),
  UNIQUE (game_id, category, rank)
);
