-- sqlc query file for simultaneous_reveals and simultaneous_reveal_entries.

-- ── Simultaneous Reveals ──────────────────────────────────────────────

-- name: CreateSimultaneousReveal :one
INSERT INTO simultaneous_reveals (game_id, plan_id, reveal_type)
VALUES ($1, $2, $3)
RETURNING id, game_id, plan_id, reveal_type, is_complete, result_delay, created_at;

-- name: GetSimultaneousReveal :one
SELECT id, game_id, plan_id, reveal_type, is_complete, result_delay, created_at
FROM simultaneous_reveals
WHERE id = $1;

-- name: SetRevealComplete :exec
UPDATE simultaneous_reveals
SET is_complete = TRUE, result_delay = $2
WHERE id = $1;

-- ── Reveal Entries ────────────────────────────────────────────────────

-- name: CreateRevealEntry :exec
INSERT INTO simultaneous_reveal_entries (reveal_id, player_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: SetRevealEntryFace :exec
UPDATE simultaneous_reveal_entries
SET face = $3, revealed_at = now()
WHERE reveal_id = $1 AND player_id = $2;

-- name: GetRevealEntry :one
SELECT reveal_id, player_id, face, revealed_at
FROM simultaneous_reveal_entries
WHERE reveal_id = $1 AND player_id = $2;

-- name: ListRevealEntries :many
SELECT reveal_id, player_id, face, revealed_at
FROM simultaneous_reveal_entries
WHERE reveal_id = $1
ORDER BY player_id;

-- name: CountRevealEntriesSubmitted :one
-- Count entries where the player has already submitted a face.
SELECT count(*) FROM simultaneous_reveal_entries
WHERE reveal_id = $1 AND revealed_at IS NOT NULL;

-- name: CountRevealEntries :one
SELECT count(*) FROM simultaneous_reveal_entries
WHERE reveal_id = $1;
