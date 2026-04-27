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

-- ── heart declarations ───────────────────────────────────────────────────────

-- name: UpsertHeartDeclaration :exec
INSERT INTO prologue_heart_declarations (game_id, player_id, track, count)
VALUES ($1, $2, $3, $4)
ON CONFLICT (game_id, player_id, track)
DO UPDATE SET count = EXCLUDED.count;

-- name: ListHeartDeclarationsByGame :many
SELECT * FROM prologue_heart_declarations WHERE game_id = $1;

-- name: SumHeartDeclarationsByPlayer :one
SELECT COALESCE(SUM(count), 0)::SMALLINT AS total
FROM prologue_heart_declarations
WHERE game_id = $1 AND player_id = $2;

-- ── games.prologue_ranking_step ──────────────────────────────────────────────

-- name: SetPrologueRankingStep :exec
UPDATE games SET prologue_ranking_step = $2 WHERE id = $1;
