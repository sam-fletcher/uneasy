-- 020_shake_up.up.sql
-- Phase 4c: the Shake-Up endgame.

ALTER TABLE games
  ADD COLUMN shake_up_category TEXT
    CHECK (shake_up_category IN ('esteem','knowledge','power')),
  ADD COLUMN shake_up_step SMALLINT
    CHECK (shake_up_step IS NULL OR shake_up_step IN (1, 2));

ALTER TABLE players
  ADD COLUMN shake_up_tokens SMALLINT NOT NULL DEFAULT 0;

-- shake_up_spends — one row per "I'm going to use a token to do X" announcement.
-- The spender pays at minimum 1 token at announcement time (immediate
-- commitment per the rulebook). Other players may spend their own tokens to
-- adjust the cost via shake_up_cost_adjustments. The spender locks in the
-- final cost by calling /commit (or the system finalizes if a turn-timeout
-- model is added later).
CREATE TABLE shake_up_spends (
  id               BIGSERIAL PRIMARY KEY,
  game_id          BIGINT      NOT NULL REFERENCES games(id),
  player_id        BIGINT      NOT NULL REFERENCES players(id),
  category         TEXT        NOT NULL CHECK (category IN ('esteem','knowledge','power')),
  option_key       TEXT        NOT NULL CHECK (option_key IN (
                     -- Esteem
                     'take_peer','take_artifact','break_resource','bump_knowledge',
                     -- Knowledge
                     'take_resource','break_holding','break_peer','bump_power',
                     -- Power
                     'take_holding','break_artifact','claim_title','bump_esteem'
                   )),
  target_asset_id  BIGINT      REFERENCES assets(id),
  target_player_id BIGINT      REFERENCES players(id),
  base_cost        SMALLINT    NOT NULL DEFAULT 1,
  final_cost       SMALLINT,            -- NULL until committed
  committed_at     TIMESTAMPTZ,         -- NULL until committed
  applied          BOOLEAN     NOT NULL DEFAULT FALSE,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX shake_up_spends_game_idx ON shake_up_spends (game_id);
CREATE INDEX shake_up_spends_open_idx ON shake_up_spends (game_id) WHERE committed_at IS NULL;

-- One adjustment row per ±1 nudge. Each row costs the bidder one token at
-- insert time. Rows accumulate while the spend is open; once committed,
-- final_cost = base_cost + SUM(adjustment).
CREATE TABLE shake_up_cost_adjustments (
  id         BIGSERIAL PRIMARY KEY,
  spend_id   BIGINT      NOT NULL REFERENCES shake_up_spends(id),
  player_id  BIGINT      NOT NULL REFERENCES players(id),
  adjustment SMALLINT    NOT NULL CHECK (adjustment IN (-1, 1)),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX shake_up_cost_adjustments_spend_idx ON shake_up_cost_adjustments (spend_id);
