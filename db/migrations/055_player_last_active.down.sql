-- 055_player_last_active.down.sql
-- Drops the activity timestamp. Nothing else reads it (the Retinue header
-- degrades to hub presence alone), and it is observational data with no
-- gameplay meaning, so there is nothing to preserve or back-fill.
ALTER TABLE players DROP COLUMN last_active_at;
