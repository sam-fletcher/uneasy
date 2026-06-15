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

-- name: ListAllAssetsByGame :many
-- Like ListAssetsByGame but INCLUDES destroyed assets. Used ONLY by the
-- retinue display handler so destroyed assets can render as read-only
-- "tombstone" cards. Never use this in gameplay logic — every mechanics
-- path (counts, plan targeting, roll staging) must stay on the filtered
-- ListAssetsByGame / ListAssetsByOwner so destroyed assets never leak in.
SELECT * FROM assets WHERE game_id = $1
ORDER BY created_at ASC;

-- name: ListAssetsByOwner :many
SELECT * FROM assets WHERE owner_id = $1 AND is_destroyed = FALSE
ORDER BY created_at ASC;

-- name: UpdateAssetName :exec
UPDATE assets SET name = $2 WHERE id = $1;

-- name: SetMainCharacter :exec
UPDATE assets SET is_main_character = $2 WHERE id = $1;

-- name: GetMainCharacterByOwner :one
-- Returns the player's main-character asset in this game, if any.
SELECT * FROM assets
WHERE game_id = $1 AND owner_id = $2 AND is_main_character = TRUE AND is_destroyed = FALSE
LIMIT 1;

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

-- name: DestroyIfAllMarginaliaTorn :execrows
-- Marks an asset destroyed iff none of its marginalia remain intact. The
-- caller (the TearMarginalia handler) invokes this after a tear; a return
-- of 1 means the tear completed the destruction, 0 means the asset still
-- has at least one intact marginalia and survives. Composing the rule in
-- a single statement keeps the "last tear destroys" invariant testable at
-- the queries layer instead of buried in handler logic.
UPDATE assets a
SET is_destroyed = TRUE, destroyed_at = now()
WHERE a.id = $1
  AND a.is_destroyed = FALSE
  AND EXISTS (SELECT 1 FROM marginalia m WHERE m.asset_id = a.id)
  AND NOT EXISTS (SELECT 1 FROM marginalia m WHERE m.asset_id = a.id AND m.is_torn = FALSE);

-- name: CountLeveragedAssets :one
SELECT count(*) FROM assets
WHERE owner_id = $1 AND is_leveraged = FALSE AND is_destroyed = FALSE;

-- name: CountPeerAssets :one
-- Returns the number of non-destroyed peer assets owned by a player in a game.
-- Used to determine whether a player is eligible to be focus player or prepare plans.
SELECT count(*) FROM assets
WHERE game_id = $1 AND owner_id = $2 AND asset_type = 'peer' AND is_destroyed = FALSE;

-- ── Marginalia ───────────────────────────────────────────────────────

-- name: GetMarginaliaByID :one
SELECT * FROM marginalia WHERE id = $1;

-- name: CreateMarginalia :one
INSERT INTO marginalia (asset_id, position, text)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListMarginaliaByAsset :many
SELECT * FROM marginalia WHERE asset_id = $1 ORDER BY position;

-- name: ListMarginaliaTextByGame :many
-- All marginalia text in a game (across every asset, torn or not), for
-- deduping suggestion pools so players aren't offered an example already in
-- play. Joins through assets to scope by game.
SELECT m.text
FROM marginalia m
JOIN assets a ON a.id = m.asset_id
WHERE a.game_id = $1;

-- name: UpdateMarginaliaText :exec
UPDATE marginalia SET text = $2 WHERE id = $1;

-- name: TearMarginalia :execrows
-- Tearing is one-shot — a torn marginalia cannot be torn again. The
-- WHERE-clause guard makes the update idempotent at the SQL layer; callers
-- treat a 0-row return as "already torn / no such marginalia". The
-- handler also checks before calling for a friendlier error, but this
-- guards direct query callers (tests, dev tooling) and races between two
-- concurrent tearers.
UPDATE marginalia
SET is_torn = TRUE, torn_at = now(), torn_by_id = $2
WHERE id = $1 AND is_torn = FALSE;

-- name: ListIntactMarginalia :many
SELECT * FROM marginalia WHERE asset_id = $1 AND is_torn = FALSE ORDER BY position;

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

-- name: GetSecretByID :one
SELECT * FROM secrets WHERE id = $1;

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

-- name: ListVisibleSecretsByGame :many
-- All secrets in this game that the given player can see.
SELECT s.* FROM secrets s
JOIN assets a ON s.asset_id = a.id
LEFT JOIN secret_visibility sv ON s.id = sv.secret_id AND sv.player_id = $2
WHERE a.game_id = $1 AND (s.author_id = $2 OR sv.player_id IS NOT NULL)
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

-- name: CountSecretsByAsset :one
-- Total secrets on an asset (existence). Public to all players — only the
-- content stays gated by secret_visibility. See SECRETS_RULES.md.
SELECT COUNT(*) FROM secrets WHERE asset_id = $1;

-- name: CountSecretsByGame :many
-- Total secret count per asset across a game, for the enriched asset list.
-- Assets with no secrets are simply absent (callers default them to 0).
SELECT s.asset_id, COUNT(*) AS secret_count
FROM secrets s
JOIN assets a ON s.asset_id = a.id
WHERE a.game_id = $1
GROUP BY s.asset_id;

-- name: RefreshAllAssets :exec
-- Un-leverage every asset in a game. Used at Shake-Up entry per rules.
UPDATE assets SET is_leveraged = FALSE WHERE game_id = $1;
