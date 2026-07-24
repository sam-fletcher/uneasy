package handler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

type festivityOptionContext struct {
	deps           *PlanDeps
	plan           *dbgen.Plan
	state          *gamepkg.FestivityResolutionData
	actingPlayerID int64
	rumorText      string
	peerName       string
	peerMarginalia []string
	assetID        int64
	marginaliaID   int64
	isMake         bool
}

type festivityOptionApplier func(ctx context.Context, fc *festivityOptionContext) error

// festivityOptionAppliers dispatches a make/mar choice to its mechanical
// effect. SpreadRumor and RumorAboutYou share an applier (it branches on
// fc.isMake) because the underlying rumor-creation flow is the same.
var festivityOptionAppliers = map[string]festivityOptionApplier{
	gamepkg.FestivityMakeSpreadRumor:    applyFestivityRumor,
	gamepkg.FestivityMarRumorAboutYou:   applyFestivityRumor,
	gamepkg.FestivityMakeIntroducePeer:  applyFestivityIntroducePeer,
	gamepkg.FestivityMakeTakeCenterPeer: applyFestivityTakeCenterPeer,
	gamepkg.FestivityMarDisagreement:    applyFestivityDisagreement,
	gamepkg.FestivityMarAcceptDuels:     applyFestivityAcceptDuels,
	gamepkg.FestivityMarBreakSelf:       applyFestivityBreakSelf,
}

// hfApplyOption performs the mechanical effect for a chosen make/mar option.
// It mutates state as needed (e.g. recording centered assets, accept_duels).
func hfApplyOption(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	state *gamepkg.FestivityResolutionData,
	actingPlayerID int64,
	choice, rumorText, peerName string,
	peerMarginalia []string,
	assetID, marginaliaID int64,
	isMake bool,
) error {
	applier, ok := festivityOptionAppliers[choice]
	if !ok {
		return nil
	}
	return applier(ctx, &festivityOptionContext{
		deps:           deps,
		plan:           plan,
		state:          state,
		actingPlayerID: actingPlayerID,
		peerMarginalia: peerMarginalia,
		rumorText:      rumorText,
		peerName:       peerName,
		assetID:        assetID,
		marginaliaID:   marginaliaID,
		isMake:         isMake,
	})
}

func applyFestivityRumor(ctx context.Context, fc *festivityOptionContext) error {
	txt := strings.TrimSpace(fc.rumorText)
	if txt == "" {
		return errors.New("rumor text is required")
	}
	if len([]rune(txt)) > maxLongTextLen {
		return fmt.Errorf("rumor text must be at most %d characters", maxLongTextLen)
	}
	var targetAssetID *int64
	if !fc.isMake {
		if mcID, err := hfFindMainCharacter(ctx, fc.deps, fc.plan.GameID, fc.actingPlayerID); err == nil {
			targetAssetID = &mcID
		}
	}
	existing, _ := fc.deps.Q.ListRumors(ctx, fc.plan.GameID)
	var src *int64
	if fc.isMake {
		src = &fc.actingPlayerID
	}
	rumor, err := fc.deps.Q.CreateRumor(ctx, dbgen.CreateRumorParams{
		GameID:         fc.plan.GameID,
		Text:           txt,
		TargetAssetID:  targetAssetID,
		OriginPlanID:   &fc.plan.ID,
		SourcePlayerID: src,
		DisplayOrder:   int16(len(existing)),
	})
	if err != nil {
		return fmt.Errorf("create rumor: %w", err)
	}
	broadcastEvent(fc.deps.Manager, fc.plan.GameID, model.EventRumorCreated, model.RumorCreatedPayload{Rumor: rumor})
	if fc.isMake {
		hfLog(ctx, fc.deps, fc.plan, model.SeverityDefault, fmt.Sprintf("%s spread a new rumor at the event.",
			playerDisplayName(ctx, fc.deps.Q, fc.actingPlayerID)))
	} else {
		hfLog(ctx, fc.deps, fc.plan, model.SeverityDefault, fmt.Sprintf("A rumor spread about %s.",
			playerDisplayName(ctx, fc.deps.Q, fc.actingPlayerID)))
	}
	return nil
}

