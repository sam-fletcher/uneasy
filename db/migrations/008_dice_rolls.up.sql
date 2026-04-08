-- Phase 2: Dice rolls — the universal resolution mechanic.

CREATE TABLE dice_rolls (
  id                  BIGSERIAL   PRIMARY KEY,
  game_id             BIGINT      NOT NULL REFERENCES games(id),
  plan_id             BIGINT      REFERENCES plans(id),    -- NULL for in-scene rolls
  row_number          SMALLINT,                             -- NULL during shake-up
  is_shake_up         BOOLEAN     NOT NULL DEFAULT FALSE,
  actor_id            BIGINT      NOT NULL REFERENCES players(id),
  difficulty          SMALLINT    NOT NULL CHECK (difficulty BETWEEN 1 AND 6),
  adjusted_difficulty SMALLINT,                             -- post-vote; NULL = no vote taken
  result              SMALLINT,                             -- distinct uncancelled faces; NULL until rolled
  outcome             TEXT        CHECK (outcome IN ('make','mar')),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at         TIMESTAMPTZ
);

-- Individual dice in the pool (actor base, leveraged, and interference).
CREATE TABLE dice_roll_dice (
  id                 BIGSERIAL PRIMARY KEY,
  roll_id            BIGINT   NOT NULL REFERENCES dice_rolls(id),
  player_id          BIGINT   NOT NULL REFERENCES players(id),
  is_interference    BOOLEAN  NOT NULL DEFAULT FALSE,
  leveraged_asset_id BIGINT   REFERENCES assets(id),   -- NULL for base dice
  face               SMALLINT CHECK (face BETWEEN 1 AND 6),  -- NULL until rolled
  is_cancelled       BOOLEAN  NOT NULL DEFAULT FALSE
);

-- Thumbs-up / thumbs-down difficulty vote.
CREATE TABLE difficulty_votes (
  roll_id    BIGINT      NOT NULL REFERENCES dice_rolls(id),
  player_id  BIGINT      NOT NULL REFERENCES players(id),
  vote       TEXT        NOT NULL CHECK (vote IN ('yea','nay')),
  voted_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (roll_id, player_id)
);
