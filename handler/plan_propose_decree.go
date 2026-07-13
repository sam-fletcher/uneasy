package handler

// handler/plan_propose_decree.go — Propose Decree plan handler (Phase 3c).
//
// Propose Decree (power, delay 4): The preparer convenes a council, drafts
// a decree, and rallies the powerful to put it into law.
//
// Difficulty: preparer's rank on the power track.
//
// Pre-Roll (Council Meeting):
//   - The council is auto-seated at OnResolve: the preparer, every player
//     ranked ABOVE them on power, and the monarchOwner (any rank) when a throne
//     is established — all attend without leveraging. Lower-ranked players may
//     still join by leveraging exactly one asset via join-council, or opt out
//     via decline-council.
//   - The preparer finalizes the decree's text and opens the debate
//     (start-debate), which posts the proposed law to the chat. Lower-ranked
//     players may join/decline before or after this.
//   - The signatory is the monarchOwner if a throne is established, else the
//     highest-power member. It is fixed when the council is seated.
//   - The signatory calls the roll to close the debate — only once it has been
//     opened AND every eligible player has joined or declined.
//
// The law is written entirely in resolution_data first and only ENACTED (the law
// row created, appearing under Laws) at the end — the rules put the decree "into
// effect WITH the signatory's addendum", so the addendum (and, on a mar, the
// amendments) must be decided first. The sub-flow, after the roll:
//
//   1. make-choice ("pass the decree", preparer): records the outcome; no law.
//   2. Mar only: the non-preparer council members rewrite the body in turn
//      (lowest power first, each working from the previous output).
//   3. set-addendum (signatory): records the "and"/"but" rider — the step right
//      before enactment, so the preparer enacts with the final text in view.
//   4. enact-law (preparer): creates the law row and, on a make, the resource
//      asset (authored/named in the same call — no placeholder). Then the plan
//      auto-resolves (AutoCompleteAfterChoice) — no separate Complete step.
//
// The final law = amended body + an "and"/"but" connector + the signatory's
// (optional) rider text.
//
// Follow Scene: Your character interacting with a law.
//
// Extra routes:
//   POST /api/plans/:planId/start-debate    Preparer finalizes the text and opens the debate.
//   POST /api/plans/:planId/join-council    Lower-ranked player joins by leveraging one asset.
//   POST /api/plans/:planId/decline-council Lower-ranked player declines to join.
//   POST /api/plans/:planId/call-roll       Signatory closes the debate; creates dice roll.
//   POST /api/plans/:planId/amend-decree   (Mar) current amender rewrites the law body.
//   POST /api/plans/:planId/set-addendum   Signatory records the and/but addendum.
//   POST /api/plans/:planId/enact-law      Preparer enacts the law (+ named resource on Make); auto-resolves.
import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// decreeBankedDieSource tags the ephemeral banked dice a player mints by
// leveraging assets to join the council. They are spendable only on the
// decree's own roll (one open roll per game) and any left unspent are
// discarded when the law is enacted — see ApplyChoice.
const decreeBankedDieSource = "decree"

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

// PlanSceneParticipants: the auto-seated council (preparer + higher-power
// players + monarchOwner) — computed directly via pdAutoSeatCouncil rather
// than read back from resolution_data, since this runs before OnResolve
// persists it. Lower-ranked joiners are added later via
// pdJoinCouncilHandler's AddPlanSceneParticipant call.
func (pdHandler) PlanSceneParticipants(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) ([]int64, error) {
	return pdAutoSeatCouncil(ctx, q, plan)
}

// Resolution authority is intentionally shared but PREPARER-ANCHORED (decided
// in the 2026-05 rules audit): the signatory has real sway during resolution —
// they call the roll (call-roll) and write the addendum (set-addendum), both
// signatory-gated — and on a mar the other council members gate progress by
// amending the preparer's text (narrative, in the chat). But the
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
		// Signatory = the monarchOwner if a throne is established, else the
		// highest-power member (lowest rank number) — the rules' "the monarch
		// OR the player highest on the power track among those present".
		_, monarchOwnerID, ok, err := currentMonarch(ctx, deps.Q, plan.GameID)
		if err != nil {
			return nil, err
		}
		var sig int64
		if ok {
			sig = monarchOwnerID
		} else {
			sig, err = pdHighestPowerMember(ctx, deps.Q, plan.GameID, council)
			if err != nil {
				return nil, err
			}
		}
		pd.SignatoryID = &sig
	}

	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("could not initialise decree resolution data: %w", err)
	}

	// kickoffPlanResolution broadcasts plan.resolving *before* this hook runs, so
	// its payload carried the pre-resolve resolution_data (an empty council).
	// Clients watching the kickoff live — notably the preparer — would otherwise
	// keep that stale snapshot and show "No one has joined yet" with themselves
	// absent. Re-broadcast the seeded plan so the auto-seated council (preparer +
	// higher-power members + signatory) appears immediately, not just on refetch.
	if fresh, err := deps.Q.GetPlanByID(ctx, plan.ID); err == nil {
		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanResolving, model.PlanPayload{Plan: fresh})
	}

	// Return nil — the roll is created later when the signatory calls call-roll.
	return nil, nil
}

