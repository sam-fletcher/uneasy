-- sqlc query file for dice rolls.

-- ── Dice Rolls ───────────────────────────────────────────────────────

-- name: CreateDiceRoll :one
INSERT INTO dice_rolls (game_id, plan_id, row_number, actor_id, difficulty, stage)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetDiceRollByID :one
SELECT * FROM dice_rolls WHERE id = $1;

-- name: SetDiceRollStage :exec
UPDATE dice_rolls SET stage = $2 WHERE id = $1;

-- name: SetDiceRollAdjustedDifficulty :exec
UPDATE dice_rolls SET adjusted_difficulty = $2 WHERE id = $1;

-- name: ResolveDiceRoll :exec
UPDATE dice_rolls
SET result = $2, outcome = $3, resolved_at = now(), stage = 'resolved'
WHERE id = $1;

-- name: ListDiceRollsByGame :many
SELECT * FROM dice_rolls WHERE game_id = $1 ORDER BY created_at ASC;

-- name: GetOpenRollByGame :one
-- The game's in-flight interactive roll, if any. Mirrors the
-- uq_one_open_roll_per_game index: shake-up rolls (a separate mechanic tracked
-- by GetOpenShakeUpRollByGame instead) are excluded, so this returns only a
-- genuine interactive roll still awaiting resolution.
SELECT * FROM dice_rolls
WHERE game_id = $1 AND resolved_at IS NULL AND is_shake_up = FALSE
ORDER BY created_at DESC
LIMIT 1;

-- name: CreateShakeUpDiceRoll :one
-- One player's step-1 roll for the current category: no difficulty (0 is the
-- shake-up sentinel), no plan/row association. Token gain is the distinct-face
-- result, computed at resolution like any other roll.
INSERT INTO dice_rolls (game_id, actor_id, difficulty, stage, is_shake_up, shake_up_category)
VALUES ($1, $2, 0, 'leverage', TRUE, $3)
RETURNING *;

-- name: GetOpenShakeUpRollByGame :one
-- The game's in-flight shake-up roll, if any. Step 1 rolls happen one player
-- at a time (reverse rank order), so at most one shake-up roll is open per
-- game.
SELECT * FROM dice_rolls
WHERE game_id = $1 AND is_shake_up = TRUE AND resolved_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: GetDiceRollByPlanID :one
-- Returns the most recent dice roll for a given plan (used during resolution).
SELECT * FROM dice_rolls WHERE plan_id = $1 ORDER BY created_at DESC LIMIT 1;

-- ── Dice Roll Dice ───────────────────────────────────────────────────

-- name: CreateDiceRollDie :one
INSERT INTO dice_roll_dice (roll_id, player_id, is_interference, leveraged_asset_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListDiceByRoll :many
SELECT * FROM dice_roll_dice WHERE roll_id = $1 ORDER BY id;

-- name: SetDieFace :exec
UPDATE dice_roll_dice SET face = $2 WHERE id = $1;

-- name: SetDieCancelled :exec
UPDATE dice_roll_dice SET is_cancelled = TRUE WHERE id = $1;

-- name: SetDieCancelledBy :exec
UPDATE dice_roll_dice
SET is_cancelled = TRUE, cancelled_by_die_id = $2
WHERE id = $1;

-- ── Dice Roll Participants ───────────────────────────────────────────

-- name: CreateRollParticipant :exec
INSERT INTO dice_roll_participants (roll_id, player_id, intent, is_ready)
VALUES ($1, $2, $3, $4);

-- name: ListParticipantsByRoll :many
SELECT * FROM dice_roll_participants WHERE roll_id = $1 ORDER BY player_id;

-- name: GetParticipant :one
SELECT * FROM dice_roll_participants WHERE roll_id = $1 AND player_id = $2;

-- name: SetParticipantIntent :exec
UPDATE dice_roll_participants
SET intent = $3
WHERE roll_id = $1 AND player_id = $2;

-- name: SetParticipantReady :exec
UPDATE dice_roll_participants
SET is_ready = $3
WHERE roll_id = $1 AND player_id = $2;

-- name: SetAllParticipantsReady :exec
UPDATE dice_roll_participants SET is_ready = TRUE WHERE roll_id = $1;

-- ── Difficulty Votes (SMALLINT ±1) ───────────────────────────────────

-- name: CreateDifficultyVote :exec
INSERT INTO difficulty_votes (roll_id, player_id, vote)
VALUES ($1, $2, $3)
ON CONFLICT (roll_id, player_id) DO UPDATE SET vote = EXCLUDED.vote;

-- name: ListVotesByRoll :many
SELECT * FROM difficulty_votes WHERE roll_id = $1 ORDER BY voted_at;

-- name: SumVotesByRoll :one
SELECT
  count(*)::bigint               AS vote_count,
  coalesce(sum(vote), 0)::bigint AS vote_sum
FROM difficulty_votes WHERE roll_id = $1;

-- name: ListInterferenceDiceByRoll :many
-- Returns each player who contributed interference dice to a roll, along
-- with their die count, ordered by count desc then player_id asc.
-- Used to find the top interferer for Spread Propaganda mar option (d).
SELECT player_id, count(*)::bigint AS dice_count
FROM dice_roll_dice
WHERE roll_id = $1 AND is_interference = true
GROUP BY player_id
ORDER BY dice_count DESC, player_id ASC;