func applyFestivityIntroducePeer(ctx context.Context, fc *festivityOptionContext) error {
	name := strings.TrimSpace(fc.peerName)
	if name == "" {
		return errors.New("peer name is required")
	}
	if len([]rune(name)) > maxAssetNameLen {
		return fmt.Errorf("peer name must be at most %d characters", maxAssetNameLen)
	}
	margText, err := requireOneMarginalia(fc.peerMarginalia)
	if err != nil {
		return err
	}
	ownerID := fc.actingPlayerID
	if fc.actingPlayerID == fc.plan.PreparerID {
		recipient, rerr := AssetRecipientForPlan(ctx, fc.deps.Q, fc.plan)
		if rerr != nil {
			return fmt.Errorf("resolve asset recipient: %w", rerr)
		}
		ownerID = recipient
	}
	var asset dbgen.Asset
	var marginalia []dbgen.Marginalium
	err = fc.deps.InTx(ctx, func(q *dbgen.Queries) error {
		var caErr error
		asset, marginalia, caErr = createAssetWithFirstMarginalia(ctx, q, dbgen.CreateAssetParams{
			GameID:    fc.plan.GameID,
			OwnerID:   ownerID,
			CreatorID: fc.actingPlayerID,
			AssetType: model.AssetPeer,
			Name:      name,
		}, margText)
		return caErr
	})
	if err != nil {
		return fmt.Errorf("create peer: %w", err)
	}
	// New peers are placed in the play area (center of table) for the
	// duration of the festivity — owned by their introducer but stealable
	// by other guests via take_center_peer.
	fc.state.CenteredAssetIDs = append(fc.state.CenteredAssetIDs, asset.ID)
	broadcastEvent(
		fc.deps.Manager,
		fc.plan.GameID,
		model.EventAssetCreated,
		model.AssetPayload{Asset: assetWithMarginalia{Asset: asset, Marginalia: marginalia}},
	)
	hfLog(
		ctx,
		fc.deps,
		fc.plan,
		model.SeverityDefault,
		fmt.Sprintf("%s introduced a new peer, %s, to the festivity, with marginalia: %q.",
			playerDisplayName(ctx, fc.deps.Q, fc.actingPlayerID), assetMark(asset.Name), margText),
	)
	return nil
}

func applyFestivityTakeCenterPeer(ctx context.Context, fc *festivityOptionContext) error {
	// The "center" referred to is the physical table in a real-life game;
	// for the digital game, this framing is not shown to players,
	// instead we focus on the narrative of peers considering changing retinues.
	if fc.assetID == 0 {
		return errors.New("asset_id required")
	}
	asset, err := fc.deps.Q.GetAssetByID(ctx, fc.assetID)
	if err != nil {
		return errors.New("asset not found")
	}
	if !slices.Contains(fc.state.CenteredAssetIDs, fc.assetID) {
		return errors.New("asset is not in available to take into your retinue")
	}
	oldOwner := asset.OwnerID
	newOwner := fc.actingPlayerID
	if fc.actingPlayerID == fc.plan.PreparerID {
		recipient, rerr := AssetRecipientForPlan(ctx, fc.deps.Q, fc.plan)
		if rerr != nil {
			return fmt.Errorf("resolve asset recipient: %w", rerr)
		}
		newOwner = recipient
	}
	updated, err := takeAssetEffect(ctx, fc.deps.Q, fc.deps.Manager, fc.plan.GameID, fc.assetID, oldOwner, newOwner)
	if err != nil {
		return fmt.Errorf("transfer asset: %w", err)
	}
	fc.state.CenteredAssetIDs = removeID(fc.state.CenteredAssetIDs, fc.assetID)
	// A peer that's taken won't rejoin its old owner broken — drop it from the
	// disagreement watch-list too.
	fc.state.DisagreementAssetIDs = removeID(fc.state.DisagreementAssetIDs, fc.assetID)
	hfLog(ctx, fc.deps, fc.plan, model.SeverityDefault, fmt.Sprintf("%s took %s into their retinue.",
		playerDisplayName(ctx, fc.deps.Q, fc.actingPlayerID), assetMark(updated.Name)))
	return nil
}

