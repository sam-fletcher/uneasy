-- Reverse of 001_phase1.up.sql
ALTER TABLE games DROP CONSTRAINT IF EXISTS fk_games_facilitator;
DROP TABLE IF EXISTS posts;
DROP TABLE IF EXISTS players;
DROP TABLE IF EXISTS games;
DROP TABLE IF EXISTS user_tokens;
