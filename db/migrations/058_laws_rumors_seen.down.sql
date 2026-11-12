-- 058_laws_rumors_seen.down.sql
-- Drops the markers and the revision columns. Everything then reads as new once
-- the migration is re-applied, which costs a dot, nothing more.
DROP TABLE IF EXISTS player_laws_rumors_seen;
DROP INDEX IF EXISTS rumors_game_rev_idx;
DROP INDEX IF EXISTS laws_game_rev_idx;
ALTER TABLE rumors DROP COLUMN IF EXISTS rev;
ALTER TABLE laws DROP COLUMN IF EXISTS rev;
DROP SEQUENCE IF EXISTS laws_rumors_rev_seq;
