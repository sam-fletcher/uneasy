-- sqlc query file for the unified chat feed.
--
-- scene_posts is one game-wide stream. Each row is either:
--   - a player message (author_id NOT NULL, system_code NULL, severity 0)
--   - a system post    (author_id NULL,    system_code NOT NULL, severity > 0)
--
-- Severity is an integer scale (see model/severity.go for named constants).
-- row_number / plan_id / scene_id are optional anchors the client uses for
-- the "jump to" UI in the Public Record sidebar.
--
-- scene_entries (the public-record one-line summaries) are unchanged here.

-- name: CreatePlayerMessage :one
INSERT INTO scene_posts (
  game_id, author_id, body, row_number, plan_id, scene_id, severity, speaking_as_asset_id
) VALUES ($1, $2, $3, $4, $5, $6, 0, $7)
RETURNING *;

-- name: CreateSystemPost :one
INSERT INTO scene_posts (
  game_id, body, row_number, plan_id, scene_id, severity, system_code, system_data
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListGamePosts :many
SELECT * FROM scene_posts
WHERE game_id = $1
ORDER BY id ASC;

-- name: GetScenePostByID :one
SELECT * FROM scene_posts WHERE id = $1;

-- name: ListGamePostsAfterLimited :many
-- Oldest `limit` posts after `id`, ascending. Used for the initial-window
-- unread span and the "after" half of an `around` window.
SELECT * FROM scene_posts
WHERE game_id = $1 AND id > $2
ORDER BY id ASC
LIMIT $3;

-- name: ListGamePostsBefore :many
-- Newest `limit` posts before `id`, returned newest-first — callers reverse
-- to ascending order themselves before merging into a window.
SELECT * FROM scene_posts
WHERE game_id = $1 AND id < $2
ORDER BY id DESC
LIMIT $3;

-- name: ListGamePostsNewest :many
-- Newest `limit` posts in the game, returned newest-first — callers reverse
-- to ascending order.
SELECT * FROM scene_posts
WHERE game_id = $1
ORDER BY id DESC
LIMIT $2;

-- name: GamePostExistsBefore :one
SELECT EXISTS (
  SELECT 1 FROM scene_posts WHERE game_id = $1 AND id < $2
) AS exists;

-- name: GamePostExistsAfter :one
SELECT EXISTS (
  SELECT 1 FROM scene_posts WHERE game_id = $1 AND id > $2
) AS exists;

-- name: CountUnreadPostsByAccount :many
-- Unread posts for every table an account sits at, in one round trip, for the
-- profile page's per-table badge (adr/CHAT_OVERHAUL_PLAN.md Phase 6). A
-- single-table form used to exist and the table list called it once per table,
-- which is a round trip per card.
--
-- This is the server-side mirror of isUnreadPost() in
-- frontend/src/lib/chatFeed.ts and MUST stay in step with it — newer than that
-- player's read marker, not authored by them, and either a player message
-- (author_id IS NOT NULL) or a system post at or above the "hide bookkeeping"
-- bar. Those are the only two statements of the rule, here and in the
-- frontend; any change to one belongs in both. min_severity is passed in
-- rather than hardcoded so model.SeverityDefault stays the single source of
-- that threshold. `author_id IS DISTINCT FROM` (not `<>`) so system posts,
-- whose author_id is NULL, survive the comparison instead of being silently
-- dropped by it.
--
-- The (game, viewer, marker) triples are read straight off the players table
-- rather than passed in, because that is where the caller got them: its list is
-- ListPlayersByAccount, one row per table. The join is LEFT so a table with
-- nothing unread still returns a row with count 0 — the caller must see every
-- table it asked about, not only the ones with news. Keyed by player id, which
-- is unique per row of that list.
SELECT p.id AS viewer_id, COUNT(sp.id) AS unread_count
FROM players p
LEFT JOIN scene_posts sp
  ON sp.game_id = p.game_id
 AND sp.id > p.last_read_post_id
 AND sp.author_id IS DISTINCT FROM p.id
 AND (sp.author_id IS NOT NULL OR sp.severity >= sqlc.arg(min_severity)::INTEGER)
WHERE p.account_id = sqlc.arg(account_id)
GROUP BY p.id;

-- name: FindAnchorPostByRow :one
SELECT id FROM scene_posts
WHERE game_id = $1 AND system_code = $2 AND row_number = $3
ORDER BY id ASC LIMIT 1;

-- name: FindAnchorPostByPlan :one
SELECT id FROM scene_posts
WHERE game_id = $1 AND system_code = $2 AND plan_id = $3
ORDER BY id ASC LIMIT 1;

-- name: FindAnchorPostByScene :one
SELECT id FROM scene_posts
WHERE game_id = $1 AND system_code = $2 AND scene_id = $3
ORDER BY id ASC LIMIT 1;

-- name: CreateSceneEntry :one
INSERT INTO scene_entries (game_id, row_number, author_id, body)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListSceneEntries :many
SELECT * FROM scene_entries
WHERE game_id = $1
ORDER BY row_number ASC, created_at ASC;

-- name: ListSceneEntriesByRow :many
SELECT * FROM scene_entries
WHERE game_id = $1 AND row_number = $2
ORDER BY created_at ASC;