// pdAutoSeatCouncil returns the initial council: the preparer, every player
// ranked above them on the power track (higher-status peers attend
// automatically), and — when a throne is established — the monarchOwner
// regardless of power rank (ADR-007 §5: "The monarch and players above you on
// the power track may each be present"). Order is unspecified;
// signatory/amender ordering is computed from ranks separately.
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
	// The monarchOwner is auto-seated at any power rank (they may already be in
	// the council via the power-rank pass, so dedupe).
	_, monarchOwnerID, ok, err := currentMonarch(ctx, q, plan.GameID)
	if err != nil {
		return nil, fmt.Errorf("current monarch: %w", err)
	}
	if ok && !slices.Contains(council, monarchOwnerID) {
		council = append(council, monarchOwnerID)
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

// ApplyChoice records the roll's outcome and opens the law-writing sub-flow. It
// does NOT enact the law: per the rules the decree "goes into effect WITH the
// signatory's addendum", so the law row (and, on a make, the resource asset) are
// created later, when the addendum is placed (pdEnactLaw, called from
// set-addendum). On a mar this step computes the amendment order; the council
// then rewrites the body before the addendum is placed and the law takes effect.
func (pdHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	_ []string,
	result string,
) error {
	pd := resData.EnsureProposeDecree()

	// Idempotent: the outcome is applied automatically when the roll resolves
	// (AutoApplyChoiceOnRoll), so a later make-choice for the same plan — a stale
	// client, or a test driving the legacy path — must not re-run the effects
	// (recomputing the amendment order, re-clearing dice, duplicating the log).
	if pd.OutcomeApplied() {
		return nil
	}

	// The law body is the text the preparer finalized when opening the debate
	// (start-debate). Fall back to the preparation notes for any decree whose
	// debate predates that step. It becomes the working body the council may
	// amend (on a mar) and is written to the law row at enactment.
	body := pd.LawText
	if body == "" && plan.PreparationNotes != nil {
		body = *plan.PreparationNotes
	}
	pd.LawText = body
	pd.Outcome = result

	if result != makeOutcome {
		// Mar: the non-preparer council members amend the body in turn, lowest
		// power first. Compute that order now; they amend before the addendum.
		order, err := pdAmendmentOrder(ctx, deps.Q, plan, pd.SignatoryPlayerIDs)
		if err != nil {
			return err
		}
		pd.AmendmentOrder = order
	}

	// The council roll is over: discard any 'decree' banked dice the joiners
	// never spent so they cannot leak onto a later, unrelated roll. (Spent dice
	// are marked used and untouched.)
	if err := deps.Q.DeleteUnspentBankedDiceBySource(ctx, dbgen.DeleteUnspentBankedDiceBySourceParams{
		GameID: plan.GameID,
		Source: decreeBankedDieSource,
	}); err != nil {
		return fmt.Errorf("could not clear unspent council dice: %w", err)
	}

	// Non-acting clients refetch the plan to pick up the new sub-phase. The law
	// row doesn't exist yet, so there's no law.enacted event to send.
	broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied, model.PlanChoiceAppliedPayload{
		PlanID: plan.ID,
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
				"The decree passed. It takes effect once %s places the addendum.",
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
				"The decree passed but was marred — the council amends it (lowest power first), "+
					"then %s places the addendum, and only then does it take effect.",
				signatory,
			),
		)
	}

	return nil
}

