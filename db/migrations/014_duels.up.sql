-- Migration 014: Duel tables (Phase 3d — Propose Duel).
--
-- Propose Duel stakes assets with a hidden die tucked under each, then runs
-- a series of comparison bouts until one side runs out of stakes.
-- Accumulated winning dice feed into the plan's final standard dice roll.
--
-- Hidden dice are readable by the *owner* of the staked asset (like a Secret
-- on an asset's underside) and by no one else until revealed during a bout.

CREATE TABLE duel_staked_assets (
  id           BIGSERIAL PRIMARY KEY,
  plan_id      BIGINT   NOT NULL REFERENCES plans(id),
  player_id    BIGINT   NOT NULL REFERENCES players(id),
  asset_id     BIGINT   NOT NULL REFERENCES assets(id),
  hidden_die   SMALLINT NOT NULL CHECK (hidden_die BETWEEN 1 AND 6),
  is_resolved  BOOLEAN  NOT NULL DEFAULT FALSE,
  is_winner    BOOLEAN,       -- NULL until bout resolves; true if this stake's die won its bout.
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (plan_id, asset_id)
);

CREATE INDEX idx_duel_stakes_plan ON duel_staked_assets(plan_id);

CREATE TABLE duel_bouts (
  id                  BIGSERIAL PRIMARY KEY,
  plan_id             BIGINT   NOT NULL REFERENCES plans(id),
  bout_number         SMALLINT NOT NULL,
  declarer_id         BIGINT   NOT NULL REFERENCES players(id),
  declarer_stake_id   BIGINT   NOT NULL REFERENCES duel_staked_assets(id),
  responder_id        BIGINT   NOT NULL REFERENCES players(id),
  responder_stake_id  BIGINT   REFERENCES duel_staked_assets(id),
  declaration         TEXT     CHECK (declaration IN ('high','low')),
  declarer_die        SMALLINT,
  responder_die       SMALLINT,
  winner_id           BIGINT   REFERENCES players(id),    -- NULL if match (tie)
  is_match            BOOLEAN  NOT NULL DEFAULT FALSE,    -- dice matched → both set aside
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at         TIMESTAMPTZ,
  UNIQUE (plan_id, bout_number)
);

CREATE INDEX idx_duel_bouts_plan ON duel_bouts(plan_id);
