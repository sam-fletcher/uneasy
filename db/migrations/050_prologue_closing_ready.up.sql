-- 050_prologue_closing_ready.up.sql
-- adr/PROLOGUE_CLOSING_STAGE_PLAN.md Session 1: per-player ready flags for
-- the new `closing` prologue ranking step. Mirrors prologue_track_done
-- (024_prologue_committed_hearts.up.sql) but with ON DELETE CASCADE FKs per
-- the DeleteGame cascade audit (migration 041) — that audit's ruling was
-- that every prologue/game-owned table must cascade so DeleteGame doesn't
-- need a bespoke child-table list.

CREATE TABLE prologue_closing_ready (
  id          BIGSERIAL PRIMARY KEY,
  game_id     BIGINT      NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  player_id   BIGINT      NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  ready       BOOLEAN     NOT NULL DEFAULT FALSE,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (game_id, player_id)
);
