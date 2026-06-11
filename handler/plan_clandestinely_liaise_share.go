package handler

import (
	"context"
	"fmt"
	"strings"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

type liaiseChoiceApplier func(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choice dbgen.ListLiaiseChoicesByPlanRow,
) error

var liaiseChoiceAppliers = map[string]liaiseChoiceApplier{
	liaiseChoiceLookAtSecret:    applyLookAtSecret,
	liaiseChoiceBreakPeer:       applyBreakPeer,
	liaiseChoiceLeveragePartner: applyLeveragePartner,
	liaiseChoiceTakeGift:        applyTakeGift,
	liaiseChoiceUpdatePeer:      applyUpdatePeer,
}

// clApplyShareChoices applies the mechanical effects of both players' choices
// after both have submitted.
func clApplyShareChoices(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	_ ResolutionData,
	choices []dbgen.ListLiaiseChoicesByPlanRow,
) error {
	for _, choice := range choices {
		applier, ok := liaiseChoiceAppliers[choice.Choice]
		if !ok {
			continue
		}
		if err := applier(ctx, deps, plan, choice); err != nil {
			return err
		}
	}
	return nil
}

func applyLookAtSecret(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choice dbgen.ListLiaiseChoicesByPlanRow,
) error {
	if choice.TargetAssetID == nil {
		return nil
	}
	if err := deps.Q.GrantSecretVisibilityForAsset(ctx, dbgen.GrantSecretVisibilityForAssetParams{
		AssetID:  *choice.TargetAssetID,
		PlayerID: choice.PlayerID,
	}); err != nil {
		return fmt.Errorf("could not grant secret visibility: %w", err)
	}
	broadcastEvent(deps.Manager, plan.GameID, model.EventSecretVisibilityGrant, model.SecretVisibilityGrantPayload{
		AssetID:  *choice.TargetAssetID,
		PlayerID: choice.PlayerID,
	})
	clLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s looked at the secrets of %s.",
		playerDisplayName(ctx, deps.Q, choice.PlayerID), clAssetName(ctx, deps, choice.TargetAssetID)))
	return nil
}

// applyUpdatePeer rewrites one marginalia on the partner's meeting peer with
// the actor-authored replacement text (recorded on the choice). "Updating" an
// asset means editing one of its marginalia — tearing is reserved for break.
// If the peer was destroyed, or the chosen marginalia was torn, before
// resolution (e.g. broken in another plan), there is nothing to update: log a
// no-op rather than implying a change.
func applyUpdatePeer(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choice dbgen.ListLiaiseChoicesByPlanRow,
) error {
	noop := func() {
		clLog(ctx, deps, plan, model.SeverityMinor, fmt.Sprintf(
			"%s could not update their partner's meeting peer — it no longer exists.",
			playerDisplayName(ctx, deps.Q, choice.PlayerID)))
	}

	if choice.TargetAssetID == nil || choice.TargetMarginaliaID == nil || choice.UpdateText == nil {
		noop()
		return nil
	}
	asset, err := deps.Q.GetAssetByID(ctx, *choice.TargetAssetID)
	if err != nil {
		return fmt.Errorf("update_peer: target asset not found: %w", err)
	}
	if asset.IsDestroyed {
		noop()
		return nil
	}
	m, err := deps.Q.GetMarginaliaByID(ctx, *choice.TargetMarginaliaID)
	if err != nil {
		return fmt.Errorf("update_peer: marginalia not found: %w", err)
	}
	if m.IsTorn {
		noop()
		return nil
	}

	newText := strings.TrimSpace(*choice.UpdateText)
	if newText == "" {
		noop()
		return nil
	}
	if err := deps.Q.UpdateMarginaliaText(ctx, dbgen.UpdateMarginaliaTextParams{
		ID:   m.ID,
		Text: newText,
	}); err != nil {
		return fmt.Errorf("could not update partner's peer marginalia: %w", err)
	}
	m.Text = newText
	broadcastEvent(deps.Manager, plan.GameID, model.EventMarginaliaUpdated, model.MarginaliaPayload{
		AssetID:    asset.ID,
		Marginalia: m,
	})

	clLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s rewrote a marginalia on their partner's peer %q.",
		playerDisplayName(ctx, deps.Q, choice.PlayerID), asset.Name))
	return nil
}

