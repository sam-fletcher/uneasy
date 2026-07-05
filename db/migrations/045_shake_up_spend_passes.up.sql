-- 045_shake_up_spend_passes.up.sql
-- Shake-Up Overhaul Session 2: the reaction gate. After a spend is announced,
-- every other player still holding >=1 token must explicitly "let it stand"
-- (pass) before the spender may commit. Any new cost adjustment reopens the
-- window for everyone (ShakeUpAdjust deletes all pass rows for the spend).
--
-- FKs cascade ON DELETE straight through, matching migration 041's fix for
-- exactly this kind of grandchild table (spend_id -> shake_up_spends ->
-- games; player_id -> players -> games).
CREATE TABLE shake_up_spend_passes (
  id         BIGSERIAL PRIMARY KEY,
  spend_id   BIGINT      NOT NULL REFERENCES shake_up_spends(id) ON DELETE CASCADE,
  player_id  BIGINT      NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (spend_id, player_id)
);

CREATE INDEX shake_up_spend_passes_spend_idx ON shake_up_spend_passes (spend_id);
