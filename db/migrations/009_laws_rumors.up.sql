-- Phase 2: Laws and rumors — placed under the public record.

CREATE TABLE laws (
  id             BIGSERIAL   PRIMARY KEY,
  game_id        BIGINT      NOT NULL REFERENCES games(id),
  text           TEXT        NOT NULL,
  addendum       TEXT,                              -- "but/and" from Propose Decree
  origin_plan_id BIGINT      REFERENCES plans(id),  -- NULL for prologue laws
  signatory_id   BIGINT      REFERENCES players(id),-- NULL for prologue laws
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  is_active      BOOLEAN     NOT NULL DEFAULT TRUE,
  display_order  SMALLINT    NOT NULL DEFAULT 0
);

CREATE TABLE rumors (
  id               BIGSERIAL   PRIMARY KEY,
  game_id          BIGINT      NOT NULL REFERENCES games(id),
  text             TEXT        NOT NULL,
  target_asset_id  BIGINT      REFERENCES assets(id),
  origin_plan_id   BIGINT      REFERENCES plans(id),  -- NULL for prologue rumors
  source_player_id BIGINT      REFERENCES players(id),-- NULL if hidden
  is_active        BOOLEAN     NOT NULL DEFAULT TRUE,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  display_order    SMALLINT    NOT NULL DEFAULT 0
);
