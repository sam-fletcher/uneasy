-- sqlc query file for the endgame-mode table vote
-- (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md).

-- name: CastEndingVote :exec
-- Upsert: a player may change their vote freely for as long as
-- games.ending_vote_open is true. created_at is refreshed on a change so the
-- row records when the *current* choice was made.
INSERT INTO ending_votes (game_id, player_id, mode, created_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (game_id, player_id)
DO UPDATE SET mode = EXCLUDED.mode, created_at = now();

-- name: ListEndingVotesByGame :many
-- Every vote cast so far, oldest first. Votes are public — who voted and for
-- what — so this drives both the running tally and the "who hasn't voted yet"
-- half of the Waiting On bar.
SELECT * FROM ending_votes WHERE game_id = $1 ORDER BY created_at ASC, player_id ASC;
