-- sqlc query file for the structured prologue (Phase 4b).

-- name: CreatePrologueChoice :one
INSERT INTO prologue_choices (game_id, player_id, turn_number, sheet_type, choice_name)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListPrologueChoicesByGame :many
SELECT * FROM prologue_choices WHERE game_id = $1 ORDER BY player_id, turn_number;

-- name: CountPrologueChoicesByPlayer :one
SELECT count(*) FROM prologue_choices WHERE game_id = $1 AND player_id = $2;

-- name: PrologueChoiceClaimed :one
SELECT EXISTS (
  SELECT 1 FROM prologue_choices
  WHERE game_id = $1 AND sheet_type = $2 AND choice_name = $3
);

-- name: ListPrologueChoiceClaimsByGame :many
SELECT sheet_type, choice_name, player_id, turn_number
FROM prologue_choices
WHERE game_id = $1;

-- ── player_cards ─────────────────────────────────────────────────────────────

-- name: GetCardOwner :one
SELECT player_id FROM player_cards
WHERE game_id = $1 AND card_suit = $2 AND card_value = $3;

-- name: InsertPlayerCard :exec
INSERT INTO player_cards (game_id, player_id, card_suit, card_value)
VALUES ($1, $2, $3, $4);

-- name: TransferPlayerCard :exec
UPDATE player_cards SET player_id = $1
WHERE game_id = $2 AND card_suit = $3 AND card_value = $4;

-- name: ListPlayerCardsByGame :many
SELECT * FROM player_cards WHERE game_id = $1 ORDER BY player_id, card_suit, card_value;

-- name: ListPlayerCardsByPlayer :many
SELECT * FROM player_cards WHERE game_id = $1 AND player_id = $2;

-- name: GetPlayerCardByID :one
SELECT * FROM player_cards WHERE id = $1;

-- ── linked-card lookups on assets ────────────────────────────────────────────

-- name: GetAssetByLinkedCard :one
SELECT * FROM assets
WHERE game_id = $1 AND linked_card_suit = $2 AND linked_card_value = $3
  AND is_destroyed = FALSE;

-- name: ClearAssetLinkedCards :exec
UPDATE assets SET linked_card_suit = NULL, linked_card_value = NULL
WHERE game_id = $1;

-- name: CreateAssetWithLinkedCard :one
INSERT INTO assets (game_id, owner_id, creator_id, asset_type, name, linked_card_suit, linked_card_value)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- ── games.prologue_ranking_step ──────────────────────────────────────────────

-- name: SetPrologueRankingStep :exec
UPDATE games SET prologue_ranking_step = $2 WHERE id = $1;

-- ── committed hearts (max-commitment model) ──────────────────────────────────

-- name: CommitHeart :exec
INSERT INTO prologue_committed_hearts (game_id, player_id, track, card_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (game_id, card_id)
DO UPDATE SET track = EXCLUDED.track, player_id = EXCLUDED.player_id;

-- name: UncommitHeart :exec
DELETE FROM prologue_committed_hearts
WHERE game_id = $1 AND card_id = $2;

-- name: ClearTrackCommittedHearts :exec
DELETE FROM prologue_committed_hearts
WHERE game_id = $1 AND track = $2;

-- name: DeleteCommittedHeartsByCardIDs :exec
DELETE FROM prologue_committed_hearts
WHERE game_id = $1 AND card_id = ANY(sqlc.arg(card_ids)::BIGINT[]);

-- name: ListCommittedHeartsByGame :many
SELECT ch.id, ch.game_id, ch.player_id, ch.track, ch.card_id,
       pc.card_value, pc.card_suit
FROM prologue_committed_hearts ch
JOIN player_cards pc ON pc.id = ch.card_id
WHERE ch.game_id = $1
ORDER BY ch.player_id, ch.track, ch.card_id;

-- ── track-done signal ────────────────────────────────────────────────────────

-- name: SetTrackDone :exec
INSERT INTO prologue_track_done (game_id, player_id, track, done, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (game_id, player_id, track)
DO UPDATE SET done = EXCLUDED.done, updated_at = now();

-- name: ListTrackDoneByGame :many
SELECT * FROM prologue_track_done WHERE game_id = $1;

-- name: ResetTrackDone :exec
DELETE FROM prologue_track_done
WHERE game_id = $1 AND track = $2;

-- ── extra peers ──────────────────────────────────────────────────────────────

-- name: InsertExtraPeer :one
INSERT INTO prologue_extra_peers (game_id, player_id, title_name, asset_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListExtraPeersByGame :many
SELECT * FROM prologue_extra_peers WHERE game_id = $1 ORDER BY created_at;

-- name: ExtraPeerExistsForPlayer :one
SELECT EXISTS (
  SELECT 1 FROM prologue_extra_peers
  WHERE game_id = $1 AND player_id = $2
);

-- name: ExtraPeerTitleClaimed :one
SELECT EXISTS (
  SELECT 1 FROM prologue_extra_peers
  WHERE game_id = $1 AND title_name = $2
);

-- ── closing-stage ready flags ────────────────────────────────────────────────

-- name: SetClosingReady :exec
INSERT INTO prologue_closing_ready (game_id, player_id, ready, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (game_id, player_id)
DO UPDATE SET ready = EXCLUDED.ready, updated_at = now();

-- name: ListClosingReadyByGame :many
SELECT * FROM prologue_closing_ready WHERE game_id = $1;
