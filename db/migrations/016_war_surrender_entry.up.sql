-- Migration 016: Make War — surrender claims & late-joiner entry payments.
--
-- Adds:
--   * war_participants.entry_payment_complete — FALSE for players who
--     joined a war that was already active. They become full participants
--     (counted for cost of battle, peace voting, etc.) once they have paid
--     one break/leverage per existing opposing opponent.
--   * war_battle_costs.is_entry — TRUE when a battle-cost row records an
--     entry payment rather than a per-row cost.
--   * war_surrender_claims — one unfulfilled row per opposing non-surrendered
--     opponent at the moment a player surrenders. Each opponent takes one
--     asset from the surrendered player to fulfill their claim.

ALTER TABLE war_participants
  ADD COLUMN entry_payment_complete BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE war_battle_costs
  ADD COLUMN is_entry BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE war_surrender_claims (
  id              BIGSERIAL PRIMARY KEY,
  war_id          BIGINT   NOT NULL REFERENCES wars(id) ON DELETE CASCADE,
  surrendered_id  BIGINT   NOT NULL REFERENCES players(id),
  claimant_id     BIGINT   NOT NULL REFERENCES players(id),
  asset_id        BIGINT   REFERENCES assets(id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  fulfilled_at    TIMESTAMPTZ,
  UNIQUE (war_id, surrendered_id, claimant_id)
);

CREATE INDEX idx_war_surrender_claims_open
  ON war_surrender_claims(war_id) WHERE fulfilled_at IS NULL;