// applyBreakPeer tears the breaker's chosen marginalia on the partner's chosen
// peer (auto-destroy if it was the last) via the canonical break helper. The
// target asset + marginalia were validated at submission time.
func applyBreakPeer(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choice dbgen.ListLiaiseChoicesByPlanRow,
) error {
	if choice.TargetAssetID == nil || choice.TargetMarginaliaID == nil {
		return nil
	}
	asset, err := deps.Q.GetAssetByID(ctx, *choice.TargetAssetID)
	if err != nil {
		return fmt.Errorf("break_peer: target asset not found: %w", err)
	}
	if asset.IsDestroyed {
		// The meeting peer was destroyed before resolution (e.g. broken in
		// another plan). Nothing left to tear — log a no-op.
		clLog(ctx, deps, plan, model.SeverityMinor, fmt.Sprintf(
			"%s could not break their partner's meeting peer — it was already destroyed.",
			playerDisplayName(ctx, deps.Q, choice.PlayerID)))
		return nil
	}
	m, err := deps.Q.GetMarginaliaByID(ctx, *choice.TargetMarginaliaID)
	if err != nil {
		return fmt.Errorf("break_peer: marginalia not found: %w", err)
	}
	if m.IsTorn {
		return nil // Already torn (e.g. both players targeted the same one).
	}
	destroyed, err := breakMarginalia(ctx, deps.Q, deps.Manager, &asset, &m, choice.PlayerID)
	if err != nil {
		return fmt.Errorf("could not break partner's peer: %w", err)
	}
	clLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf("%s %s their partner's peer %q.",
		playerDisplayName(ctx, deps.Q, choice.PlayerID), breakVerb(destroyed), asset.Name))
	return nil
}

func applyLeveragePartner(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choice dbgen.ListLiaiseChoicesByPlanRow,
) error {
	if choice.TargetAssetID != nil {
		if err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
			ID:          *choice.TargetAssetID,
			IsLeveraged: true,
		}); err != nil {
			return fmt.Errorf("could not leverage partner asset: %w", err)
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventAssetLeveraged, model.AssetIDPayload{
			AssetID:  *choice.TargetAssetID,
			PlayerID: choice.PlayerID,
		})
	}
	// Bank a die for a future roll. The die rolls a random face at resolution
	// time like any other die — banked dice do not carry a pre-determined face.
	if _, err := deps.Q.CreateBankedDie(ctx, dbgen.CreateBankedDieParams{
		GameID:   plan.GameID,
		PlayerID: choice.PlayerID,
		Source:   "liaise",
	}); err != nil {
		return fmt.Errorf("could not bank die: %w", err)
	}
	clLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s leveraged %s and banked a die for a future roll.",
		playerDisplayName(ctx, deps.Q, choice.PlayerID), clAssetName(ctx, deps, choice.TargetAssetID)))
	return nil
}

// applyTakeGift transfers the partner-owned target asset to the chooser.
// Consent is social; the server transfers ownership.
func applyTakeGift(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choice dbgen.ListLiaiseChoicesByPlanRow,
) error {
	if choice.TargetAssetID == nil {
		return nil
	}
	asset, err := deps.Q.GetAssetByID(ctx, *choice.TargetAssetID)
	if err != nil {
		return fmt.Errorf("take_gift: target asset not found: %w", err)
	}
	oldOwner := asset.OwnerID
	if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
		ID:      *choice.TargetAssetID,
		OwnerID: choice.PlayerID,
	}); err != nil {
		return fmt.Errorf("could not transfer gift asset: %w", err)
	}
	updated, _ := deps.Q.GetAssetByID(ctx, *choice.TargetAssetID)
	broadcastEvent(deps.Manager, plan.GameID, model.EventAssetTaken, model.AssetTakenPayload{
		Asset:      updated,
		OldOwnerID: oldOwner,
		NewOwnerID: choice.PlayerID,
	})
	clLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf("%s took %q from their partner as a gift.",
		playerDisplayName(ctx, deps.Q, choice.PlayerID), asset.Name))
	return nil
}

// clAssetName resolves an asset id to its name for log bodies; the
// fallbackAssetName placeholder on a nil id or any failure so the log line
// still reads cleanly.
func clAssetName(ctx context.Context, deps *PlanDeps, assetID *int64) string {
	if assetID == nil {
		return fallbackAssetName
	}
	return assetDisplayName(ctx, deps.Q, *assetID)
}

// clLog emits a Clandestinely Liaise action-log entry anchored to the plan row.
func clLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	planID := plan.ID
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.clandestinely_liaise",
		severity, body, plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
}
