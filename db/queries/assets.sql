-- sqlc query file for assets, marginalia, and secrets.

-- ── Assets ───────────────────────────────────────────────────────────

-- name: CreateAsset :one
INSERT INTO assets (game_id, owner_id, creator_id, asset_type, name, is_main_character)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAssetByID :one
SELECT * FROM assets WHERE id = $1;

-- name: ListAssetsByGame :many
SELECT * FROM assets WHERE game_id = $1 AND is_destroyed = FALSE
ORDER BY created_at ASC;

-- name: ListAssetsByOwner :many
SELECT * FROM assets WHERE owner_id = $1 AND is_destroyed = FALSE
ORDER BY created_at ASC;

-- name: UpdateAssetName :exec
UPDATE assets SET name = $2 WHERE id = $1;

-- name: SetMainCharacter :exec
UPDATE assets SET is_main_character = $2 WHERE id = $1;

-- name: ClearMainCharacter :exec
-- Unset main character for all of a player's assets in a game.
UPDATE assets SET is_main_character = FALSE
WHERE owner_id = $1 AND game_id = $2 AND is_main_character = TRUE;

-- name: SetAssetLeveraged :exec
UPDATE assets SET is_leveraged = $2 WHERE id = $1;

-- name: RefreshPlayerAssets :exec
-- Un-leverage up to N assets for a player (used by the refresh action).
-- The caller picks which assets to refresh; this updates them individually.
UPDATE assets SET is_leveraged = FALSE WHERE id = $1;

-- name: TransferAsset :exec
UPDATE assets SET owner_id = $2 WHERE id = $1;

-- name: DestroyAsset :exec
UPDATE assets SET is_destroyed = TRUE, destroyed_at = now() WHERE id = $1;

-- name: CountLeveragedAssets :one
SELECT count(*) FROM assets
WHERE owner_id = $1 AND is_leveraged = FALSE AND is_destroyed = FALSE;

-- ── Marginalia ───────────────────────────────────────────────────────

-- name: CreateMarginalia :one
INSERT INTO marginalia (asset_id, position, text)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListMarginaliaByAsset :many
SELECT * FROM marginalia WHERE asset_id = $1 ORDER BY position;

-- name: UpdateMarginaliaText :exec
UPDATE marginalia SET text = $2 WHERE id = $1;

-- name: TearMarginalia :exec
UPDATE marginalia SET is_torn = TRUE, torn_at = now(), torn_by_id = $2 WHERE id = $1;

-- name: CountIntactMarginalia :one
SELECT count(*) FROM marginalia WHERE asset_id = $1 AND is_torn = FALSE;

-- name: CountTornMarginalia :one
SELECT count(*) FROM marginalia WHERE asset_id = $1 AND is_torn = TRUE;

-- name: CountMarginalia :one
SELECT count(*) FROM marginalia WHERE asset_id = $1;

-- ── Secrets ──────────────────────────────────────────────────────────

-- name: CreateSecret :one
INSERT INTO secrets (asset_id, author_id, text)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListSecretsByAsset :many
SELECT s.* FROM secrets s
WHERE s.asset_id = $1
ORDER BY s.created_at ASC;

-- name: ListVisibleSecrets :many
-- Secrets the player can see: authored by them, or with a visibility row.
SELECT s.* FROM secrets s
LEFT JOIN secret_visibility sv ON s.id = sv.secret_id AND sv.player_id = $2
WHERE s.asset_id = $1 AND (s.author_id = $2 OR sv.player_id IS NOT NULL)
ORDER BY s.created_at ASC;

-- name: AddSecretVisibility :exec
INSERT INTO secret_visibility (secret_id, player_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RevealSecret :exec
UPDATE secrets SET is_revealed = TRUE, revealed_at = now() WHERE id = $1;

-- name: GrantSecretVisibilityForAsset :exec
-- Give a player visibility on ALL secrets of an asset (used when taking/breaking).
INSERT INTO secret_visibility (secret_id, player_id)
SELECT id, $2 FROM secrets WHERE asset_id = $1
ON CONFLICT DO NOTHING;
