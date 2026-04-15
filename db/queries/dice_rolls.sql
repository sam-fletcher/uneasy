-- sqlc query file for dice rolls.

-- ── Dice Rolls ───────────────────────────────────────────────────────

-- name: CreateDiceRoll :one
INSERT INTO dice_rolls (game_id, plan_id, row_number, actor_id, difficulty)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetDiceRollByID :one
SELECT * FROM dice_rolls WHERE id = $1;

-- name: SetDiceRollAdjustedDifficulty :exec
UPDATE dice_rolls SET adjusted_difficulty = $2 WHERE id = $1;

-- name: ResolveDiceRoll :exec
UPDATE dice_rolls SET result = $2, outcome = $3, resolved_at = now() WHERE id = $1;

-- name: ListDiceRollsByGame :many
SELECT * FROM dice_rolls WHERE game_id = $1 ORDER BY created_at ASC;

-- name: GetOpenRollByGame :one
SELECT * FROM dice_rolls
WHERE game_id = $1 AND resolved_at IS NULL
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

-- ── Difficulty Votes ─────────────────────────────────────────────────

-- name: CreateDifficultyVote :exec
INSERT INTO difficulty_votes (roll_id, player_id, vote)
VALUES ($1, $2, $3)
ON CONFLICT (roll_id, player_id) DO UPDATE SET vote = EXCLUDED.vote;

-- name: ListVotesByRoll :many
SELECT * FROM difficulty_votes WHERE roll_id = $1;

-- name: CountVotesByRoll :one
SELECT
  count(*) FILTER (WHERE vote = 'yea') AS yea_count,
  count(*) FILTER (WHERE vote = 'nay') AS nay_count
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