// pdEnactLaw puts the decree into effect: it creates the law row with the final
// body and the signatory's composed addendum, and — on a make — creates the
// resource asset under the preparer-authored resourceName and its one required
// marginalia (one transaction, no placeholder). This is the LAST writing step,
// called from enact-law, so the law goes under Laws already carrying its
// addendum (and, on a mar, the council's amendments), as the rules require.
// Sets pd.LawID; the caller saves resolution_data. resourceName/resourceMarg
// are ignored on a mar (no asset).
func pdEnactLaw(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	resourceName, resourceMarg string,
) error {
	pd := resData.EnsureProposeDecree()

	laws, err := deps.Q.ListLaws(ctx, plan.GameID)
	if err != nil {
		return fmt.Errorf("could not list laws: %w", err)
	}
	displayOrder := int16(len(laws) + 1)

	planID := plan.ID
	law, err := deps.Q.CreateLaw(ctx, dbgen.CreateLawParams{
		GameID:       plan.GameID,
		Text:         pd.LawText,
		Addendum:     pdComposeAddendum(pd),
		OriginPlanID: &planID,
		SignatoryID:  pd.SignatoryID,
		DisplayOrder: displayOrder,
	})
	if err != nil {
		return fmt.Errorf("could not create law: %w", err)
	}
	pd.LawID = &law.ID

	if pd.Outcome == makeOutcome {
		if err = pdCreateLawAsset(ctx, deps, plan, resData, resourceName, resourceMarg); err != nil {
			return err
		}
	}

	broadcastEvent(deps.Manager, plan.GameID, model.EventLawEnacted, model.LawEnactedPayload{
		PlanID: plan.ID,
		Law:    law,
	})

	signatory := "the council"
	if pd.SignatoryID != nil {
		signatory = playerDisplayName(ctx, deps.Q, *pd.SignatoryID)
	}
	if pd.Outcome == makeOutcome {
		pdLog(ctx, deps, plan, model.SeverityImportant,
			fmt.Sprintf("The decree was enacted, signed by %s, and a new resource was created, with marginalia: %q.",
				signatory, resourceMarg))
	} else {
		pdLog(ctx, deps, plan, model.SeverityImportant,
			fmt.Sprintf("The amended decree was enacted, signed by %s.", signatory))
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

// pdCreateLawAsset creates the resource asset that accompanies a made law, under
// the preparer-authored name. Owner is the recipient determined by
// AssetRecipientForPlan — the preparer by default (the rule grants "what YOU
// gain" to the decree's proposer), or a keep_assets Make Demands winner if one
// has taken over. NOT the signatory: the signatory signs the law into being, but
// the worldly resource belongs to the player who drafted and resolved the decree.
//
// The name is authored by the preparer at enactment with the final law text in
// view (deliberately NOT derived from the law wording — the resource represents
// the law's worldly consequence the preparer narrates). Created with its one
// required marginalia in the same call, so the asset never exists unnamed or
// blank.
func pdCreateLawAsset(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	name, margText string,
) error {
	pd := resData.EnsureProposeDecree()

	ownerID, err := AssetRecipientForPlan(ctx, deps.Q, plan)
	if err != nil {
		return fmt.Errorf("resolve asset recipient: %w", err)
	}

	var asset dbgen.Asset
	var marginalia []dbgen.Marginalium
	err = deps.InTx(ctx, func(q *dbgen.Queries) error {
		var caErr error
		asset, marginalia, caErr = createAssetWithFirstMarginalia(ctx, q, dbgen.CreateAssetParams{
			GameID:    plan.GameID,
			OwnerID:   ownerID,
			CreatorID: plan.PreparerID,
			AssetType: model.AssetResource,
			Name:      name,
		}, margText)
		return caErr
	})
	if err != nil {
		return fmt.Errorf("could not create law resource asset: %w", err)
	}

	pd.ResourceAssetID = &asset.ID

	broadcastEvent(
		deps.Manager,
		plan.GameID,
		model.EventAssetCreated,
		model.AssetPayload{Asset: assetWithMarginalia{Asset: asset, Marginalia: marginalia}},
	)
	return nil
}

// CanComplete gates completion on the full law-writing sequence: the outcome
// must be applied (make-choice), on a mar every council amender must have taken
// their turn, the signatory must have placed their addendum (even if blank), and
// the preparer must have enacted the law (which creates the law row, so LawID is
// the enactment signal). Completion normally runs automatically right after
// enact-law (AutoCompleteAfterChoice).
func (pdHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	pd := resData.ProposeDecree
	if pd == nil || !pd.OutcomeApplied() {
		return errors.New("make-choice must be submitted before the plan can be completed")
	}
	if next := pd.NextAmender(); next != 0 {
		return errors.New("the council is still amending the law")
	}
	if !pd.AddendumPlaced {
		return errors.New("the signatory must place their addendum before completing")
	}
	if pd.LawID == nil {
		return errors.New("the preparer must enact the law before completing")
	}
	return nil
}

// ResolvingWaitees names the player the table is actually waiting on at each
// Propose Decree sub-phase, since the signatory and the amenders are usually
// NOT the preparer (auto-seating means a higher-power member almost always
// holds the pen). Without this override the generic case would mis-attribute
// every wait to the preparer.
//
//   - Pre-roll, no roll yet → the signatory (only they can call the roll;
//     lower-power joins are optional, so they don't block).
//   - Roll in progress / post-roll before make-choice → generic preparer case
//     (the preparer drives the roll and submits make-choice to pass the decree).
//   - Mar amendment chain incomplete → the next amender.
//   - Amendments done, addendum unplaced → the signatory.
//   - Addendum placed, law not yet enacted → generic preparer case (they enact,
//     which auto-completes the plan).
func (pdHandler) ResolvingWaitees(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	pd := loadResolutionData(plan.ResolutionData).ProposeDecree
	if pd == nil {
		return model.RowState{}, false
	}

	signatory := func() (model.RowState, bool) {
		if pd.SignatoryID == nil {
			return model.RowState{}, false
		}
		return model.RowState{
			Kind:            model.RowStatePlanResolving,
			ActingPlayerIDs: []int64{*pd.SignatoryID},
		}, true
	}

	// Until make-choice is submitted (the outcome applied), the blocker is the
	// signatory (call the roll) — unless the roll has already been called, in
	// which case the preparer owns the roll/enact (generic case).
	if !pd.OutcomeApplied() {
		if roll, err := q.GetDiceRollByPlanID(ctx, &plan.ID); err == nil && roll.ID != 0 {
			return model.RowState{}, false
		}
		// Pre-roll council meeting. Name everyone who can currently act on a gate
		// that blocks the roll, since several can act in parallel:
		//   - the preparer, until they finalize the text and open the debate;
		//   - every eligible player who still owes a join/decline decision (they
		//     may decide before or during the debate);
		//   - the signatory, but only once the debate is open AND all have decided
		//     (until then they cannot call the roll, so they're not yet actionable).
		var actors []int64
		if !pd.DebateStarted {
			actors = append(actors, plan.PreparerID)
		}
		if pending, err := pdPendingDeciders(ctx, q, plan, pd); err == nil {
			actors = append(actors, pending...)
		}
		if len(actors) > 0 {
			return model.RowState{
				Kind:            model.RowStatePlanResolving,
				ActingPlayerIDs: actors,
			}, true
		}
		return signatory()
	}

	// Outcome applied. On a mar the council amends in turn — name the next amender.
	if next := pd.NextAmender(); next != 0 {
		return model.RowState{
			Kind:            model.RowStatePlanResolving,
			ActingPlayerIDs: []int64{next},
		}, true
	}
	// Amendments done; the signatory must place the addendum.
	if !pd.AddendumPlaced {
		return signatory()
	}
	// Addendum placed: the preparer enacts the law (which auto-completes the
	// plan) — generic case names the preparer.
	return model.RowState{}, false
}

func (pdHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"start-debate":    pdStartDebateHandler(deps),
		"join-council":    pdJoinCouncilHandler(deps),
		"decline-council": pdDeclineCouncilHandler(deps),
		"call-roll":       pdCallRollHandler(deps),
		"amend-decree":    pdAmendDecreeHandler(deps),
		"skip-amend":      pdSkipAmendHandler(deps),
		"set-addendum":    pdSetAddendumHandler(deps),
		"enact-law":       pdEnactLawHandler(deps),
	}
}

// AutoCompleteAfterChoice opts Propose Decree into auto-completion: enact-law is
// the terminal action (it writes the law and, on a make, the named resource), so
// once CanComplete passes the plan resolves itself — no separate Complete click.
func (pdHandler) AutoCompleteAfterChoice(_ *dbgen.Plan, _ *ResolutionData) bool {
	return true
}

// AutoApplyChoiceOnRoll opts Propose Decree into recording the roll's outcome
// the instant the dice land: passing the decree is not a decision (the outcome
// is whatever the roll says), so finalizeRoll applies it via ApplyChoice instead
// of parking the row on a no-op "Pass the decree" gate. The real decisions — the
// council's amendments (mar), the signatory's addendum, the resource name —
// still follow in the law-writing sub-flow.
func (pdHandler) AutoApplyChoiceOnRoll() bool {
	return true
}

// pdComposeAddendum builds the law's addendum rider: "<connector> <text>" (e.g.
// "but salt is exempt"), or nil when the signatory left the rider blank.
func pdComposeAddendum(pd *ProposeDecreeResolutionData) *string {
	if !pd.AddendumPlaced || pd.Addendum == "" {
		return nil
	}
	rider := pd.Addendum
	if pd.AddendumConnector != "" {
		rider = pd.AddendumConnector + " " + pd.Addendum
	}
	return &rider
}
