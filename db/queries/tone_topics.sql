-- sqlc query file for tone-setting topics.

-- name: CreateToneTopic :one
INSERT INTO tone_topics (game_id, topic, status)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListToneTopics :many
SELECT * FROM tone_topics WHERE game_id = $1 ORDER BY id;

-- name: UpdateToneTopicStatusScoped :one
-- Ownership check, phase gate, and write in one round trip — the tone tiles
-- are tapped in bursts, so this endpoint's latency is felt directly (see
-- handler/tone.go). The outer SELECT returns a row whenever the topic
-- exists, so the caller can still tell "no such topic" (no rows) from
-- "wrong game" (game_id) from "tones are locked" (did_update = false)
-- rather than collapsing all three into an empty result.
--
-- allowed_phases is the set of phases in which tone edits are allowed. It is
-- passed in rather than hardcoded here so handler/tone.go's tonesLocked stays
-- the one definition of the rule.
--
-- The UPDATE runs exactly once regardless of whether the outer query reads it
-- (Postgres executes data-modifying CTEs to completion), and matches no rows
-- when its guards fail. It re-derives those guards from the parameters rather
-- than selecting from `target` — sqlc's analyser does not resolve a CTE
-- referenced from inside a data-modifying CTE's subquery — which costs
-- nothing at run time, both being primary-key lookups against pages the
-- planner has already touched for `target`.
WITH target AS (
    SELECT t.game_id, t.topic
    FROM tone_topics t
    WHERE t.id = $1
), updated AS (
    UPDATE tone_topics SET status = $3
    WHERE tone_topics.id = $1
      AND tone_topics.game_id = $2
      AND EXISTS (
          SELECT 1 FROM games
          WHERE games.id = tone_topics.game_id
            AND games.phase::text = ANY(@allowed_phases::text[])
      )
    RETURNING tone_topics.id
)
SELECT
    target.game_id,
    target.topic,
    EXISTS (SELECT 1 FROM updated) AS did_update
FROM target;

-- name: SeedToneTopic :exec
-- Used for bulk-seeding default topics; silently skips duplicates.
INSERT INTO tone_topics (game_id, topic, status)
VALUES ($1, $2, $3)
ON CONFLICT (game_id, topic) DO NOTHING;
