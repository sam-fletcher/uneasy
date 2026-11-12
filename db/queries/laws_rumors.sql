-- sqlc query file for laws and rumors.

-- ── Laws ─────────────────────────────────────────────────────────────

-- name: CreateLaw :one
-- writer_id is the player who wrote the law's text, or NULL for none. Their "new"
-- marker follows the law (migration 058): your own write isn't news to you, as
-- chat never counts your own posts. The marker is one number, so it moves only
-- when the writer had no OTHER unseen law behind this one; otherwise it stays
-- and their own law shows as new, the safe direction.
--
-- The catch-up is a data-modifying CTE of THIS statement, not a follow-up
-- statement. A separate "best effort" statement that fails inside a transaction
-- aborts the whole transaction (the Prologue creates laws inside one), whereas
-- a part of the insert itself can only fail when the insert would have. It
-- selects FROM players, so an id that isn't a player at this game inserts
-- nothing and cannot trip the foreign key.
--
-- Nothing records who wrote it: the only effect is on the writer's own private
-- marker row. Revs are allocated in call order, not commit order, so a marker
-- can in principle pass a lower rev that commits a moment later and never show
-- that entry as new. Chat's id marker has the same narrow window.
WITH new_rev AS (
  SELECT nextval('laws_rumors_rev_seq') AS rev
),
ack AS (
  INSERT INTO player_laws_rumors_seen (player_id, laws_seen_rev)
  SELECT p.id, new_rev.rev
  FROM players p, new_rev
  WHERE p.id = sqlc.narg(writer_id)::BIGINT
    AND p.game_id = sqlc.arg(game_id)::BIGINT
    AND NOT EXISTS (
      SELECT 1 FROM laws l
      WHERE l.game_id = p.game_id AND l.is_active
        AND l.rev > COALESCE(
          (SELECT s.laws_seen_rev FROM player_laws_rumors_seen s WHERE s.player_id = p.id), 0)
    )
  ON CONFLICT (player_id) DO UPDATE
    SET laws_seen_rev = GREATEST(player_laws_rumors_seen.laws_seen_rev, EXCLUDED.laws_seen_rev)
)
INSERT INTO laws (game_id, text, addendum, origin_plan_id, signatory_id, display_order, rev)
SELECT sqlc.arg(game_id)::BIGINT, sqlc.arg(text)::TEXT, sqlc.narg(addendum)::TEXT,
       sqlc.narg(origin_plan_id)::BIGINT, sqlc.narg(signatory_id)::BIGINT,
       sqlc.arg(display_order)::SMALLINT, new_rev.rev
FROM new_rev
RETURNING *;

-- name: ListLaws :many
SELECT * FROM laws WHERE game_id = $1 AND is_active = TRUE
ORDER BY display_order ASC, created_at ASC;

-- name: UpdateLawText :one
-- An edit draws a fresh rev, so the law counts as new to everyone whose marker
-- is behind it (migration 058). editor_id is the player who made the edit; like
-- the writer in CreateLaw, their marker follows it when they were otherwise
-- caught up (the law being edited is left out of that check). Nobody is
-- recorded: who edited a law is not stored on the row, and for a rumor it must
-- never be (see UpdateRumorText).
WITH new_rev AS (
  SELECT nextval('laws_rumors_rev_seq') AS rev
),
ack AS (
  INSERT INTO player_laws_rumors_seen (player_id, laws_seen_rev)
  SELECT p.id, new_rev.rev
  FROM laws target
  JOIN players p ON p.game_id = target.game_id
  CROSS JOIN new_rev
  WHERE target.id = sqlc.arg(id)::BIGINT
    AND p.id = sqlc.narg(editor_id)::BIGINT
    AND NOT EXISTS (
      SELECT 1 FROM laws l
      WHERE l.game_id = p.game_id AND l.is_active AND l.id <> target.id
        AND l.rev > COALESCE(
          (SELECT s.laws_seen_rev FROM player_laws_rumors_seen s WHERE s.player_id = p.id), 0)
    )
  ON CONFLICT (player_id) DO UPDATE
    SET laws_seen_rev = GREATEST(player_laws_rumors_seen.laws_seen_rev, EXCLUDED.laws_seen_rev)
)
UPDATE laws
SET text = sqlc.arg(text)::TEXT,
    addendum = sqlc.narg(addendum)::TEXT,
    rev = (SELECT rev FROM new_rev)
WHERE id = sqlc.arg(id)::BIGINT
RETURNING *;

-- name: GetLawByID :one
SELECT * FROM laws WHERE id = $1;

-- ── Rumors ───────────────────────────────────────────────────────────

