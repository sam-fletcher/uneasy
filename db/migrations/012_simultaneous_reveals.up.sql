-- Migration 012: Simultaneous Reveals (Phase 3c)
--
-- Used by Clandestinely Liaise (liaise_delay, liaise_redelay) and
-- Make War (make_war_delay) to coordinate multi-player secret die reveals.
-- The server holds each entry until ALL participants have submitted,
-- then broadcasts the complete result.

CREATE TABLE simultaneous_reveals (
  id            BIGSERIAL PRIMARY KEY,
  game_id       BIGINT   NOT NULL REFERENCES games(id),
  plan_id       BIGINT   REFERENCES plans(id),
  reveal_type   TEXT     NOT NULL CHECK (reveal_type IN (
                  'make_war_delay', 'liaise_delay', 'liaise_redelay'
                )),
  is_complete   BOOLEAN  NOT NULL DEFAULT FALSE,
  result_delay  SMALLINT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE simultaneous_reveal_entries (
  reveal_id    BIGINT   NOT NULL REFERENCES simultaneous_reveals(id) ON DELETE CASCADE,
  player_id    BIGINT   NOT NULL REFERENCES players(id),
  face         SMALLINT CHECK (face BETWEEN 0 AND 6),
  revealed_at  TIMESTAMPTZ,
  PRIMARY KEY (reveal_id, player_id)
);

-- Index to look up entries by player (used by the submit endpoint).
CREATE INDEX idx_reveal_entries_player ON simultaneous_reveal_entries(player_id);
