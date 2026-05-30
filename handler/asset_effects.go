package handler

// asset_effects.go — shared mechanical asset effects reused across plan
// handlers (break, transfer). These wrap the canonical DB ops + WebSocket
// broadcasts + chat-log emission so each plan's make/mar code doesn't
// re-implement (and drift from) the same sequence.

import (
	"context"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// breakMarginalia performs the canonical "break an asset" effect: tear one
// marginalia, reveal the asset's secrets to the tearing player, broadcast the
// tear, and — if that was the asset's last intact marginalium — destroy the
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

	// If that was the last intact marginalium, the asset is destroyed.
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
