package handler

// handler/plan_propose_decree.go — Propose Decree plan handler (Phase 3c).
//
// Propose Decree (power, delay 4): The preparer convenes a council, drafts
// a decree, and rallies the powerful to put it into law.
//
// Difficulty: preparer's rank on the power track.
//
// Pre-Roll (Council Meeting):
//   - The preparer states the drafted decree.
//   - The council is auto-seated at OnResolve: the preparer plus every player
//     ranked ABOVE them on power (they attend without leveraging). Lower-ranked
//     players may still join by leveraging ≥1 asset via join-council.
//   - The highest-power member is the signatory.
//   - The signatory calls the roll when discussion is done.
//
// The law row is created at enact (make-choice) time with the decree body and
// no addendum, then UPDATED IN PLACE as the law is written:
//
//   Make: a resource asset is created; the signatory places the addendum.
//   Mar:  NO asset; the non-preparer council members rewrite the body in turn
//         (lowest power first, each working from the previous output), THEN the
//         signatory places the addendum.
//
// The final law = amended body + an "and"/"but" connector + the signatory's
// (optional) rider text. Completion is gated on all amendments done AND the
// addendum placed (a required step even if the rider text is blank).
//
// Follow Scene: Your character interacting with a law.
//
// Extra routes:
//   POST /api/plans/:planId/join-council   Lower-ranked player joins by leveraging assets.
//   POST /api/plans/:planId/call-roll      Signatory closes council; creates dice roll.
//   POST /api/plans/:planId/amend-decree   (Mar) current amender rewrites the law body.
//   POST /api/plans/:planId/set-addendum   Signatory places the and/but addendum.
//   POST /api/plans/:planId/name-resource  (Make) preparer names the created resource.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanProposeDecree, pdHandler{})
}

type pdHandler struct{}

func (pdHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryPower, Delay: 4}
}

func (pdHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (*int16, string) {
	if v.Notes == "" {
		return nil, "propose_decree requires preparation_notes describing the proposed decree"
	}
	return nil, ""
}

func (pdHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	rank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryPower)
	if err != nil {
		return 0, fmt.Errorf("could not determine preparer power rank: %w", err)
	}
	return gamepkg.ProposeDecreeDifficulty(rank), nil
}

// Resolution authority is intentionally shared but PREPARER-ANCHORED (decided
// in the 2026-05 rules audit): the signatory has real sway during resolution —
// they call the roll (call-roll) and write the addendum (set-addendum), both
// signatory-gated — and on a mar the other council members gate progress by
// amending the preparer's text (narrative, in the scene thread). But the
// PREPARER remains the plan's resolver: only they submit make-choice (which
// enacts the law and, on a make, creates the resource asset) and complete the
// plan, consistent with every other plan. This holds even when a higher-power
// council member is the signatory.
//
// OnResolve initialises the council: sets the default signatory to the
// preparer and returns nil (the dice roll is created later by call-roll,
// once the council meeting is complete).
func (pdHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)
	pd := resData.EnsureProposeDecree()

	if len(pd.SignatoryPlayerIDs) == 0 {
		// Auto-seat the council: the preparer plus every player ranked ABOVE
		// them on the power track (they attend without leveraging an asset).
		// Lower-ranked players may still join later via join-council.
		council, err := pdAutoSeatCouncil(ctx, deps.Q, plan)
		if err != nil {
			return nil, err
		}
		pd.SignatoryPlayerIDs = council
		// Signatory = the highest-power member (lowest rank number).
		sig, err := pdHighestPowerMember(ctx, deps.Q, plan.GameID, council)
		if err != nil {
			return nil, err
		}
		pd.SignatoryID = &sig
	}

	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("could not initialise decree resolution data: %w", err)
	}

	// Return nil — the roll is created later when the signatory calls call-roll.
	return nil, nil
}

