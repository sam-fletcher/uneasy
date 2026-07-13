package handler

// handler/shake_up_effects.go — Mechanical effects for each committed
// Shake-Up spend: take/break an asset, bump a rank, or claim a new title.
// See shake_up.go for the phase's full lifecycle.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// applyShakeUpEffect dispatches the option's mechanical effect.
func applyShakeUpEffect(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	finalCost int16,
) error {
	info, err := gamepkg.ShakeUpOption(spend.OptionKey)
	if err != nil {
		return err
	}
	switch spend.OptionKey {
	case gamepkg.ShakeUpOptTakePeer, gamepkg.ShakeUpOptTakeArtifact,
		gamepkg.ShakeUpOptTakeResource, gamepkg.ShakeUpOptTakeHolding:
		return shakeUpTakeAsset(ctx, q, manager, gameID, spend, expectedTakeType(spend.OptionKey), finalCost)
	case gamepkg.ShakeUpOptBreakResource, gamepkg.ShakeUpOptBreakHolding,
		gamepkg.ShakeUpOptBreakPeer, gamepkg.ShakeUpOptBreakArtifact:
		return shakeUpBreakAsset(ctx, q, manager, gameID, spend, expectedBreakType(spend.OptionKey), finalCost)
	case gamepkg.ShakeUpOptBumpEsteem, gamepkg.ShakeUpOptBumpKnowledge, gamepkg.ShakeUpOptBumpPower:
		return shakeUpBumpRank(ctx, q, manager, gameID, spend, info.BumpsTrack, finalCost)
	case gamepkg.ShakeUpOptClaimTitle:
		return shakeUpClaimTitle(ctx, q, manager, gameID, spend, finalCost)
	}
	return errors.New("no applier for option")
}

func expectedTakeType(opt string) model.AssetType {
	switch opt {
	case gamepkg.ShakeUpOptTakePeer:
		return model.AssetPeer
	case gamepkg.ShakeUpOptTakeArtifact:
		return model.AssetArtifact
	case gamepkg.ShakeUpOptTakeResource:
		return model.AssetResource
	case gamepkg.ShakeUpOptTakeHolding:
		return model.AssetHolding
	}
	return ""
}

func expectedBreakType(opt string) model.AssetType {
	switch opt {
	case gamepkg.ShakeUpOptBreakResource:
		return model.AssetResource
	case gamepkg.ShakeUpOptBreakHolding:
		return model.AssetHolding
	case gamepkg.ShakeUpOptBreakPeer:
		return model.AssetPeer
	case gamepkg.ShakeUpOptBreakArtifact:
		return model.AssetArtifact
	}
	return ""
}

func shakeUpTakeAsset(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	want model.AssetType,
	finalCost int16,
) error {
	if spend.TargetAssetID == nil {
		return errors.New("target_asset_id required")
	}
	asset, err := q.GetAssetByID(ctx, *spend.TargetAssetID)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}
	if asset.GameID != gameID {
		return errors.New("target asset belongs to another game")
	}
	if asset.AssetType != want {
		return fmt.Errorf("target must be a %s asset", want)
	}
	if asset.IsDestroyed {
		return errors.New("asset is destroyed")
	}
	if asset.OwnerID == spend.PlayerID {
		return errors.New("cannot take your own asset")
	}
	oldOwner := asset.OwnerID
	if _, err = takeAssetEffect(ctx, q, manager, gameID, asset.ID, oldOwner, spend.PlayerID); err != nil {
		return fmt.Errorf("transfer: %w", err)
	}
	spender := playerDisplayName(ctx, q, spend.PlayerID)
	from := playerDisplayName(ctx, q, oldOwner)
	EmitShakeUpCommitted(
		ctx,
		q,
		manager,
		gameID,
		spend,
		finalCost,
		fmt.Sprintf(
			"%s spent %d token(s) to take %s (%s) from %s",
			spender,
			finalCost,
			assetMark(asset.Name),
			want,
			from,
		),
		map[string]any{"effect": "take", "asset_id": asset.ID, "old_owner_id": oldOwner},
	)
	return nil
}

// validateShakeUpBreakTarget checks an announce-time break target: a marginalia
// must be named, exist, belong to the named asset, and still be intact. It
// writes the error response and returns false on any failure. The apply step
// (shakeUpBreakAsset) re-checks authoritatively at commit, since the marginalia
// could be torn by another action while the spend is open.
func validateShakeUpBreakTarget(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	info gamepkg.ShakeUpOptionInfo,
	targetAssetID, targetMarginaliaID *int64,
) bool {
	if targetMarginaliaID == nil {
		respondErr(w, http.StatusBadRequest, "target_marginalia_id required for "+info.Key)
		return false
	}
	m, err := q.GetMarginaliaByID(ctx, *targetMarginaliaID)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "target marginalia not found")
		return false
	}
	if targetAssetID == nil || m.AssetID != *targetAssetID {
		respondErr(w, http.StatusBadRequest, "marginalia does not belong to the target asset")
		return false
	}
	if m.IsTorn {
		respondErr(w, http.StatusConflict, "marginalia is already torn")
		return false
	}
	return true
}

