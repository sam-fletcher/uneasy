-- 044_shake_up_rolls.up.sql
-- Shake-Up Overhaul Session 1: real server-rolled dice for the shake-up's
-- step-1 rolls, replacing the self-reported textbox. A shake-up roll has no
-- meaningful difficulty (tokens = distinct faces, not a make/mar check), so
-- it needs an exemption from the difficulty CHECK, a category tag, and a
-- durable "already rolled this category" guard.

-- difficulty stays NOT NULL SMALLINT; 0 is the shake-up sentinel, never a
-- real difficulty (interactive rolls still require 1-6).
ALTER TABLE dice_rolls DROP CONSTRAINT dice_rolls_difficulty_check;
ALTER TABLE dice_rolls ADD CONSTRAINT dice_rolls_difficulty_check
  CHECK ((difficulty BETWEEN 1 AND 6) OR (is_shake_up AND difficulty = 0));

ALTER TABLE dice_rolls ADD COLUMN shake_up_category TEXT
  CHECK (shake_up_category IN ('esteem', 'knowledge', 'power'));

-- One shake-up roll per player per category — the durable guard that
-- replaces the "tokens > 0" proxy (which broke down once tokens could be
-- spent before every roll of a category resolved).
CREATE UNIQUE INDEX uq_one_shake_up_roll_per_category
  ON dice_rolls (game_id, actor_id, shake_up_category)
  WHERE is_shake_up;