// pdAutoSeatCouncil returns the initial council: the preparer plus every player
// ranked above them on the power track (Monarch and higher-status peers attend
// automatically). Order is unspecified; signatory/amender ordering is computed
// from ranks separately.
func pdAutoSeatCouncil(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) ([]int64, error) {
	preparerRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryPower)
	if err != nil {
		return nil, fmt.Errorf("preparer power rank: %w", err)
	}
	ranks, err := q.ListRankingsByGame(ctx, plan.GameID)
	if err != nil {
		return nil, fmt.Errorf("list rankings: %w", err)
	}
	council := []int64{plan.PreparerID}
	for _, rk := range ranks {
		if rk.Category != model.CategoryPower || rk.PlayerID == nil {
			continue
		}
		// Strictly higher power = lower rank number than the preparer.
		if rk.Rank < preparerRank && *rk.PlayerID != plan.PreparerID {
			council = append(council, *rk.PlayerID)
		}
	}
	return council, nil
}

// pdHighestPowerMember returns the council member with the best (lowest) power
// rank — the signatory.
func pdHighestPowerMember(ctx context.Context, q *dbgen.Queries, gameID int64, council []int64) (int64, error) {
	best := int64(0)
	bestRank := int16(0)
	for _, id := range council {
		rank, err := playerRankInCategory(ctx, q, gameID, id, model.CategoryPower)
		if err != nil {
			return 0, fmt.Errorf("member power rank: %w", err)
		}
		if best == 0 || rank < bestRank {
			best = id
			bestRank = rank
		}
	}
	return best, nil
}

// ApplyChoice creates the law record. On make, it also creates a resource
// asset representing the enacted law; on mar it records the law without
// a corresponding asset.
func (pdHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	_ []string,
	result string,
) error {
	var preparationNotes string
	if plan.PreparationNotes != nil {
		preparationNotes = *plan.PreparationNotes
	}

	// Compute the next display_order for laws in this game.
	laws, err := deps.Q.ListLaws(ctx, plan.GameID)
	if err != nil {
		return fmt.Errorf("could not list laws: %w", err)
	}
	displayOrder := int16(len(laws) + 1)

	pd := resData.EnsureProposeDecree()

	// The law row is created now (so it appears in the Laws panel immediately)
	// with the decree body and no addendum yet. The signatory's addendum and,
	// on a mar, the council's amendments update this row in place before the
	// plan completes.
	planID := plan.ID
	law, err := deps.Q.CreateLaw(ctx, dbgen.CreateLawParams{
		GameID:       plan.GameID,
		Text:         preparationNotes,
		Addendum:     nil,
		OriginPlanID: &planID,
		SignatoryID:  pd.SignatoryID,
		DisplayOrder: displayOrder,
	})
	if err != nil {
		return fmt.Errorf("could not create law: %w", err)
	}

	pd.LawID = &law.ID
	pd.LawText = preparationNotes

	if result == makeOutcome {
		if err = pdCreateLawAsset(ctx, deps, plan, resData, preparationNotes); err != nil {
			return err
		}
	} else {
		// Mar: the non-preparer council members amend the body in turn, lowest
		// power first. Compute that order now.
		order, err := pdAmendmentOrder(ctx, deps.Q, plan, pd.SignatoryPlayerIDs)
		if err != nil {
			return err
		}
		pd.AmendmentOrder = order
	}

	broadcastEvent(deps.Manager, plan.GameID, model.EventLawEnacted, model.LawEnactedPayload{
		PlanID: plan.ID,
		Law:    law,
	})

	signatory := "the council"
	if pd.SignatoryID != nil {
		signatory = playerDisplayName(ctx, deps.Q, *pd.SignatoryID)
	}
	if result == makeOutcome {
		pdLog(
			ctx,
			deps,
			plan,
			model.SeverityImportant,
			fmt.Sprintf(
				"The decree was enacted, signed by %s, and a new resource was created. Awaiting the addendum.",
				signatory,
			),
		)
	} else {
		pdLog(
			ctx,
			deps,
			plan,
			model.SeverityImportant,
			fmt.Sprintf(
				"The decree passed but was marred — the council amends it (lowest power first) before %s adds the addendum.",
				signatory,
			),
		)
	}

	return nil
}

