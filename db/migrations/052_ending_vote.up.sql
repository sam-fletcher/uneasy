-- 052_ending_vote.up.sql
-- adr/ENDGAME_VOTE_AND_FINALE_PLAN.md Session 1: the table vote that settles
-- how the game ends, pinned to the row 7 -> 8 advance.

-- The vote occupies the gap *inside* a row advance — row 7 is over, row 8 has
-- not begun — so current_row is still 7 for its whole duration. That makes
-- current_row useless as a discriminator: ComputeRowState cannot otherwise tell
-- "row 7 is finished and the advance is blocked" from "row 7 is in progress"
-- (both look like a focus player with no turn-scene). This flag is written by
-- the one code path that CAN tell the difference — the row-advance gate in
-- handler/turn.go — and read by ComputeRowState's step 2.5.
ALTER TABLE games ADD COLUMN ending_vote_open BOOLEAN NOT NULL DEFAULT false;

-- One row per player per game; the PK is the upsert conflict target, since a
-- player may change their vote freely while the window is open.
--
-- The CHECK deliberately omits 'long_campaign' (which games.ending_mode's own
-- CHECK does allow, see 002_game_phases.up.sql): Long Campaign is deferred and
-- rejected by the API, so no vote may name it. Clause 3 of the tie-break is
-- written and unit-tested against a three-option ballot anyway, so adding the
-- mode later is a one-line CHECK change with no tie-break work.
--
-- ON DELETE CASCADE on both FKs, per the DeleteGame cascade audit (migration
-- 041): every game-owned table must cascade so DeleteGame stays a single
-- DELETE FROM games with no bespoke child-table list.
CREATE TABLE ending_votes (
  game_id    BIGINT      NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  player_id  BIGINT      NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  mode       TEXT        NOT NULL CHECK (mode IN ('smooth_landing','explosive_finale')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (game_id, player_id)
);

-- Explosive Finale's one-shortened-delay-plan-per-player slot. A player's slot
-- is spent iff they hold a plan with is_finale_bonus = true whose status is not
-- 'cancelled' — derived from plans rather than a flag on players so it stays
-- auditable. Written by Session 3 (preparation-time clamp, and the dice-delay
-- collapse at reveal time); the column lands here with the rest of the
-- migration so there is one migration for the feature.
ALTER TABLE plans ADD COLUMN is_finale_bonus BOOLEAN NOT NULL DEFAULT false;