-- name: CreateRumor :one
-- writer_id is the player who wrote the rumor's text, whether or not the rumor
-- hides its source (source_player_id is the public attribution and may be NULL
-- for a hidden one; writer_id is never stored). See CreateLaw for how the
-- writer's marker follows the rumor, and why that is part of this statement.
WITH new_rev AS (
  SELECT nextval('laws_rumors_rev_seq') AS rev
),
ack AS (
  INSERT INTO player_laws_rumors_seen (player_id, rumors_seen_rev)
  SELECT p.id, new_rev.rev
  FROM players p, new_rev
  WHERE p.id = sqlc.narg(writer_id)::BIGINT
    AND p.game_id = sqlc.arg(game_id)::BIGINT
    AND NOT EXISTS (
      SELECT 1 FROM rumors l
      WHERE l.game_id = p.game_id AND l.is_active
        AND l.rev > COALESCE(
          (SELECT s.rumors_seen_rev FROM player_laws_rumors_seen s WHERE s.player_id = p.id), 0)
    )
  ON CONFLICT (player_id) DO UPDATE
    SET rumors_seen_rev = GREATEST(player_laws_rumors_seen.rumors_seen_rev, EXCLUDED.rumors_seen_rev)
)
INSERT INTO rumors (game_id, text, target_asset_id, origin_plan_id, source_player_id, display_order, rev)
SELECT sqlc.arg(game_id)::BIGINT, sqlc.arg(text)::TEXT, sqlc.narg(target_asset_id)::BIGINT,
       sqlc.narg(origin_plan_id)::BIGINT, sqlc.narg(source_player_id)::BIGINT,
       sqlc.arg(display_order)::SMALLINT, new_rev.rev
FROM new_rev
RETURNING *;

-- name: ListRumors :many
SELECT * FROM rumors WHERE game_id = $1 AND is_active = TRUE
ORDER BY display_order ASC, created_at ASC;

-- name: SetRumorSourceHidden :exec
-- Remove the source attribution from a rumor (Spread Rumors hide-source option).
UPDATE rumors SET source_player_id = NULL WHERE id = $1;

-- name: GetRumorByID :one
SELECT * FROM rumors WHERE id = $1;

-- name: UpdateRumorText :one
-- Draws a fresh rev (an edit counts as new, migration 058). Deliberately stores
-- no editor: rumors rows are served whole to every player, and a hidden-source
-- rumor is edited by its hidden author, so a column naming the editor would name
-- them. editor_id only moves the editor's own private marker (see UpdateLawText).
WITH new_rev AS (
  SELECT nextval('laws_rumors_rev_seq') AS rev
),
ack AS (
  INSERT INTO player_laws_rumors_seen (player_id, rumors_seen_rev)
  SELECT p.id, new_rev.rev
  FROM rumors target
  JOIN players p ON p.game_id = target.game_id
  CROSS JOIN new_rev
  WHERE target.id = sqlc.arg(id)::BIGINT
    AND p.id = sqlc.narg(editor_id)::BIGINT
    AND NOT EXISTS (
      SELECT 1 FROM rumors l
      WHERE l.game_id = p.game_id AND l.is_active AND l.id <> target.id
        AND l.rev > COALESCE(
          (SELECT s.rumors_seen_rev FROM player_laws_rumors_seen s WHERE s.player_id = p.id), 0)
    )
  ON CONFLICT (player_id) DO UPDATE
    SET rumors_seen_rev = GREATEST(player_laws_rumors_seen.rumors_seen_rev, EXCLUDED.rumors_seen_rev)
)
UPDATE rumors
SET text = sqlc.arg(text)::TEXT,
    rev = (SELECT rev FROM new_rev)
WHERE id = sqlc.arg(id)::BIGINT
RETURNING *;

-- ── "New" markers (migration 058) ────────────────────────────────────

-- name: GetLawsRumorsSeen :one
-- The viewer's two markers. A player with no row reads 0 for both (everything
-- is new to them), which is why this joins from players rather than selecting
-- from the markers table: it always returns a row for a real player.
SELECT COALESCE(s.laws_seen_rev, 0)::BIGINT   AS laws_seen_rev,
       COALESCE(s.rumors_seen_rev, 0)::BIGINT AS rumors_seen_rev
FROM players p
LEFT JOIN player_laws_rumors_seen s ON s.player_id = p.id
WHERE p.id = $1;

-- name: MarkLawsSeen :one
-- Moves the viewer's laws marker forward to requested_rev and returns the
-- stored value. Monotonic (GREATEST: a late or replayed request can't move it
-- back) and clamped to the game's newest law rev (LEAST: a client can't mark
-- laws that don't exist yet seen). The mirror of UpdateReadMarker for chat.
INSERT INTO player_laws_rumors_seen (player_id, laws_seen_rev)
VALUES (
  sqlc.arg(player_id),
  LEAST(
    sqlc.arg(requested_rev)::BIGINT,
    COALESCE((SELECT max(rev) FROM laws WHERE game_id = sqlc.arg(game_id) AND is_active), 0)
  )
)
ON CONFLICT (player_id) DO UPDATE
  SET laws_seen_rev = GREATEST(player_laws_rumors_seen.laws_seen_rev, EXCLUDED.laws_seen_rev)
RETURNING laws_seen_rev;

-- name: MarkRumorsSeen :one
-- MarkLawsSeen for rumors.
INSERT INTO player_laws_rumors_seen (player_id, rumors_seen_rev)
VALUES (
  sqlc.arg(player_id),
  LEAST(
    sqlc.arg(requested_rev)::BIGINT,
    COALESCE((SELECT max(rev) FROM rumors WHERE game_id = sqlc.arg(game_id) AND is_active), 0)
  )
)
ON CONFLICT (player_id) DO UPDATE
  SET rumors_seen_rev = GREATEST(player_laws_rumors_seen.rumors_seen_rev, EXCLUDED.rumors_seen_rev)
RETURNING rumors_seen_rev;