// pdAmendmentOrder returns the non-preparer council members ordered by power,
// lowest power (highest rank number) first — the order they amend a marred law.
func pdAmendmentOrder(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan, council []int64) ([]int64, error) {
	type member struct {
		id   int64
		rank int16
	}
	members := make([]member, 0, len(council))
	for _, id := range council {
		if id == plan.PreparerID {
			continue
		}
		rank, err := playerRankInCategory(ctx, q, plan.GameID, id, model.CategoryPower)
		if err != nil {
			return nil, fmt.Errorf("council member power rank: %w", err)
		}
		members = append(members, member{id: id, rank: rank})
	}
	// Lowest power first = highest rank number first.
	slices.SortFunc(members, func(a, b member) int { return int(b.rank) - int(a.rank) })
	order := make([]int64, len(members))
	for i, m := range members {
		order[i] = m.id
	}
	return order, nil
}

// pdLog emits a Propose Decree action-log entry anchored to the plan's row.
func pdLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	planID := plan.ID
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.propose_decree",
		severity, body, plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
}

// pdCreateLawAsset creates the resource asset that accompanies a made law.
// Owner is the signatory if present, otherwise the recipient determined by
// AssetRecipientForPlan (which honors a keep_assets Make Demands winner).
//
// The asset is created with a neutral placeholder name; the preparer names it
// afterwards via the name-asset route. (Deliberately NOT derived from the law
// text — the resource represents the law's worldly consequence, which the
// preparer narrates, not a copy of the decree's wording.) The created asset id
// is recorded so the naming step knows what to rename.
func pdCreateLawAsset(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	_ string,
) error {
	pd := resData.EnsureProposeDecree()

	var ownerID int64
	if pd.SignatoryID != nil {
		ownerID = *pd.SignatoryID
	} else {
		recipient, err := gamepkg.AssetRecipientForPlan(ctx, deps.Q, plan)
		if err != nil {
			return fmt.Errorf("resolve asset recipient: %w", err)
		}
		ownerID = recipient
	}

	asset, err := deps.Q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:    plan.GameID,
		OwnerID:   ownerID,
		CreatorID: plan.PreparerID,
		AssetType: model.AssetResource,
		Name:      lawResourceNameDefault,
	})
	if err != nil {
		return fmt.Errorf("could not create law resource asset: %w", err)
	}

	pd.ResourceAssetID = &asset.ID

	broadcastEvent(
		deps.Manager,
		plan.GameID,
		model.EventAssetCreated,
		model.AssetPayload{Asset: assetWithMarginalia{Asset: asset, Marginalia: []dbgen.Marginalium{}}},
	)
	return nil
}

// CanComplete gates completion on the full law-writing sequence: the law must
// be enacted (make-choice), on a mar every council amender must have taken
// their turn, and the signatory must have placed their addendum (even if blank).
func (pdHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	pd := resData.ProposeDecree
	if pd == nil || pd.LawID == nil {
		return errors.New("make-choice must be submitted before the plan can be completed")
	}
	if next := pd.NextAmender(); next != 0 {
		return errors.New("the council is still amending the law")
	}
	if !pd.AddendumPlaced {
		return errors.New("the signatory must place their addendum before completing")
	}
	return nil
}

func (pdHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"join-council":  pdJoinCouncilHandler(deps),
		"call-roll":     pdCallRollHandler(deps),
		"amend-decree":  pdAmendDecreeHandler(deps),
		"set-addendum":  pdSetAddendumHandler(deps),
		"name-resource": pdNameAssetHandler(deps),
	}
}

// ── Name Asset ────────────────────────────────────────────────────────────────