// validateShakeUpTakeTarget checks an announce-time take target: the asset
// must exist, belong to this game, be the option's asset type, be intact, and
// NOT be owned by the spender (taking your own asset is a meaningless no-op —
// ruling 8). It writes the error response and returns false on any failure.
// shakeUpTakeAsset re-checks ownership authoritatively at commit, since the
// asset could change hands while the spend is open. Caller guarantees
// targetAssetID is non-nil (the option's NeedsAsset check runs first).
func validateShakeUpTakeTarget(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	gameID, spenderID int64,
	want model.AssetType,
	targetAssetID *int64,
) bool {
	asset, err := q.GetAssetByID(ctx, *targetAssetID)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "target asset not found")
		return false
	}
	if asset.GameID != gameID {
		respondErr(w, http.StatusBadRequest, "target asset belongs to another game")
		return false
	}
	if asset.AssetType != want {
		respondErr(w, http.StatusBadRequest, fmt.Sprintf("target must be a %s asset", want))
		return false
	}
	if asset.IsDestroyed {
		respondErr(w, http.StatusConflict, "asset is destroyed")
		return false
	}
	if asset.OwnerID == spenderID {
		respondErr(w, http.StatusForbidden, "you cannot take your own asset")
		return false
	}
	return true
}

// shakeUpBreakAsset applies a "break a … asset" spend by tearing the single
// marginalia the breaker chose — the canonical break (see breakMarginalia):
// "tear off one marginalia = breaking an asset; all 4 gone → destroyed". This
// also grants the breaker visibility on the asset's secrets and, via
// EmitMarginaliaTorn, writes the standard marginalia.torn log with its
// "how has it changed?" prompt — none of which the old whole-asset DestroyAsset
// did. The shake_up.committed post still records the token spend for the ledger.
func shakeUpBreakAsset(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	want model.AssetType,
	finalCost int16,
) error {
	if spend.TargetAssetID == nil {
		return errors.New("target_asset_id required")
	}
	if spend.TargetMarginaliaID == nil {
		return errors.New("target_marginalia_id required")
	}
	asset, err := q.GetAssetByID(ctx, *spend.TargetAssetID)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}
	if asset.GameID != gameID {
		return errors.New("target asset belongs to another game")
	}
	if asset.AssetType != want {
		return fmt.Errorf("target must be a %s asset", want)
	}
	if asset.IsDestroyed {
		return errors.New("asset is already destroyed")
	}
	m, err := q.GetMarginaliaByID(ctx, *spend.TargetMarginaliaID)
	if err != nil {
		return fmt.Errorf("marginalia not found: %w", err)
	}
	if m.AssetID != asset.ID {
		return errors.New("marginalia does not belong to the target asset")
	}
	if m.IsTorn {
		return errors.New("marginalia is already torn")
	}

	// Canonical tear + destroy-if-last + secret-visibility grant to the breaker.
	destroyed, err := breakMarginalia(ctx, q, manager, &asset, &m, spend.PlayerID)
	if err != nil {
		return fmt.Errorf("tear marginalia: %w", err)
	}
	// breakMarginalia doesn't log the tear; emit the canonical marginalia.torn
	// post (with the owner re-describe prompt) like every other break.
	if g, gErr := q.GetGameByID(ctx, gameID); gErr == nil {
		EmitMarginaliaTorn(ctx, q, manager, gameID, asset, m, spend.PlayerID, destroyed, g.CurrentRow)
	}

	spender := playerDisplayName(ctx, q, spend.PlayerID)
	owner := playerDisplayName(ctx, q, asset.OwnerID)
	body := fmt.Sprintf(
		"%s spent %d token(s) to break %s's %s (%s)",
		spender,
		finalCost,
		owner,
		assetMark(asset.Name),
		want,
	)
	if destroyed {
		body += ", destroying it"
	}
	EmitShakeUpCommitted(
		ctx,
		q,
		manager,
		gameID,
		spend,
		finalCost,
		body,
		map[string]any{
			"effect":        "break",
			"asset_id":      asset.ID,
			"owner_id":      asset.OwnerID,
			"marginalia_id": m.ID,
			"destroyed":     destroyed,
		},
	)
	return nil
}

