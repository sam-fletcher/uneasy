-- Migration 013: Liaise Choices and Banked Dice (Phase 3c)
--
-- liaise_choices records each player's "Things We Share" selection during
-- Clandestinely Liaise Phase 3. Choices are held until both players submit,
-- then revealed simultaneously via WebSocket.
--
-- banked_dice stores dice set aside via the Clandestinely Liaise
-- "leverage_partner" option. Banked dice can be spent on any future roll.

CREATE TABLE liaise_choices (
  id               BIGSERIAL PRIMARY KEY,
  plan_id          BIGINT   NOT NULL REFERENCES plans(id),
  player_id        BIGINT   NOT NULL REFERENCES players(id),
  choice           TEXT     NOT NULL CHECK (choice IN (
                     'look_at_secret', 'update_peer', 'break_peer',
                     'take_gift', 'leverage_partner'
                   )),
  target_asset_id  BIGINT   REFERENCES assets(id),
  banked_die_face  SMALLINT CHECK (banked_die_face BETWEEN 1 AND 6),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (plan_id, player_id)
);

CREATE TABLE banked_dice (
  id           BIGSERIAL PRIMARY KEY,
  game_id      BIGINT   NOT NULL REFERENCES games(id),
  player_id    BIGINT   NOT NULL REFERENCES players(id),
  face         SMALLINT NOT NULL CHECK (face BETWEEN 1 AND 6),
  source       TEXT     NOT NULL DEFAULT 'liaise',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  used_at      TIMESTAMPTZ,
  used_roll_id BIGINT   REFERENCES dice_rolls(id)
);

-- Index for looking up a player's available (unused) banked dice.
CREATE INDEX idx_banked_dice_player_unused ON banked_dice(game_id, player_id) WHERE used_at IS NULL;
