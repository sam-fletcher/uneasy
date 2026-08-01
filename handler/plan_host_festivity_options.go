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

// festivityOptionInput carries the option-specific parameters a make/mar choice
// may need — the union of every applier's inputs, since one route serves all of
// them. Each applier reads only the fields its own option uses.
type festivityOptionInput struct {
	// ActingPlayerID is who the effect lands on. For host makes and insisted
	// host mars that is plan.PreparerID, not necessarily the caller.
	ActingPlayerID int64
	Choice         string
	RumorText      string   // spread_rumor, rumor_about_you
	PeerName       string   // introduce_peer
	PeerMarginalia []string // introduce_peer
	// AssetID names a real asset: the peer to fall out with (disagreement), or
	// a disagreement peer to take from the center.
	AssetID int64
	// DraftID names a centered draft to take instead — a peer introduced at the
	// party, who owns no asset row until claimed (D7). Exactly one of AssetID
	// and DraftID is set on take_center_peer.
	DraftID      string
	MarginaliaID int64 // break_self
	IsMake       bool
}

type festivityOptionContext struct {
	festivityOptionInput

	deps    *PlanDeps
	plan    *dbgen.Plan
	resData *ResolutionData
	state   *gamepkg.FestivityResolutionData
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
// It mutates the festivity sub-state as needed (recording centered peers,
// accept_duels, …); the caller persists resData once the option has applied.
// Appliers that write to the database take resData with them so their own
// transaction can save the matching state change — see applyFestivityTakeCenterPeer.
func hfApplyOption(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	in festivityOptionInput,
) error {
	applier, ok := festivityOptionAppliers[in.Choice]
	if !ok {
		return nil
	}
	return applier(ctx, &festivityOptionContext{
		festivityOptionInput: in,
		deps:                 deps,
		plan:                 plan,
		resData:              resData,
		state:                resData.EnsureFestivity(),
	})
}

func applyFestivityRumor(ctx context.Context, fc *festivityOptionContext) error {
	txt := strings.TrimSpace(fc.RumorText)
	if txt == "" {
		return errors.New("rumor text is required")
	}
	if len([]rune(txt)) > maxLongTextLen {
		return fmt.Errorf("rumor text must be at most %d characters", maxLongTextLen)
	}
	var targetAssetID *int64
	if !fc.IsMake {
		if mcID, err := hfFindMainCharacter(ctx, fc.deps, fc.plan.GameID, fc.ActingPlayerID); err == nil {
			targetAssetID = &mcID
		}
	}
	existing, _ := fc.deps.Q.ListRumors(ctx, fc.plan.GameID)
	var src *int64
	if fc.IsMake {
		src = &fc.ActingPlayerID
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
	if fc.IsMake {
		hfLog(ctx, fc.deps, fc.plan, model.SeverityDefault, fmt.Sprintf("%s spread a new rumor at the event.",
			playerDisplayName(ctx, fc.deps.Q, fc.ActingPlayerID)))
	} else {
		hfLog(ctx, fc.deps, fc.plan, model.SeverityDefault, fmt.Sprintf("A rumor spread about %s.",
			playerDisplayName(ctx, fc.deps.Q, fc.ActingPlayerID)))
	}
	return nil
}

// applyFestivityIntroducePeer records the new peer as a *draft* — name and the
// one marginalia the introducer wrote, and no `assets` row at all. Per D7 the
// peer belongs to nobody while they work the room: any guest may claim them via
// take_center_peer, and whoever is left unclaimed leaves with their introducer
// when the event ends (hfSettleUnclaimedCenteredDrafts). Creation is deferred to
// that moment, so the "introduce a new peer *(if still at table → add to
// retinue)*" rule no longer starts by contradicting itself.
func applyFestivityIntroducePeer(ctx context.Context, fc *festivityOptionContext) error {
	name := strings.TrimSpace(fc.PeerName)
	if name == "" {
		return errors.New("peer name is required")
	}
	if len([]rune(name)) > maxAssetNameLen {
		return fmt.Errorf("peer name must be at most %d characters", maxAssetNameLen)
	}
	margText, err := requireOneMarginalia(fc.PeerMarginalia)
	if err != nil {
		return err
	}
	fc.state.CenteredDrafts = append(fc.state.CenteredDrafts, gamepkg.DraftPeer{
		ID:         gamepkg.NewDraftPeerID(),
		Name:       name,
		Marginalia: margText,
		CreatorID:  fc.ActingPlayerID,
	})
	hfLog(
		ctx,
		fc.deps,
		fc.plan,
		model.SeverityDefault,
		fmt.Sprintf("%s introduced a new peer, %s, to the festivity, with marginalia: %q.",
			playerDisplayName(ctx, fc.deps.Q, fc.ActingPlayerID), assetMark(name), margText),
	)
	return nil
}

// applyFestivityTakeCenterPeer claims one of the peers at the centre of the
// party. Two kinds sit there and players see one list, so the request names
// whichever applies: a draft introduced at the event (draft_id — a first claim,
// materialized straight into the claimant's retinue) or a real peer shoved to
// the centre by a disagreement (asset_id — an ordinary transfer from the owner
// who fell out with them).
func applyFestivityTakeCenterPeer(ctx context.Context, fc *festivityOptionContext) error {
	// The "center" referred to is the physical table in a real-life game;
	// for the digital game, this framing is not shown to players,
	// instead we focus on the narrative of peers considering changing retinues.
	if fc.DraftID != "" {
		return hfClaimCenteredDraft(ctx, fc)
	}
	if fc.AssetID == 0 {
		return errors.New("asset_id or draft_id required")
	}
	asset, err := fc.deps.Q.GetAssetByID(ctx, fc.AssetID)
	if err != nil {
		return errors.New("asset not found")
	}
	if !slices.Contains(fc.state.CenteredAssetIDs, fc.AssetID) {
		return errors.New("asset is not in available to take into your retinue")
	}
	newOwner, err := hfAssetOwnerFor(ctx, fc.deps, fc.plan, fc.ActingPlayerID)
	if err != nil {
		return err
	}
	updated, err := takeAssetEffect(
		ctx,
		fc.deps.Q,
		fc.deps.Manager,
		fc.plan.GameID,
		fc.AssetID,
		asset.OwnerID,
		newOwner,
	)
	if err != nil {
		return fmt.Errorf("transfer asset: %w", err)
	}
	fc.state.CenteredAssetIDs = removeID(fc.state.CenteredAssetIDs, fc.AssetID)
	// A peer that's taken won't rejoin its old owner broken — drop it from the
	// disagreement watch-list too.
	fc.state.DisagreementAssetIDs = removeID(fc.state.DisagreementAssetIDs, fc.AssetID)
	hfLog(ctx, fc.deps, fc.plan, model.SeverityDefault, fmt.Sprintf("%s took %s into their retinue.",
		playerDisplayName(ctx, fc.deps.Q, fc.ActingPlayerID), assetMark(updated.Name)))
	return nil
}

// hfClaimCenteredDraft is the draft half of take_center_peer: a first claim, not
// a transfer. There is no prior owner and no secret to inherit (the peer has
// existed only as a draft), so it never touches takeAssetEffect — arrival *is*
// creation (D4). The draft leaves the centre in the same transaction that
// creates the asset, so a failed write can't strand a peer who is both claimed
// and still claimable.
func hfClaimCenteredDraft(ctx context.Context, fc *festivityOptionContext) error {
	draft := gamepkg.FindDraftPeer(fc.state.CenteredDrafts, fc.DraftID)
	if draft == nil {
		return errors.New("that peer is no longer available to take into your retinue")
	}
	claimed := *draft
	newOwner, err := hfAssetOwnerFor(ctx, fc.deps, fc.plan, fc.ActingPlayerID)
	if err != nil {
		return err
	}
	centered := fc.state.CenteredDrafts
	var asset dbgen.Asset
	err = fc.deps.InTx(ctx, func(q *dbgen.Queries) error {
		a, _, mErr := materializeDraftPeer(ctx, q, fc.deps.Manager, fc.plan.GameID, claimed, newOwner)
		if mErr != nil {
			return mErr
		}
		asset = a
		fc.state.CenteredDrafts = gamepkg.RemoveDraftPeer(centered, claimed.ID)
		return saveResolutionData(ctx, q, fc.plan.ID, *fc.resData)
	})
	if err != nil {
		fc.state.CenteredDrafts = centered // the transaction rolled back; so does the state
		return fmt.Errorf("claim peer: %w", err)
	}
	hfLog(ctx, fc.deps, fc.plan, model.SeverityDefault, fmt.Sprintf("%s took %s into their retinue.",
		playerDisplayName(ctx, fc.deps.Q, fc.ActingPlayerID), assetMark(asset.Name)))
	return nil
}

// hfAssetOwnerFor resolves who actually receives an asset that acting player
// gains during this festivity. Everyone receives their own — except the host,
// whose gains are redirected by a made Make Demands' keep_assets winner.
func hfAssetOwnerFor(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, actingPlayerID int64) (int64, error) {
	if actingPlayerID != plan.PreparerID {
		return actingPlayerID, nil
	}
	recipient, err := AssetRecipientForPlan(ctx, deps.Q, plan)
	if err != nil {
		return 0, fmt.Errorf("resolve asset recipient: %w", err)
	}
	return recipient, nil
}

// hfLog emits a Host Festivity action-log entry anchored to the plan's row.
func hfLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	planID := plan.ID
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.host_festivity",
		severity, body, plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
}

func applyFestivityDisagreement(ctx context.Context, fc *festivityOptionContext) error {
	if fc.AssetID == 0 {
		return errors.New("asset_id required for disagreement")
	}
	asset, err := fc.deps.Q.GetAssetByID(ctx, fc.AssetID)
	if err != nil {
		return errors.New("asset not found")
	}
	if asset.AssetType != model.AssetPeer {
		return errors.New("disagreement target must be a peer")
	}
	// "Get into a disagreement with one of your peers" — the peer must belong
	// to the acting player.
	if asset.OwnerID != fc.ActingPlayerID {
		return errors.New("you can only have a disagreement with one of your own peers")
	}
	if !slices.Contains(fc.state.CenteredAssetIDs, fc.AssetID) {
		fc.state.CenteredAssetIDs = append(fc.state.CenteredAssetIDs, fc.AssetID)
	}
	// Track that this peer reached the center via a disagreement: if no one takes
	// it before the event ends, it rejoins its owner broken (see
	// hfBreakAbandonedDisagreementPeers).
	if !slices.Contains(fc.state.DisagreementAssetIDs, fc.AssetID) {
		fc.state.DisagreementAssetIDs = append(fc.state.DisagreementAssetIDs, fc.AssetID)
	}
	hfLog(
		ctx,
		fc.deps,
		fc.plan,
		model.SeverityDefault,
		fmt.Sprintf("%s fell out with their peer %s, who is now considering changing retinue.",
			playerDisplayName(ctx, fc.deps.Q, fc.ActingPlayerID), assetMark(asset.Name)),
	)
	return nil
}

func applyFestivityAcceptDuels(ctx context.Context, fc *festivityOptionContext) error {
	if !slices.Contains(fc.state.AcceptDuels, fc.ActingPlayerID) {
		fc.state.AcceptDuels = append(fc.state.AcceptDuels, fc.ActingPlayerID)
	}
	hfLog(
		ctx,
		fc.deps,
		fc.plan,
		model.SeverityDefault,
		fmt.Sprintf("%s must accept any duel challenge during the event.",
			playerDisplayName(ctx, fc.deps.Q, fc.ActingPlayerID)),
	)
	return nil
}

// applyFestivityBreakSelf tears the acting player's chosen marginalia on their
// main character (auto-destroy if it was the last) via the canonical break
// helper. If no marginalia is specified, falls back to the first intact one —
// or, on a blank main character, to destroying it outright (D3), so an insisted
// break can still be discharged by a player whose MC carries no notes.
func applyFestivityBreakSelf(ctx context.Context, fc *festivityOptionContext) error {
	mcID, err := hfFindMainCharacter(ctx, fc.deps, fc.plan.GameID, fc.ActingPlayerID)
	if err != nil {
		return fmt.Errorf("find main character: %w", err)
	}
	mc, err := fc.deps.Q.GetAssetByID(ctx, mcID)
	if err != nil {
		return fmt.Errorf("load main character: %w", err)
	}

	var m *dbgen.Marginalium
	if fc.MarginaliaID != 0 {
		picked, gErr := fc.deps.Q.GetMarginaliaByID(ctx, fc.MarginaliaID)
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

	destroyed, err := breakAsset(ctx, fc.deps.Q, fc.deps.Manager, &mc, m, fc.ActingPlayerID)
	if err != nil {
		return fmt.Errorf("break self: %w", err)
	}
	hfLog(
		ctx,
		fc.deps,
		fc.plan,
		model.SeverityDefault,
		fmt.Sprintf("%s %s themselves — word of their gaffe gets around.%s",
			playerDisplayName(ctx, fc.deps.Q, fc.ActingPlayerID), breakVerb(destroyed),
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
	if len(state.DisagreementAssetIDs) == 0 {
		return nil
	}

	// Two queries for the whole centre instead of the two-or-three per peer
	// this loop used to spend (GetAssetByID, ListIntactMarginalia, and
	// assetIsBlank's CountMarginalia). Prefetching is safe here because the ids
	// are deduped on the way in (hfDisagree appends only when absent) and each
	// iteration's break touches only its own asset, so no earlier iteration can
	// stale another's row.
	assetRows, err := deps.Q.ListAssetsByIDs(ctx, state.DisagreementAssetIDs)
	if err != nil {
		return fmt.Errorf("list abandoned peers: %w", err)
	}
	assetByID := make(map[int64]dbgen.Asset, len(assetRows))
	for _, a := range assetRows {
		assetByID[a.ID] = a
	}

	// Torn rows come back too, and that is the point: blankness is "no
	// marginalia rows at all", so an empty intact list alone can't tell a blank
	// peer (breaks by being destroyed outright) from an all-torn one (skipped).
	margRows, err := deps.Q.ListMarginaliaByAssets(ctx, state.DisagreementAssetIDs)
	if err != nil {
		return fmt.Errorf("list marginalia for abandoned peers: %w", err)
	}
	intactByAsset := make(map[int64][]dbgen.Marginalium, len(assetRows))
	hasAnyMarginalia := make(map[int64]bool, len(assetRows))
	for _, m := range margRows {
		hasAnyMarginalia[m.AssetID] = true
		if !m.IsTorn {
			intactByAsset[m.AssetID] = append(intactByAsset[m.AssetID], m)
		}
	}

	for _, id := range state.DisagreementAssetIDs {
		asset, found := assetByID[id]
		if !found || asset.IsDestroyed {
			continue // already gone — nothing to break
		}
		// A blank peer has nothing to tear, so its break destroys it outright
		// (D3); only an all-torn-but-alive peer (unreachable in a live game) is
		// skipped here.
		var m *dbgen.Marginalium
		marg := intactByAsset[id]
		switch {
		case len(marg) > 0:
			m = &marg[0]
		default:
			if hasAnyMarginalia[id] { // not blank — every note already torn
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

// hfSettleUnclaimedCenteredDrafts settles the other tail of the evening: a peer
// introduced at the party whom no guest claimed leaves with the player who
// introduced them (D7). That is the moment they become a real asset — until now
// they existed only as a draft, which is why an unclaimed peer can't have been
// broken, taken or leveraged while they worked the room.
//
// Every arrival and the emptied centre commit together, so a failure can't
// leave the same draft both materialized and still claimable. The host's own
// introductions still respect a made Make Demands' keep_assets winner, exactly
// as they did when the asset was created up front.
func hfSettleUnclaimedCenteredDrafts(
	ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, resData *ResolutionData,
) error {
	state := resData.EnsureFestivity()
	centered := state.CenteredDrafts
	if len(centered) == 0 {
		return nil
	}
	owners := make([]int64, len(centered))
	for i, d := range centered {
		owner, err := hfAssetOwnerFor(ctx, deps, plan, d.CreatorID)
		if err != nil {
			return err
		}
		owners[i] = owner
	}

	assets := make([]dbgen.Asset, len(centered))
	err := deps.InTx(ctx, func(q *dbgen.Queries) error {
		for i, d := range centered {
			a, _, mErr := materializeDraftPeer(ctx, q, deps.Manager, plan.GameID, d, owners[i])
			if mErr != nil {
				return mErr
			}
			assets[i] = a
		}
		state.CenteredDrafts = nil
		return saveResolutionData(ctx, q, plan.ID, *resData)
	})
	if err != nil {
		state.CenteredDrafts = centered // the transaction rolled back; so does the state
		return fmt.Errorf("settle unclaimed peers: %w", err)
	}

	for i, a := range assets {
		hfLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
			"Nobody else claimed %s, who left with %s.",
			assetMark(a.Name), playerDisplayName(ctx, deps.Q, owners[i])))
	}
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