// shakeUpBumpRank moves spender up one rank on the target track and pushes
// whoever was above them down one slot. Dummies are passed over.
func shakeUpBumpRank(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	track string,
	finalCost int16,
) error {
	playerID := spend.PlayerID
	cat := model.RankingCategory(track)
	spender := playerDisplayName(ctx, q, playerID)
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return fmt.Errorf("load rankings: %w", err)
	}
	// Build the category's rank→occupant map (nil = dummy/empty slot) so we can
	// climb past dummies. Dummy tokens occupy real rank slots (e.g. rank 1 in
	// 2–3p games), so "rank N-1" is not necessarily a real player — bumping must
	// pass over dummies and swap with the first *real* player above, matching the
	// engrailed ranking update (swapTokenPlayerWithAbove). Swapping into a dummy's
	// slot would corrupt the track and let a top-real player illegitimately "rise".
	occupant := map[int16]*int64{}
	var current int16
	for _, rk := range rankings {
		if rk.Category != cat {
			continue
		}
		occupant[rk.Rank] = rk.PlayerID
		if rk.PlayerID != nil && *rk.PlayerID == playerID {
			current = rk.Rank
		}
	}
	// Search upward from current-1 for the first real player to overtake; skip
	// dummy (nil) slots. No real player above → the spender is effectively at the
	// top, so the bump is a (logged) no-op.
	var target int16
	var displaced *int64
	for r := current - 1; r >= 1; r-- {
		if occupant[r] != nil {
			target = r
			displaced = occupant[r]
			break
		}
	}
	if target == 0 {
		// Already at the top (only dummies / nothing above) — the token is still
		// spent, nothing moves. The rules dwell on spends that change nothing, so
		// log it anyway.
		EmitShakeUpCommitted(ctx, q, manager, gameID, spend, finalCost,
			fmt.Sprintf("%s spent %d token(s) to rise on %s, but is already at the top — no change",
				spender, finalCost, shakeUpCategoryTitle(track)),
			map[string]any{"effect": "bump", "track": track, "changed": false})
		return nil
	}
	pid := playerID
	if err = q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: gameID, PlayerID: &pid, Category: cat, Rank: target,
	}); err != nil {
		return fmt.Errorf("set bumped rank: %w", err)
	}
	if err = q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: gameID, PlayerID: displaced, Category: cat, Rank: current,
	}); err != nil {
		return fmt.Errorf("set displaced rank: %w", err)
	}
	updated, _ := q.ListRankingsByGame(ctx, gameID)
	broadcastEvent(manager, gameID, model.EventRankingsUpdated, model.RankingsUpdatedPayload{Rankings: updated})
	body := fmt.Sprintf("%s spent %d token(s) to rise to rank %d on %s",
		spender, finalCost, target, shakeUpCategoryTitle(track))
	if displaced != nil {
		body += fmt.Sprintf(" (displacing %s)", playerDisplayName(ctx, q, *displaced))
	}
	EmitShakeUpCommitted(ctx, q, manager, gameID, spend, finalCost, body,
		map[string]any{"effect": "bump", "track": track, "changed": true, "new_rank": target})
	return nil
}

// shakeUpTitleAlreadyClaimed reports whether titleID has been claimed anywhere in
// the game (ADR-007 game-wide uniqueness). It counts every claim site — Prologue
// choosing-phase claims, the ≤3-player extra-peer claim, and prior Shake-Up
// claims — and counts torn / destroyed claims too: a deposed monarch's title is
// still "already claimed", so it can't be re-minted onto a fresh peer.
func shakeUpTitleAlreadyClaimed(ctx context.Context, q *dbgen.Queries, gameID int64, titleID string) (bool, error) {
	claimed, err := q.ListClaimedTitleIDsByGame(ctx, gameID)
	if err != nil {
		return false, fmt.Errorf("load claimed titles: %w", err)
	}
	for _, c := range claimed {
		if c != nil && *c == titleID {
			return true, nil
		}
	}
	return false, nil
}