// pdNameAssetHandler handles POST /api/plans/:planId/name-resource.
//
// The preparer names the resource asset the made decree created (it starts with
// a placeholder). Optional; does not gate completion.
func pdNameAssetHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nameCreatedPlanAsset(w, r, deps, model.PlanProposeDecree,
			func(rd *ResolutionData) *int64 {
				if rd.ProposeDecree == nil {
					return nil
				}
				return rd.ProposeDecree.ResourceAssetID
			},
			func(rd *ResolutionData) { rd.EnsureProposeDecree().ResourceNamed = true },
		)
	}
}

// ── Join Council ──────────────────────────────────────────────────────────────

// pdJoinCouncilHandler handles POST /api/plans/:planId/join-council.
//
// A player joins the council by leveraging ≥1 of their assets. Eligible
// players: rank 1 on the power track (Monarch), or any player ranked above
// the preparer on the power track.
//
// After joining, the signatory is recalculated: the player with the best
// (lowest) power rank among all council members becomes the new signatory.
//
// Request body: {"asset_ids": [N, ...]}
//
//nolint:funlen,gocognit // signatory selection with eligibility branches
func pdJoinCouncilHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "join-council is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		var body struct {
			AssetIDs []int64 `json:"asset_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.AssetIDs) == 0 {
			respondErr(w, http.StatusBadRequest, "asset_ids (non-empty array) is required")
			return
		}

		ctx := r.Context()

		// Determine preparer's power rank.
		preparerRank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, plan.PreparerID, model.CategoryPower)
		if err != nil {
			respondInternalErr(w, r, "could not determine preparer power rank", err)
			return
		}

		// Check if the joining player is the preparer themselves — they're already in.
		if player.ID == plan.PreparerID {
			respondErr(w, http.StatusConflict, "the preparer is already in the council")
			return
		}

		// Determine the joiner's power rank.
		joinerRank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, player.ID, model.CategoryPower)
		if err != nil {
			respondInternalErr(w, r, "could not determine your power rank", err)
			return
		}

		// Eligibility: joiner must be rank 1 (Monarch) or rank < preparer's rank.
		if joinerRank != 1 && joinerRank >= preparerRank {
			respondErr(w, http.StatusForbidden,
				"only the Monarch or players ranked above the preparer may join the council")
			return
		}

		// Verify and leverage each specified asset.
		for _, assetID := range body.AssetIDs {
			asset, err := deps.Q.GetAssetByID(ctx, assetID)
			if err != nil {
				respondErr(w, http.StatusNotFound, fmt.Sprintf("asset %d not found", assetID))
				return
			}
			if asset.GameID != plan.GameID {
				respondErr(w, http.StatusBadRequest, "asset does not belong to this game")
				return
			}
			if asset.OwnerID != player.ID {
				respondErr(w, http.StatusForbidden, "you do not own this asset")
				return
			}
			if asset.IsLeveraged {
				respondErr(w, http.StatusConflict, fmt.Sprintf("asset %d is already leveraged", assetID))
				return
			}
			if err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
				ID:          assetID,
				IsLeveraged: true,
			}); err != nil {
				respondInternalErr(w, r, "could not leverage asset", err)
				return
			}
			broadcastEvent(deps.Manager, plan.GameID, model.EventAssetLeveraged, model.AssetIDPayload{
				AssetID:  assetID,
				PlayerID: player.ID,
			})
		}

		resData := loadResolutionData(plan.ResolutionData)
		pd := resData.EnsureProposeDecree()

		// Add to council if not already there.
		if !slices.Contains(pd.SignatoryPlayerIDs, player.ID) {
			pd.SignatoryPlayerIDs = append(pd.SignatoryPlayerIDs, player.ID)
		}

		// Recompute signatory: best rank (lowest) among all council members.
		bestRank := preparerRank
		bestPlayerID := plan.PreparerID
		for _, memberID := range pd.SignatoryPlayerIDs {
			memberRank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, memberID, model.CategoryPower)
			if err != nil {
				continue
			}
			if memberRank < bestRank {
				bestRank = memberRank
				bestPlayerID = memberID
			}
		}
		pd.SignatoryID = &bestPlayerID

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save council data", err)
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, model.EventDecreeCouncilJoined, model.DecreeCouncilJoinedPayload{
			PlanID:      plan.ID,
			PlayerID:    player.ID,
			SignatoryID: *pd.SignatoryID,
		})

		pdLog(
			ctx,
			deps,
			plan,
			model.SeverityDefault,
			fmt.Sprintf("%s joined the council; %s now holds the signatory's pen.",
				playerDisplayName(ctx, deps.Q, player.ID), playerDisplayName(ctx, deps.Q, *pd.SignatoryID)),
		)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":      plan.ID,
			"player_id":    player.ID,
			"signatory_id": *pd.SignatoryID,
			"council":      pd.SignatoryPlayerIDs,
		})
	}
}

// ── Call Roll ─────────────────────────────────────────────────────────────────

// pdCallRollHandler handles POST /api/plans/:planId/call-roll.
//
// The signatory closes the council meeting and triggers the dice roll.
// Only the current signatory may call the roll.
func pdCallRollHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "call-roll is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		pd := resData.ProposeDecree
		if pd == nil || pd.SignatoryID == nil || *pd.SignatoryID != player.ID {
			respondErr(w, http.StatusForbidden, "only the current signatory may call the roll")
			return
		}

		// Verify there's no existing roll for this plan.
		existingRoll, rollErr := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
		if rollErr == nil && existingRoll.ID != 0 {
			respondErr(w, http.StatusConflict, "a roll has already been created for this plan")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load game", err)
			return
		}

		difficulty, err := pdHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
		if err != nil {
			respondInternalErr(w, r, "could not compute difficulty", err)
			return
		}

		// The preparer is the actor; the roll uses preparer's dice.
		roll, err := createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
		if err != nil {
			respondInternalErr(w, r, "could not create dice roll", err)
			return
		}

		pdLog(
			ctx,
			deps,
			plan,
			model.SeverityDefault,
			fmt.Sprintf("%s closed the council and called the roll on the decree.",
				playerDisplayName(ctx, deps.Q, player.ID)),
		)

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"roll":    roll,
		})
	}
}

// ── Amend Decree (mar) ────────────────────────────────────────────────────────

// pdAmendDecreeHandler handles POST /api/plans/:planId/amend-decree.
//
// On a marred decree, the non-preparer council members rewrite the law body in
// turn, lowest power first (the order computed at enact). Each amender submits
// the full revised body, which replaces the law's text; the next amender works
// from that output. Only the current NextAmender() may submit.
//
// Request body: {"text": "the revised full law body"}
func pdAmendDecreeHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "amend-decree is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		pd := resData.EnsureProposeDecree()
		if pd.LawID == nil {
			respondErr(w, http.StatusConflict, "the decree has not been enacted yet")
			return
		}
		next := pd.NextAmender()
		if next == 0 {
			respondErr(w, http.StatusConflict, "the council has finished amending the law")
			return
		}
		if player.ID != next {
			respondErr(w, http.StatusConflict, "it is not your turn to amend the law")
			return
		}

		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
			respondErr(w, http.StatusBadRequest, "text (the revised law body) is required")
			return
		}

		if err := pdComposeLaw(ctx, deps.Q, *pd.LawID, body.Text, pd); err != nil {
			respondInternalErr(w, r, "could not update law text", err)
			return
		}
		pd.AmendedBy = append(pd.AmendedBy, player.ID)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save amendment", err)
			return
		}

		pdLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s amended the decree's text.", playerDisplayName(ctx, deps.Q, player.ID)))
		pdBroadcastLaw(ctx, deps, plan, *pd.LawID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"amended":   player.ID,
			"next":      pd.NextAmender(),
			"remaining": len(pd.AmendmentOrder) - len(pd.AmendedBy),
		})
	}
}

// ── Set Addendum ──────────────────────────────────────────────────────────────

// pdSetAddendumHandler handles POST /api/plans/:planId/set-addendum.
//
// The signatory attaches their rider to the law: an "and"/"but" connector plus
// optional free text. This is a required blocking step (AddendumPlaced) — the
// signatory must confirm it (even with blank text) before the plan completes.
// Only valid once the law is enacted and (on a mar) the council has finished
// amending.
//
// Request body: {"connector": "and"|"but", "addendum": "free text"}
func pdSetAddendumHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "set-addendum is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		pd := resData.EnsureProposeDecree()
		if pd.SignatoryID == nil || *pd.SignatoryID != player.ID {
			respondErr(w, http.StatusForbidden, "only the current signatory may set the addendum")
			return
		}
		if pd.LawID == nil {
			respondErr(w, http.StatusConflict, "the decree has not been enacted yet")
			return
		}
		if next := pd.NextAmender(); next != 0 {
			respondErr(w, http.StatusConflict, "the council must finish amending before the addendum is placed")
			return
		}

		var body struct {
			Connector string `json:"connector"`
			Addendum  string `json:"addendum"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		// A connector is only required when there's addendum text to attach.
		if strings.TrimSpace(body.Addendum) != "" && body.Connector != "and" && body.Connector != "but" {
			respondErr(w, http.StatusBadRequest, "connector must be 'and' or 'but' when an addendum is provided")
			return
		}

		pd.Addendum = strings.TrimSpace(body.Addendum)
		pd.AddendumConnector = body.Connector
		pd.AddendumPlaced = true

		// Re-fetch the (possibly amended) law body and rewrite text+addendum.
		law, err := deps.Q.GetLawByID(ctx, *pd.LawID)
		if err != nil {
			respondInternalErr(w, r, "could not load law", err)
			return
		}
		if err := pdComposeLaw(ctx, deps.Q, *pd.LawID, law.Text, pd); err != nil {
			respondInternalErr(w, r, "could not save addendum", err)
			return
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save addendum", err)
			return
		}

		pdLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s placed the signatory's addendum.", playerDisplayName(ctx, deps.Q, player.ID)))
		pdBroadcastLaw(ctx, deps, plan, *pd.LawID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"addendum":  pd.Addendum,
			"connector": pd.AddendumConnector,
		})
	}
}

