-- 024_prologue_committed_hearts.up.sql
-- "Maximum commitment" model for prologue ranking.
--
-- Replaces the count-based prologue_heart_declarations flow with
-- specific-card commitments plus a per-(player, track) "done" signal.
-- A player taps individual hearts to commit them to a track as their
-- "maximum if needed"; the server continuously computes which hearts
-- are doing work and which would be refunded. When every player marks
-- themselves done for the active track, the server resolves it,
-- locking in the bright (necessary) hearts and returning the grey
-- (wasted) hearts to each player's hand for the next track.
--
-- The old prologue_heart_declarations table is left in place for now
-- and will be dropped once the legacy DeclareHearts/FinalizeTrackRanking
-- handlers are retired.

CREATE TABLE prologue_committed_hearts (
  id          BIGSERIAL PRIMARY KEY,
  game_id     BIGINT      NOT NULL REFERENCES games(id),
  player_id   BIGINT      NOT NULL REFERENCES players(id),
  track       TEXT        NOT NULL CHECK (track IN ('power','knowledge','esteem')),
  card_id     BIGINT      NOT NULL REFERENCES player_cards(id) ON DELETE CASCADE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- A given card can be committed to at most one track at a time.
  UNIQUE (game_id, card_id)
);

CREATE INDEX prologue_committed_hearts_player_track_idx
  ON prologue_committed_hearts (game_id, player_id, track);

CREATE TABLE prologue_track_done (
  id          BIGSERIAL PRIMARY KEY,
  game_id     BIGINT      NOT NULL REFERENCES games(id),
  player_id   BIGINT      NOT NULL REFERENCES players(id),
  track       TEXT        NOT NULL CHECK (track IN ('power','knowledge','esteem')),
  done        BOOLEAN     NOT NULL DEFAULT FALSE,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (game_id, player_id, track)
);
