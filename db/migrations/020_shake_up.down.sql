-- 020_shake_up.down.sql

DROP TABLE IF EXISTS shake_up_cost_adjustments;
DROP TABLE IF EXISTS shake_up_spends;

ALTER TABLE players DROP COLUMN IF EXISTS shake_up_tokens;

ALTER TABLE games
  DROP COLUMN IF EXISTS shake_up_step,
  DROP COLUMN IF EXISTS shake_up_category;
