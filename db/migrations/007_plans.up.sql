-- Phase 2: Plans and plan tokens.
-- The plan_type CHECK includes all 12 plan types for forward compatibility,
-- but only 3 are implemented in Phase 2.

CREATE TABLE plans (
  id                BIGSERIAL   PRIMARY KEY,
  game_id           BIGINT      NOT NULL REFERENCES games(id),
  plan_type         TEXT        NOT NULL CHECK (plan_type IN (
                      'make_demands','propose_decree','exchange_courtiers','make_war',
                      'make_introductions','seek_answers','chronicle_histories',
                      'clandestinely_liaise','spread_propaganda','spread_rumors',
                      'propose_duel','host_festivity'
                    )),
  category          TEXT        NOT NULL CHECK (category IN ('power','knowledge','esteem')),
  preparer_id       BIGINT      NOT NULL REFERENCES players(id),
  target_player_id  BIGINT      REFERENCES players(id),
  target_asset_id   BIGINT      REFERENCES assets(id),
  row_number        SMALLINT    NOT NULL,
  row_order         SMALLINT    NOT NULL DEFAULT 0,
  prepared_at_row   SMALLINT    NOT NULL,
  status            TEXT        NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','resolving','resolved','cancelled')),
  result            TEXT        CHECK (result IN ('make','mar')),
  resolved_at       TIMESTAMPTZ,
  preparation_notes TEXT,
  FOREIGN KEY (game_id, row_number)
    REFERENCES public_record_rows(game_id, row_number)
);

CREATE TABLE plan_tokens (
  id          BIGSERIAL   PRIMARY KEY,
  game_id     BIGINT      NOT NULL REFERENCES games(id),
  plan_type   TEXT        NOT NULL,
  player_id   BIGINT      NOT NULL REFERENCES players(id),
  plan_id     BIGINT      NOT NULL REFERENCES plans(id),
  placed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (game_id, plan_type, player_id)
);