// hfLog emits a Host Festivity action-log entry anchored to the plan's row.
func hfLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	planID := plan.ID
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.host_festivity",
		severity, body, plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
}

func applyFestivityDisagreement(ctx context.Context, fc *festivityOptionContext) error {
	if fc.assetID == 0 {
		return errors.New("asset_id required for disagreement")
	}
	asset, err := fc.deps.Q.GetAssetByID(ctx, fc.assetID)
	if err != nil {
		return errors.New("asset not found")
	}
	if asset.AssetType != model.AssetPeer {
		return errors.New("disagreement target must be a peer")
	}
	// "Get into a disagreement with one of your peers" — the peer must belong
	// to the acting player.
	if asset.OwnerID != fc.actingPlayerID {
		return errors.New("you can only have a disagreement with one of your own peers")
	}
	if !slices.Contains(fc.state.CenteredAssetIDs, fc.assetID) {
		fc.state.CenteredAssetIDs = append(fc.state.CenteredAssetIDs, fc.assetID)
	}
	// Track that this peer reached the center via a disagreement: if no one takes
	// it before the event ends, it rejoins its owner broken (see
	// hfBreakAbandonedDisagreementPeers).
	if !slices.Contains(fc.state.DisagreementAssetIDs, fc.assetID) {
		fc.state.DisagreementAssetIDs = append(fc.state.DisagreementAssetIDs, fc.assetID)
	}
	hfLog(
		ctx,
		fc.deps,
		fc.plan,
		model.SeverityDefault,
		fmt.Sprintf("%s fell out with their peer %s, who is now considering changing retinue.",
			playerDisplayName(ctx, fc.deps.Q, fc.actingPlayerID), assetMark(asset.Name)),
	)
	return nil
}

func applyFestivityAcceptDuels(ctx context.Context, fc *festivityOptionContext) error {
	if !slices.Contains(fc.state.AcceptDuels, fc.actingPlayerID) {
		fc.state.AcceptDuels = append(fc.state.AcceptDuels, fc.actingPlayerID)
	}
	hfLog(
		ctx,
		fc.deps,
		fc.plan,
		model.SeverityDefault,
		fmt.Sprintf("%s must accept any duel challenge during the event.",
			playerDisplayName(ctx, fc.deps.Q, fc.actingPlayerID)),
	)
	return nil
}

// applyFestivityBreakSelf tears the acting player's chosen marginalia on their
// main character (auto-destroy if it was the last) via the canonical break
// helper. If no marginalia is specified, falls back to the first intact one —
// or, on a blank main character, to destroying it outright (D3), so an insisted
// break can still be discharged by a player whose MC carries no notes.
func applyFestivityBreakSelf(ctx context.Context, fc *festivityOptionContext) error {
	mcID, err := hfFindMainCharacter(ctx, fc.deps, fc.plan.GameID, fc.actingPlayerID)
	if err != nil {
		return fmt.Errorf("find main character: %w", err)
	}
	mc, err := fc.deps.Q.GetAssetByID(ctx, mcID)
	if err != nil {
		return fmt.Errorf("load main character: %w", err)
	}

	var m *dbgen.Marginalium
	if fc.marginaliaID != 0 {
		picked, gErr := fc.deps.Q.GetMarginaliaByID(ctx, fc.marginaliaID)
		if gErr != nil {
			return errors.New("marginalia not found")
		}
		if picked.AssetID != mcID {
			return errors.New("marginalia does not belong to your main character")
		}
		if picked.IsTorn {
			return errors.New("marginalia is already torn")
		}
		m = &picked
	} else {
		marg, listErr := fc.deps.Q.ListIntactMarginalia(ctx, mcID)
		if listErr != nil {
			return fmt.Errorf("list marginalia: %w", listErr)
		}
		if len(marg) > 0 {
			m = &marg[0]
		} else if blank, bErr := assetIsBlank(ctx, fc.deps.Q, mcID); bErr != nil || !blank {
			return errors.New("no intact marginalia to tear")
		}
		// m stays nil on a blank MC — breakAsset destroys it outright.
	}

	destroyed, err := breakAsset(ctx, fc.deps.Q, fc.deps.Manager, &mc, m, fc.actingPlayerID)
	if err != nil {
		return fmt.Errorf("break self: %w", err)
	}
	hfLog(
		ctx,
		fc.deps,
		fc.plan,
		model.SeverityDefault,
		fmt.Sprintf("%s %s themselves — word of their gaffe gets around.%s",
			playerDisplayName(ctx, fc.deps.Q, fc.actingPlayerID), breakVerb(destroyed),
			brokenAssetDetail(ctx, fc.deps.Q, mc.OwnerID, m, destroyed)),
	)
	return nil
}

