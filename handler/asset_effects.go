package handler

// asset_effects.go — shared mechanical asset effects reused across plan
// handlers (break, transfer). These wrap the canonical DB ops + WebSocket
// broadcasts + chat-log emission so each plan's make/mar code doesn't
// re-implement (and drift from) the same sequence.

import (
	"context"
	"fmt"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// breakVerb returns the past-tense verb for a break action's chat log: "broke"
// for a normal tear, or "destroyed" when that tear removed the asset's last
// marginalia. Centralises the phrasing shared by every plan that breaks.
func breakVerb(destroyed bool) string {
	if destroyed {
		return "destroyed"
	}
	return "broke"
}

// brokenAssetPrompt returns the trailing clause inviting an asset's owner to
// narrate the effect of a break: " <Owner>, how has the asset changed?". It
// returns "" when the tear destroyed the asset — nothing remains to re-describe,
// and a separate asset.destroyed post already records the loss.
func brokenAssetPrompt(ctx context.Context, q *dbgen.Queries, ownerID int64, destroyed bool) string {
	if destroyed {
		return ""
	}
	return fmt.Sprintf(" %s, how has the asset changed?", playerDisplayName(ctx, q, ownerID))
}

// brokenAssetDetail is brokenAssetPrompt prefixed with the text of the marginalia
// just torn, for plan break logs whose flavour line doesn't already quote it. The
// marginalia text stays quoted (only asset names are bolded — see assetMark).
func brokenAssetDetail(
	ctx context.Context,
	q *dbgen.Queries,
	ownerID int64,
	m *dbgen.Marginalium,
	destroyed bool,
) string {
	return fmt.Sprintf(" The torn marginalia read %q.", m.Text) + brokenAssetPrompt(ctx, q, ownerID, destroyed)
}

// breakMarginalia performs the canonical "break an asset" effect: tear one
// marginalia, reveal the asset's secrets to the tearing player, broadcast the
// tear, and — if that was the asset's last intact marginalia — destroy the
// asset and emit the asset.destroyed events. Returns whether the asset was
// destroyed.
//
// This is the single source of truth for breaking; see the rules glossary
// ("Break = tear off one marginalia; all 4 gone → destroyed"). The standalone
// asset tear endpoint (assets.go) predates this helper and inlines the same
// sequence.
func breakMarginalia(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	asset *dbgen.Asset,
	m *dbgen.Marginalium,
	tornBy int64,
) (destroyed bool, err error) {
	if _, err = q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
		ID:       m.ID,
		TornByID: &tornBy,
	}); err != nil {
		return false, err
	}

	// Tearing reveals the asset's current secrets to the tearing player
	// (idempotent — a no-op if already visible).
	_ = q.GrantSecretVisibilityForAsset(ctx, dbgen.GrantSecretVisibilityForAssetParams{
		AssetID:  asset.ID,
		PlayerID: tornBy,
	})

	broadcastEvent(manager, asset.GameID, model.EventMarginaliaTorn, model.MarginaliaTornPayload{
		AssetID:  asset.ID,
		Position: m.Position,
		TornByID: tornBy,
	})
	broadcastEvent(manager, asset.GameID, model.EventSecretVisibilityGrant, model.SecretVisibilityGrantPayload{
		AssetID:  asset.ID,
		PlayerID: tornBy,
	})

	// If that was the last intact marginalia, the asset is destroyed.
	destroyedRows, _ := q.DestroyIfAllMarginaliaTorn(ctx, asset.ID)
	if destroyedRows > 0 {
		broadcastEvent(manager, asset.GameID, model.EventAssetDestroyed, model.AssetIDPayload{AssetID: asset.ID})
		if game, gerr := q.GetGameByID(ctx, asset.GameID); gerr == nil {
			EmitAssetDestroyed(ctx, q, manager, asset.GameID, *asset, game.CurrentRow)
		}
		return true, nil
	}
	return false, nil
}

// grantSecretsOnTake gives newOwnerID visibility on every secret of an asset and
// broadcasts the secret-visibility grant, per the rules: "if you take or break
// an asset, you can look on its underside to learn any secrets it might be
// holding" (SECRETS_RULES.md). The DB grant is idempotent (a no-op if already
// visible). Use this whenever an asset changes hands; takeAssetEffect wraps it
// for the common transfer+broadcast case, while sites that emit a non-standard
// transfer event (e.g. Make War's war.seized) call this directly after their own
// transfer. Skipping it is the bug class that left new owners unable to read the
// secrets of assets they had just taken.
func grantSecretsOnTake(
	ctx context.Context, q *dbgen.Queries, manager *hub.Manager, gameID, assetID, newOwnerID int64,
) {
	_ = q.GrantSecretVisibilityForAsset(ctx, dbgen.GrantSecretVisibilityForAssetParams{
		AssetID:  assetID,
		PlayerID: newOwnerID,
	})
	broadcastEvent(manager, gameID, model.EventSecretVisibilityGrant, model.SecretVisibilityGrantPayload{
		AssetID:  assetID,
		PlayerID: newOwnerID,
	})
}

// takeAssetEffect performs the canonical "a player takes ownership of an asset"
// effect: transfer ownership, grant the new owner visibility on the asset's
// secrets, and broadcast both asset.taken (with the refreshed asset) and the
// secret-visibility grant. Returns the refreshed asset so callers can use its
// post-transfer fields. This is the single source of truth for an asset changing
// hands through the standard asset.taken event; callers add their own
// context-specific chat-log line. Sites with a bespoke transfer event use
// grantSecretsOnTake instead.
func takeAssetEffect(
	ctx context.Context, q *dbgen.Queries, manager *hub.Manager,
	gameID, assetID, oldOwnerID, newOwnerID int64,
) (dbgen.Asset, error) {
	if err := q.TransferAsset(ctx, dbgen.TransferAssetParams{
		ID:      assetID,
		OwnerID: newOwnerID,
	}); err != nil {
		return dbgen.Asset{}, err
	}
	updated, _ := q.GetAssetByID(ctx, assetID)
	broadcastEvent(manager, gameID, model.EventAssetTaken, model.AssetTakenPayload{
		Asset:      updated,
		OldOwnerID: oldOwnerID,
		NewOwnerID: newOwnerID,
	})
	grantSecretsOnTake(ctx, q, manager, gameID, assetID, newOwnerID)
	return updated, nil
}
