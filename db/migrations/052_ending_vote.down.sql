-- 052_ending_vote.down.sql
ALTER TABLE plans DROP COLUMN IF EXISTS is_finale_bonus;
DROP TABLE IF EXISTS ending_votes;
ALTER TABLE games DROP COLUMN IF EXISTS ending_vote_open;
