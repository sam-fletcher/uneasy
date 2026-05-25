-- Migration 031: Dice roll stage machine + per-participant intent/ready,
-- authoritative cancellation pairing, ±1 difficulty votes, and the
-- banked-die face bug fix.

-- ── Stage column on dice_rolls ───────────────────────────────────────────
ALTER TABLE dice_rolls
  ADD COLUMN stage TEXT NOT NULL DEFAULT 'leverage'
    CHECK (stage IN ('decide_vote','voting','leverage','resolved'));

-- Backfill: pre-existing unresolved rolls keep the old "leverage coexists
-- with vote" behaviour (default), and resolved rolls jump to 'resolved'.
UPDATE dice_rolls SET stage = 'resolved' WHERE result IS NOT NULL;

-- ── Server-authoritative cancellation pairing ────────────────────────────
ALTER TABLE dice_roll_dice
  ADD COLUMN cancelled_by_die_id BIGINT REFERENCES dice_roll_dice(id);

-- ── Per-roll participants (intent + ready state) ─────────────────────────
CREATE TABLE dice_roll_participants (
  roll_id    BIGINT NOT NULL REFERENCES dice_rolls(id) ON DELETE CASCADE,
  player_id  BIGINT NOT NULL REFERENCES players(id)    ON DELETE CASCADE,
  intent     TEXT CHECK (intent IN ('aid','interfere')),
  is_ready   BOOLEAN NOT NULL DEFAULT FALSE,
  PRIMARY KEY (roll_id, player_id)
);

-- ── Difficulty votes: text yea/nay → SMALLINT ±1 ─────────────────────────
-- yea (easier) → -1, nay (harder) → +1.
ALTER TABLE difficulty_votes ADD COLUMN vote_int SMALLINT;
UPDATE difficulty_votes SET vote_int = CASE vote WHEN 'yea' THEN -1 ELSE 1 END;
ALTER TABLE difficulty_votes DROP CONSTRAINT IF EXISTS difficulty_votes_vote_check;
ALTER TABLE difficulty_votes DROP COLUMN vote;
ALTER TABLE difficulty_votes RENAME COLUMN vote_int TO vote;
ALTER TABLE difficulty_votes
  ALTER COLUMN vote SET NOT NULL,
  ADD CONSTRAINT difficulty_votes_vote_check CHECK (vote IN (1, -1));

-- ── Banked-die face bug fix ──────────────────────────────────────────────
-- The TTRPG rules don't support a pre-determined face on a banked die;
-- it should roll randomly at resolution like any other die.
ALTER TABLE banked_dice DROP COLUMN face;
ALTER TABLE liaise_choices DROP COLUMN banked_die_face;
