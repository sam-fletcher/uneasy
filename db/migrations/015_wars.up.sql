-- Migration 015: Wars (Phase 3d — Make War)
--
-- A war is created when a Make War plan is prepared. It persists across rows
-- until all active participants agree to peace, or one side has surrendered
-- unconditionally. Every row advance, each active participant pays the cost
-- of battle once per opponent on the opposite side, in reverse power order.

CREATE TABLE wars (
  id             BIGSERIAL PRIMARY KEY,
  game_id        BIGINT   NOT NULL REFERENCES games(id),
  origin_plan_id BIGINT   NOT NULL REFERENCES plans(id),
  status         TEXT     NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active', 'ended')),
  started_at_row SMALLINT NOT NULL,
  ended_at_row   SMALLINT,
  end_reason     TEXT CHECK (end_reason IN ('peace', 'surrender', 'all_surrendered')),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_wars_game_active ON wars(game_id) WHERE status = 'active';

CREATE TABLE war_participants (
  war_id              BIGINT   NOT NULL REFERENCES wars(id) ON DELETE CASCADE,
  player_id           BIGINT   NOT NULL REFERENCES players(id),
  side                SMALLINT NOT NULL CHECK (side IN (1, 2)),
  joined_at_row       SMALLINT NOT NULL,
  surrendered_at_row  SMALLINT,
  PRIMARY KEY (war_id, player_id)
);

CREATE INDEX idx_war_participants_player ON war_participants(player_id);

-- One row per (war, row, payer, opponent) cost paid. A payer must have one
-- row per opposing opponent for the current row before the row can advance.
CREATE TABLE war_battle_costs (
  id          BIGSERIAL PRIMARY KEY,
  war_id      BIGINT   NOT NULL REFERENCES wars(id) ON DELETE CASCADE,
  row_number  SMALLINT NOT NULL,
  payer_id    BIGINT   NOT NULL REFERENCES players(id),
  opponent_id BIGINT   NOT NULL REFERENCES players(id),
  choice      TEXT     NOT NULL CHECK (choice IN ('break_asset', 'leverage_two')),
  asset_id_1  BIGINT   REFERENCES assets(id),
  asset_id_2  BIGINT   REFERENCES assets(id),
  surrendered BOOLEAN  NOT NULL DEFAULT FALSE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (war_id, row_number, payer_id, opponent_id)
);

CREATE INDEX idx_war_battle_costs_lookup
  ON war_battle_costs(war_id, row_number, payer_id);

-- Peace proposals. At most one open proposal per war at a time (enforced in
-- application code; schema allows a history of closed proposals).
CREATE TABLE war_peace_proposals (
  id          BIGSERIAL PRIMARY KEY,
  war_id      BIGINT   NOT NULL REFERENCES wars(id) ON DELETE CASCADE,
  proposer_id BIGINT   NOT NULL REFERENCES players(id),
  terms       TEXT     NOT NULL,
  status      TEXT     NOT NULL DEFAULT 'open'
              CHECK (status IN ('open', 'accepted', 'rejected')),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ
);

CREATE INDEX idx_war_peace_proposals_open
  ON war_peace_proposals(war_id) WHERE status = 'open';

CREATE TABLE war_peace_votes (
  proposal_id BIGINT   NOT NULL REFERENCES war_peace_proposals(id) ON DELETE CASCADE,
  player_id   BIGINT   NOT NULL REFERENCES players(id),
  accepted    BOOLEAN  NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (proposal_id, player_id)
);
