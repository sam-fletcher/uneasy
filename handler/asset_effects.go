package handler

// asset_effects.go — shared mechanical asset effects reused across plan
// handlers (break, transfer). These wrap the canonical DB ops + WebSocket
// broadcasts + chat-log emission so each plan's make/mar code doesn't
// re-implement (and drift from) the same sequence.

import (
	"context"
	"fmt"
	"net/http"

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
//
// m == nil is the blank-asset break (see breakAsset): there is no torn text to
// quote, and the break always destroys, so this contributes nothing.
func brokenAssetDetail(
	ctx context.Context,
	q *dbgen.Queries,
	ownerID int64,
	m *dbgen.Marginalium,
	destroyed bool,
) string {
	if m == nil {
		return brokenAssetPrompt(ctx, q, ownerID, destroyed)
	}
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
//
// It is a thin wrapper over breakAsset for the ~10 callers that always hold a
// concrete marginalium; reach for breakAsset directly when the target may be
// blank.
func breakMarginalia(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	asset *dbgen.Asset,
	m *dbgen.Marginalium,
	tornBy int64,
) (destroyed bool, err error) {
	return breakAsset(ctx, q, manager, asset, m, tornBy)
}

// breakAsset is breakMarginalia widened to cover the blank asset — one carrying
// no marginalia rows at all (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md, D3).
//
// m != nil is the ordinary break: tear that marginalium, then destroy the asset
// if it was the last intact one.
//
// m == nil means the asset is blank, so there is nothing to tear and the break
// lands on the asset itself: it is destroyed outright. That skips the tear and,
// with it, the secret-visibility grant the tear implies — the grant is a
// consequence of turning the marginalia over, and nothing is turned over here.
// Without this branch a blank asset is invulnerable: every break site names a
// marginalia, and DestroyIfAllMarginaliaTorn is guarded by an EXISTS on the same
// table. That guard is deliberately left alone — the blank case is settled
// before any tear, so the query's "all torn ⇒ destroyed" invariant stays honest.
//
// Callers are responsible for checking blankness before passing nil; see
// resolveBreakTarget, which does it from a request's optional marginalia id.
func breakAsset(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	asset *dbgen.Asset,
	m *dbgen.Marginalium,
	tornBy int64,
) (destroyed bool, err error) {
	if m == nil {
		if err = q.DestroyAsset(ctx, asset.ID); err != nil {
			return false, err
		}
		emitAssetDestruction(ctx, q, manager, asset)
		return true, nil
	}

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
		emitAssetDestruction(ctx, q, manager, asset)
		return true, nil
	}
	return false, nil
}

// emitAssetDestruction broadcasts asset.destroyed and writes the matching
// system post. Shared by breakAsset's two destroy paths so both reach clients
// and the action log identically.
func emitAssetDestruction(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, asset *dbgen.Asset) {
	broadcastEvent(manager, asset.GameID, model.EventAssetDestroyed, model.AssetIDPayload{AssetID: asset.ID})
	if game, gerr := q.GetGameByID(ctx, asset.GameID); gerr == nil {
		EmitAssetDestroyed(ctx, q, manager, asset.GameID, *asset, logRow(game))
	}
}

// assetIsBlank reports whether an asset carries no marginalia rows at all —
// not "no *intact* rows". Marginalia are append-only (tearing only flips
// is_torn, there is no DELETE anywhere), so blankness is fixed at creation:
// an asset that ever had a note can never become blank, and an asset whose
// notes are all torn is already destroyed. Breaking a blank asset destroys it
// outright (breakAsset with m == nil).
func assetIsBlank(ctx context.Context, q *dbgen.Queries, assetID int64) (bool, error) {
	n, err := q.CountMarginalia(ctx, assetID)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// assetIsBreakable reports whether a live asset can absorb a break: it either
// has an intact marginalia to tear, or it is blank and the break destroys it
// outright (breakAsset with m == nil). Only an all-torn-but-alive asset is
// unbreakable, and no such asset exists in a live game — the last tear always
// destroys. Callers must have already excluded destroyed assets.
//
// This is the eligibility counterpart to resolveBreakTarget: forfeit hatches and
// soft-lock guards refuse to discharge a pick while any eligible target remains,
// so the two must agree on what "eligible" means or a plan can wedge.
func assetIsBreakable(ctx context.Context, q *dbgen.Queries, assetID int64) (bool, error) {
	blank, err := assetIsBlank(ctx, q, assetID)
	if err != nil {
		return false, err
	}
	if blank {
		return true, nil // nothing to tear: breaking destroys it
	}
	intact, err := q.CountIntactMarginalia(ctx, assetID)
	if err != nil {
		return false, err
	}
	return intact > 0, nil
}

// resolveBreakTarget turns a request's optional marginalia id into the argument
// breakAsset wants. marginaliaID == 0 means "none named", which is legal only
// against a blank asset — the break then destroys it outright and nil is
// returned. Naming a marginalia that belongs to another asset, or is already
// torn, is rejected; so is omitting one on an asset that has notes to tear.
//
// Every route that lets a player choose a break target should resolve through
// this, so the "blank ⇒ destroy" rule is stated once rather than re-derived per
// plan.
func resolveBreakTarget(
	ctx context.Context, q *dbgen.Queries, asset *dbgen.Asset, marginaliaID int64,
) (*dbgen.Marginalium, error) {
	if marginaliaID == 0 {
		blank, err := assetIsBlank(ctx, q, asset.ID)
		if err != nil {
			return nil, httpErr(http.StatusInternalServerError, "could not inspect the asset's marginalia")
		}
		if !blank {
			return nil, httpErr(http.StatusBadRequest, "marginalia_id is required")
		}
		return nil, nil
	}
	m, err := q.GetMarginaliaByID(ctx, marginaliaID)
	if err != nil {
		return nil, httpErr(http.StatusNotFound, "marginalia not found")
	}
	if m.AssetID != asset.ID {
		return nil, httpErr(http.StatusBadRequest, "marginalia does not belong to the specified asset")
	}
	if m.IsTorn {
		return nil, httpErr(http.StatusConflict, "marginalia is already torn")
	}
	return &m, nil
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