// removeID returns ids with the first occurrence of target removed, preserving
// order. Returns a fresh slice (empty, never sharing the input's backing array).
func removeID(ids []int64, target int64) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != target {
			out = append(out, id)
		}
	}
	return out
}

// hfBreakAbandonedDisagreementPeers settles the tail end of every disagreement
// when the event winds down: a peer shoved to the center that no guest took
// "rejoins its owner, broken." The peer never actually changed hands (a
// disagreement keeps the owner), so rejoining is just dropping the centered flag;
// "broken" tears one marginalia, destroying the peer if it was the last.
//
// Unlike the insisted break_self / disagreement *choices* (which the owner picks,
// via resolve-host-mar), this break is automatic: the rules frame the rejoin as a
// consequence of not being taken, not a decision. Owner is the actor.
func hfBreakAbandonedDisagreementPeers(
	ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, state *gamepkg.FestivityResolutionData,
) error {
	for _, id := range state.DisagreementAssetIDs {
		asset, err := deps.Q.GetAssetByID(ctx, id)
		if err != nil || asset.IsDestroyed {
			continue // already gone — nothing to break
		}
		// A blank peer has nothing to tear, so its break destroys it outright
		// (D3); only an all-torn-but-alive peer (unreachable in a live game) is
		// skipped here.
		var m *dbgen.Marginalium
		marg, err := deps.Q.ListIntactMarginalia(ctx, id)
		if err != nil {
			return fmt.Errorf("list marginalia for abandoned peer %d: %w", id, err)
		}
		switch {
		case len(marg) > 0:
			m = &marg[0]
		default:
			blank, bErr := assetIsBlank(ctx, deps.Q, id)
			if bErr != nil {
				return fmt.Errorf("inspect abandoned peer %d: %w", id, bErr)
			}
			if !blank {
				continue
			}
		}
		destroyed, err := breakAsset(ctx, deps.Q, deps.Manager, &asset, m, asset.OwnerID)
		if err != nil {
			return fmt.Errorf("break abandoned peer %d: %w", id, err)
		}
		owner := playerDisplayName(ctx, deps.Q, asset.OwnerID)
		detail := brokenAssetDetail(ctx, deps.Q, asset.OwnerID, m, destroyed)
		if destroyed {
			hfLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
				"%s never made up with %s and fell apart for good.%s", assetMark(asset.Name), owner, detail))
		} else {
			hfLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
				"%s rejoined %s, broken by the falling-out.%s", assetMark(asset.Name), owner, detail))
		}
		state.CenteredAssetIDs = removeID(state.CenteredAssetIDs, id)
	}
	state.DisagreementAssetIDs = nil
	return nil
}

// hfHostCanBreakSelf reports whether an insisted break_self has anything to
// land on: the host's main character either has an un-torn marginalia to tear,
// or is blank, in which case the break destroys it outright (D3). Only an
// all-torn-but-alive MC — a state a live game never reaches — has nothing.
func hfHostCanBreakSelf(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (bool, error) {
	mcID, err := hfFindMainCharacter(ctx, deps, plan.GameID, plan.PreparerID)
	if err != nil {
		return false, err
	}
	return assetIsBreakable(ctx, deps.Q, mcID)
}

func hfFindMainCharacter(ctx context.Context, deps *PlanDeps, gameID, playerID int64) (int64, error) {
	assets, err := deps.Q.ListAssetsByOwner(ctx, playerID)
	if err != nil {
		return 0, err
	}
	for _, a := range assets {
		if a.GameID == gameID && a.IsMainCharacter {
			return a.ID, nil
		}
	}
	return 0, errors.New("no main character found")
}
