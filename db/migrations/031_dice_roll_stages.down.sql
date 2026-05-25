-- Reverse migration 031. Banked-die face data is unrecoverable; the
-- column is re-added as nullable with no backfill.

-- Banked dice / liaise choices: re-add nullable face columns.
ALTER TABLE banked_dice    ADD COLUMN face SMALLINT CHECK (face BETWEEN 1 AND 6);
ALTER TABLE liaise_choices ADD COLUMN banked_die_face SMALLINT CHECK (banked_die_face BETWEEN 1 AND 6);

-- Difficulty votes: SMALLINT ±1 → text yea/nay (-1 → yea, 1 → nay).
ALTER TABLE difficulty_votes ADD COLUMN vote_text TEXT;
UPDATE difficulty_votes SET vote_text = CASE vote WHEN -1 THEN 'yea' ELSE 'nay' END;
ALTER TABLE difficulty_votes DROP CONSTRAINT IF EXISTS difficulty_votes_vote_check;
ALTER TABLE difficulty_votes DROP COLUMN vote;
ALTER TABLE difficulty_votes RENAME COLUMN vote_text TO vote;
ALTER TABLE difficulty_votes
  ALTER COLUMN vote SET NOT NULL,
  ADD CONSTRAINT difficulty_votes_vote_check CHECK (vote IN ('yea','nay'));

DROP TABLE IF EXISTS dice_roll_participants;

ALTER TABLE dice_roll_dice DROP COLUMN cancelled_by_die_id;

ALTER TABLE dice_rolls DROP COLUMN stage;