// validateShakeUpClaimTitle checks an announce-time claim-title spend: the chosen
// title must be a real, game-wide-unclaimed title, and the receiving asset must
// be one of the claimer's own peers with a free marginalia slot. It writes the
// error response and returns false on any failure. shakeUpClaimTitle re-checks
// authoritatively at commit (another claim could land while this spend is open).
func validateShakeUpClaimTitle(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	gameID, playerID int64,
	titleID *string,
	targetAssetID *int64,
) bool {
	if titleID == nil || gamepkg.TitleChoiceByID(*titleID) == nil {
		respondErr(w, http.StatusBadRequest, "target_title_id must be a valid title")
		return false
	}
	claimed, err := shakeUpTitleAlreadyClaimed(ctx, q, gameID, *titleID)
	if err != nil {
		respondInternalErr(w, nil, "could not check claimed titles", err)
		return false
	}
	if claimed {
		respondErr(w, http.StatusConflict, "that title has already been claimed")
		return false
	}
	if targetAssetID == nil {
		respondErr(w, http.StatusBadRequest, "target_asset_id (the peer to title) required for claim_title")
		return false
	}
	asset, err := q.GetAssetByID(ctx, *targetAssetID)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "target asset not found")
		return false
	}
	if asset.OwnerID != playerID {
		respondErr(w, http.StatusForbidden, "you can only title your own peer")
		return false
	}
	if asset.AssetType != model.AssetPeer || asset.IsDestroyed {
		respondErr(w, http.StatusBadRequest, "the title must be stamped on one of your peers")
		return false
	}
	pos, err := findOpenMarginaliaPosition(ctx, q, asset.ID)
	if err != nil {
		respondInternalErr(w, nil, "could not inspect marginalia", err)
		return false
	}
	if pos == 0 {
		respondErr(w, http.StatusConflict, "that peer has no free marginalia slot for a title")
		return false
	}
	return true
}

// shakeUpClaimTitle stamps a freshly claimed title as a marginalia on one of the
// claimer's peers (ADR-007). Unlike the pre-ADR stub — which minted a generic
// "New Title" artifact invisible to the line of succession — this routes through
// the same CreateTitleMarginalia + EstablishThrone path the Prologue uses, so a
// monarchy or heir claimed here is a real, contestable title: it trips the throne
// gate when it's the monarch, and currentMonarch / Propose Decree / the crown UI
// all pick it up. No artifact and no playing cards are created — the role lives
// on the marginalia.
func shakeUpClaimTitle(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	spend *dbgen.ShakeUpSpend,
	finalCost int16,
) error {
	if spend.TargetTitleID == nil {
		return errors.New("target_title_id required")
	}
	titleID := *spend.TargetTitleID
	choice := gamepkg.TitleChoiceByID(titleID)
	if choice == nil {
		return errors.New("unknown title")
	}
	// Re-check uniqueness at commit: another player may have claimed this title
	// while the spend was open.
	claimed, err := shakeUpTitleAlreadyClaimed(ctx, q, gameID, titleID)
	if err != nil {
		return err
	}
	if claimed {
		return errors.New("that title has already been claimed")
	}
	if spend.TargetAssetID == nil {
		return errors.New("target_asset_id required")
	}
	asset, err := q.GetAssetByID(ctx, *spend.TargetAssetID)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}
	if asset.OwnerID != spend.PlayerID || asset.AssetType != model.AssetPeer || asset.IsDestroyed {
		return errors.New("the title must be stamped on one of your own peers")
	}
	pos, err := findOpenMarginaliaPosition(ctx, q, asset.ID)
	if err != nil {
		return fmt.Errorf("inspect marginalia: %w", err)
	}
	if pos == 0 {
		return errors.New("that peer has no free marginalia slot for a title")
	}

	// The marginalia text is the player's freeform flavor; the title id is the
	// immutable role. Default to the title's display name when no flavor is given.
	text := choice.Name
	if spend.TitleFlavor != nil && strings.TrimSpace(*spend.TitleFlavor) != "" {
		text = strings.TrimSpace(*spend.TitleFlavor)
	}
	m, err := q.CreateTitleMarginalia(ctx, dbgen.CreateTitleMarginaliaParams{
		AssetID:  asset.ID,
		Position: pos,
		Text:     text,
		Title:    &titleID,
	})
	if err != nil {
		return fmt.Errorf("create title marginalia: %w", err)
	}

	// Claiming the monarch title trips the one-way throne gate the succession
	// hinges on — exactly as the Prologue claim does.
	if titleID == gamepkg.TitleMonarch {
		if err = q.EstablishThrone(ctx, gameID); err != nil {
			return fmt.Errorf("establish throne: %w", err)
		}
	}

	// Broadcast the new marginalia so connected clients update the peer's card and
	// flip throne_established live (ws-handlers' establishThroneIfMonarch) — that's
	// what lights up the Phase D crown UI without a refresh.
	broadcastEvent(manager, gameID, model.EventMarginaliaAdded, model.MarginaliaPayload{
		AssetID:    asset.ID,
		Marginalia: m,
	})

	spender := playerDisplayName(ctx, q, spend.PlayerID)
	EmitShakeUpCommitted(ctx, q, manager, gameID, spend, finalCost,
		fmt.Sprintf("%s spent %d token(s) to claim the title %s on %s",
			spender, finalCost, assetMark(choice.Name), assetMark(asset.Name)),
		map[string]any{
			"effect":        "claim_title",
			"title":         titleID,
			"asset_id":      asset.ID,
			"marginalia_id": m.ID,
		})
	return nil
}