// pdComposeLaw writes the law body and the composed addendum rider to the law
// row. The addendum column holds "<connector> <text>" (e.g. "but salt is
// exempt"), or NULL when the signatory left it blank.
func pdComposeLaw(
	ctx context.Context,
	q *dbgen.Queries,
	lawID int64,
	body string,
	pd *ProposeDecreeResolutionData,
) error {
	var addendumPtr *string
	if pd.AddendumPlaced && pd.Addendum != "" {
		rider := pd.Addendum
		if pd.AddendumConnector != "" {
			rider = pd.AddendumConnector + " " + pd.Addendum
		}
		addendumPtr = &rider
	}
	_, err := q.UpdateLawText(ctx, dbgen.UpdateLawTextParams{
		ID:       lawID,
		Text:     body,
		Addendum: addendumPtr,
	})
	if err == nil {
		pd.LawText = body
	}
	return err
}

// pdBroadcastLaw re-broadcasts the law row so clients refresh the Laws panel
// after an amendment or addendum edit.
func pdBroadcastLaw(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, lawID int64) {
	law, err := deps.Q.GetLawByID(ctx, lawID)
	if err != nil {
		return
	}
	broadcastEvent(deps.Manager, plan.GameID, model.EventLawEnacted, model.LawEnactedPayload{
		PlanID: plan.ID,
		Law:    law,
	})
}
