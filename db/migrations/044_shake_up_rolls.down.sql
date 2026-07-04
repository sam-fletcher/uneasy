-- 044_shake_up_rolls.down.sql
DROP INDEX IF EXISTS uq_one_shake_up_roll_per_category;
ALTER TABLE dice_rolls DROP COLUMN shake_up_category;
ALTER TABLE dice_rolls DROP CONSTRAINT dice_rolls_difficulty_check;
ALTER TABLE dice_rolls ADD CONSTRAINT dice_rolls_difficulty_check
  CHECK (difficulty BETWEEN 1 AND 6);
