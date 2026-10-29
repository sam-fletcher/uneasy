package handler

// handler/plan_spread_propaganda.go — Spread Propaganda plan handler (Phase 3b).
//
// Spread Propaganda (esteem, delay 3): The preparer spreads a message.
// Difficulty = preparer's rank on the esteem track.
//
// Make option: "create_artifact" — the preparer authors the societal-shift
// artifact (narrative + asset creation).
// Mar options (repeatable — the preparer picks (difficulty − result) total,
// possibly repeating a key):
//   (a) "give_peer"    — a peer leaves the preparer's retinue, given to
//                        another player (sub-flow, once per pick)
//   (b) "lay_low"      — esteem lockout: preparer's next plan cannot be an
//                        esteem plan (applied immediately; repeats no-op)
//   (c) "break_self"   — the preparer breaks one of their own assets
//                        (sub-flow, once per pick)
//   (d) "counter_prop" — top interferer spreads their own propaganda
//                        immediately (applied immediately; a repeat pick
//                        no-ops once the recursive plan exists)
//
// Esteem lockout (b): sets ResData.SpreadPropaganda.EsteemLockout = true.
// Checked in validatePlanPreparation for all esteem-category plan types.
//
// (a) and (c) are depletable — a retinue runs out of peers, a player runs out
// of breakable assets — so each committed pick that has no target left is
// discharged through sp-forfeit-step. Without that hatch CanComplete blocks on
// a pick nobody can satisfy and the row can never be completed.
//
// Recursive resolve (d): finds the top interferer by dice count (ties broken
// by best esteem rank), creates a new SP plan at current_row with status
// 'resolving', and creates a dice roll. Depth capped at 1.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanSpreadPropaganda, spHandler{})
}

type spHandler struct{}

// Mar option keys for Spread Propaganda. Defined as constants because each is
// referenced across ApplyChoice, CanComplete, the sub-flow routes and the
// forfeit hatch. Mirrors Spread Rumors' srOpt* block.
const (
	spOptGivePeer    = "give_peer"
	spOptLayLow      = "lay_low"
	spOptBreakSelf   = "break_self"
	spOptCounterProp = "counter_prop"
)

func (spHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryEsteem, Delay: 3}
}

func (spHandler) ValidatePreparation(_ context.Context, _ *ValidationContext) (*int16, string) {
	return nil, "" // no plan-specific prerequisites; fixed delay
}

func (spHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	preparerRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryEsteem)
	if err != nil {
		return 0, fmt.Errorf("could not determine preparer ranking: %w", err)
	}
	return gamepkg.SpreadPropagandaDifficulty(preparerRank), nil
}

// OnResolve opens the pre-roll narration step and returns nil — no roll yet. The
// plan sits in 'resolving' until the preparer summarises the message and how
// they have been spreading it, which sp-cast-roll posts for the table to read
// before it creates the roll (plan_preroll.go). No log entry here: the standard
// plan.resolving post marks the start and cast-roll posts the narration itself.
func (spHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)
	resData.EnsureSpreadPropaganda().PreRollDone = false
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("could not open pre-roll step: %w", err)
	}
	return nil, nil
}

// CheckChoice refuses a make/mar before the pre-roll narration has been cast, or
// while the roll it opened is still unsettled.
func (spHandler) CheckChoice(
	_ context.Context,
	_ *dbgen.Queries,
	_ *dbgen.Plan,
	_ *dbgen.Player,
	_ *ResolutionData,
	roll *dbgen.DiceRoll,
	_ string,
) error {
	if roll == nil {
		return errors.New("summarize your message and cast the dice before choosing options")
	}
	if roll.Outcome == nil {
		return errDiceNotSettled
	}
	return nil
}

// spCastRollHandler handles POST /api/plans/:planId/sp-cast-roll. The preparer
// "summarize[s] the message you want the public to take from your propaganda and
// describe[s] again how you've been spreading it" (book p.58). The text is
// recorded, posted as the preparer's own scene post, and only then is the roll
// created — so everyone choosing whether to help or interfere has read it.
//
// Request body: {"narration": "..."}
func spCastRollHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePreRollOpen(w, r, deps.Q, model.PlanSpreadPropaganda)
		if !ok {
			return
		}
		var body struct {
			Narration string `json:"narration"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		narration, ok := textField(w, "narration", body.Narration, maxLongTextLen)
		if !ok {
			return
		}
		if narration == "" {
			respondErr(w, http.StatusBadRequest,
				"summarize your message and describe how you've been spreading it before casting")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		difficulty, err := spHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
		if err != nil {
			respondInternalErr(w, r, "could not compute difficulty", err)
			return
		}
		// Post BEFORE the roll exists: the narration is what the table reads to decide
		// whether to help or interfere, so it must be on the record first.
		postPlanNarration(ctx, deps, plan, player.ID, narration)
		roll, err := openPreRollRoll(ctx, deps, plan, difficulty)
		if err != nil {
			respondInternalErr(w, r, "could not create dice roll", err)
			return
		}
		sp := resData.EnsureSpreadPropaganda()
		sp.PreRollDone = true
		sp.PreRollText = narration
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not close pre-roll step", err)
			return
		}

		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "roll": roll})
	}
}

// ApplyChoice resolves a Spread Propaganda make/mar.
//
// Make: opens the artifact-authoring sub-flow (the rules' only make effect;
// "create_artifact" is the single make option in the UI). The artifact itself is
// created — and named — later by the preparer via create-artifact, in one
// transaction (no placeholder); see applySpreadPropagandaMake.
//
// Mar (option keys match frontend MAR_OPTIONS.spread_propaganda):
//   - "lay_low"      → esteem lockout (preparer's next plan cannot be esteem)
//   - "break_self"   → preparer breaks one of their own assets (extra route)
//   - "give_peer"    → a peer leaves the preparer's retinue (extra route)
//   - "counter_prop" → the top interferer spreads their own propaganda now
func (spHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	choices []string,
	result string,
) error {
	if result == makeOutcome {
		return applySpreadPropagandaMake(ctx, deps, plan, resData)
	}

	// Check co-opt can fire BEFORE anything is applied. A later pick in the same
	// submission failing would otherwise leave the earlier ones half-applied —
	// lay_low has already posted its log line by the time counter_prop errors —
	// and the picker greys counter_prop out in exactly this case, so reaching
	// here means a stale client.
	if slices.Contains(choices, spOptCounterProp) && !spCoOptAlreadyFired(resData) {
		_, reason, err := spCoOptInterferers(ctx, deps.Q, plan, resData)
		if err != nil {
			return err
		}
		if reason != "" {
			return errors.New(reason)
		}
	}

	for _, choice := range choices {
		switch choice {
		case spOptLayLow:
			// Repeatable in the choice list, but the effect is a single flag —
			// a second pick is a harmless no-op.
			resData.EnsureSpreadPropaganda().EsteemLockout = true
			spLog(ctx, deps, plan, model.SeverityDefault,
				fmt.Sprintf("%s must keep their head down — their next plan cannot involve esteem.",
					playerDisplayName(ctx, deps.Q, plan.PreparerID)))

		case spOptGivePeer, spOptBreakSelf:
			// Asset pickers — the actual transfer/break happens via the
			// give-peer / break-self routes, once per pick. How many are owed
			// comes from pickedChoiceCount, not an immediate action here.

		case spOptCounterProp:
			// Fires its side effect immediately (unlike give_peer/break_self,
			// which defer to a sub-flow), so a duplicate pick in the same
			// submission must no-op rather than spawn a second recursive plan.
			if spCoOptAlreadyFired(resData) {
				continue
			}
			if err := applyCoOpt(ctx, deps, plan, resData); err != nil {
				return err
			}
		}
	}
	return nil
}

// applySpreadPropagandaMake opens the artifact-authoring sub-flow the make step
// mandates. It does NOT create the artifact: the rules call for the preparer to
// author it ("Create an artifact representing the societal shift"), so the asset
// is created — and named — in one transaction by create-artifact (spCreateArtifact),
// never as a placeholder. Here we only flag that the artifact is owed; CanComplete
// gates the plan until it exists. Idempotent.
func applySpreadPropagandaMake(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
) error {
	sp := resData.EnsureSpreadPropaganda()
	if sp.ArtifactID != nil {
		return nil // already authored (idempotent re-apply)
	}
	sp.ArtifactRequired = true

	spLog(ctx, deps, plan, model.SeverityDefault,
		fmt.Sprintf("%s reshaped society — they must now author the artifact embodying the shift.",
			playerDisplayName(ctx, deps.Q, plan.PreparerID)))
	return nil
}

// CanComplete blocks completion until the made artifact has been authored and
// every committed give_peer / break_self pick has been performed — or
// discharged through sp-forfeit-step, the hatch for a pick whose last target is
// gone. The mar is repeatable, so "how many are owed" is server-authoritative
// from the committed choices (pickedChoiceCount), not a boolean flag — see
// subflowPicksRemaining.
func (spHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	// Unlike a plain nil-check, EnsureSpreadPropaganda() also covers the case
	// where give_peer/break_self were picked but ApplyChoice never allocated
	// SpreadPropaganda (it has no immediate effect to record — the sub-flow
	// routes are the only writers) — subflowPicksRemaining below still needs
	// to run in that case.
	sp := resData.EnsureSpreadPropaganda()
	if sp.ArtifactRequired && sp.ArtifactID == nil {
		return errors.New("preparer must author the societal-shift artifact (POST /plans/{planId}/create-artifact)")
	}
	return subflowPicksRemaining(resData,
		subflowProgress{spOptGivePeer, "give-peer", sp.GivePeerDone},
		subflowProgress{spOptBreakSelf, "break-self", sp.BreakSelfDone},
	)
}

// ResolvingWaitees names a Make Demands perform_steps winner while the committed
// make/mar still owes them work. The winner stands in for the preparer on every
// post-commit step — the artifact, the peer handed over, the asset broken, the
// forfeit — and the server refuses the preparer all of them (requireResolutionActor),
// so the generic plan_resolving case, which names the preparer, would point the
// table at the one player who cannot act.
//
// The pre-commit window is covered by pendingPerformStepsChooser, and once
// nothing is owed the preparer completes (completion stays theirs), so both fall
// through to the generic case.
func (spHandler) ResolvingWaitees(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	winner := performStepsWinner(ctx, q, plan)
	if winner == 0 || winner == plan.PreparerID {
		return model.RowState{}, false
	}
	resData := loadResolutionData(plan.ResolutionData)
	if len(resData.MakeMarChoices) == 0 {
		return model.RowState{}, false
	}
	if err := (spHandler{}).CanComplete(plan, &resData); err == nil {
		return model.RowState{}, false
	}
	return model.RowState{Kind: model.RowStatePlanResolving, ActingPlayerIDs: []int64{winner}}, true
}

func (spHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"sp-cast-roll":    spCastRollHandler(deps),
		"give-peer":       spGivePeerHandler(deps),
		"break-self":      spBreakSelfHandler(deps),
		"create-artifact": spCreateArtifactHandler(deps),
		"sp-forfeit-step": spForfeitStepHandler(deps),
	}
}

// spCreateArtifactHandler handles POST /api/plans/:planId/create-artifact.
//
// On a made plan the preparer "describe[s] the changes that come from this, and
// create[s] an artifact that embodies the shift in society" (book p.58). Both
// halves are one step — no extra gate (principle 3): the narration of the changes
// rides in the same request as the artifact and is posted as the author's own
// scene post, so it is on the record whether or not anyone reads the artifact.
//
// The artifact is created under the chosen name in a single transaction (no placeholder)
// and routes ownership through AssetRecipientForPlan, so a resolved Make Demands
// keep_assets winner claims it. Preparer-gated and required — it gates completion
// (CanComplete) until the artifact exists. Mirrors Propose Decree's enact-law.
//
// Request body: {"narration": "the changes society undergoes",
//
//	"name": "the artifact's name", "marginalia": ["one entry"]}
func spCreateArtifactHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanSpreadPropaganda)
		if !ok {
			return
		}
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
			return
		}

		var body struct {
			Narration  string   `json:"narration"`
			Name       string   `json:"name"`
			Marginalia []string `json:"marginalia"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		narration, ok := textField(w, "narration", body.Narration, maxLongTextLen)
		if !ok {
			return
		}
		if narration == "" {
			respondErr(
				w,
				http.StatusBadRequest,
				"describe the changes that come from this before authoring the artifact",
			)
			return
		}
		name, ok := textField(w, "name", body.Name, maxAssetNameLen)
		if !ok {
			return
		}
		if name == "" {
			respondErr(w, http.StatusBadRequest, "name is required to author the artifact")
			return
		}
		margText, err := requireOneMarginalia(body.Marginalia)
		if err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		sp := resData.EnsureSpreadPropaganda()
		if !sp.ArtifactRequired {
			respondErr(w, http.StatusConflict, "this plan does not call for an artifact")
			return
		}
		if sp.ArtifactID != nil {
			respondErr(w, http.StatusConflict, "the artifact has already been authored")
			return
		}

		recipient, err := AssetRecipientForPlan(ctx, deps.Q, plan)
		if err != nil {
			respondInternalErr(w, r, "could not resolve asset recipient", err)
			return
		}

		// Created already named (deliberately NOT derived from the propaganda
		// message — the artifact represents the societal shift the preparer
		// narrates, not a copy of their talking points).
		var asset dbgen.Asset
		var marginalia []dbgen.Marginalium
		err = deps.InTx(ctx, func(q *dbgen.Queries) error {
			var caErr error
			asset, marginalia, caErr = createAssetWithFirstMarginalia(ctx, q, dbgen.CreateAssetParams{
				GameID:    plan.GameID,
				OwnerID:   recipient,
				CreatorID: plan.PreparerID,
				AssetType: model.AssetArtifact,
				Name:      name,
			}, margText)
			return caErr
		})
		if err != nil {
			respondInternalErr(w, r, "could not create societal-shift artifact", err)
			return
		}
		sp.ArtifactID = &asset.ID
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record the artifact", err)
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, model.EventAssetCreated,
			model.AssetPayload{Asset: assetWithMarginalia{Asset: asset, Marginalia: marginalia}})
		postPlanNarration(ctx, deps, plan, player.ID, "The changes society undergoes: "+narration)
		spLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s authored a new artifact, %s, embodying the societal shift, with marginalia: %q.",
				playerDisplayName(ctx, deps.Q, plan.PreparerID), assetMark(asset.Name), margText))
		// make-choice's panel watches for the artifact to appear; nudge non-actors.
		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied, model.PlanChoiceAppliedPayload{
			PlanID: plan.ID,
		})

		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "asset": asset})
	}
}

// MaxChoices: make creates exactly one artifact; the mar list is chosen equal
// to (difficulty − result).
func (spHandler) MaxChoices(result string, rollResult, difficulty int16) int {
	if result == makeOutcome {
		return 1
	}
	return int(difficulty - rollResult)
}

// spLog emits a Spread Propaganda action-log entry anchored to the plan's row.
func spLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.spread_propaganda",
		severity, body, plan.RowNumber, &plan.ID, nil,
		map[string]any{"plan_id": plan.ID})
}

// ── Co-opt (recursive propaganda) ────────────────────────────────────────────

// spCoOptAlreadyFired reports whether this plan already spawned its recursive
// propaganda plan — the idempotence guard for a duplicate counter_prop pick.
func spCoOptAlreadyFired(resData *ResolutionData) bool {
	return resData.SpreadPropaganda != nil && resData.SpreadPropaganda.RecursivePlanID != nil
}

// spCoOptInterferers is the single source of truth for whether mar option (d)
// can fire. It returns the roll's interferers (ordered top first) when it can,
// or a user-facing reason when it cannot — a recursive plan (depth is capped at
// 1) or a roll nobody interfered with, in which case there is no one to spread
// propaganda of their own. err is for genuine failures only.
//
// The picker mirrors this to grey the option out with the same reason
// (SpreadPropagandaPanel.svelte coOptBlocker); keep the two in step.
func spCoOptInterferers(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	resData *ResolutionData,
) (interferers []dbgen.ListInterferenceDiceByRollRow, reason string, err error) {
	if sp := resData.SpreadPropaganda; sp != nil && sp.OriginalPlanID != nil {
		return nil, "Counter-propaganda can't be chosen on a plan that is itself counter-propaganda.", nil
	}
	roll, err := q.GetDiceRollByPlanID(ctx, &plan.ID)
	if err != nil {
		return nil, "", fmt.Errorf("could not find dice roll for plan: %w", err)
	}
	interferers, err = q.ListInterferenceDiceByRoll(ctx, roll.ID)
	if err != nil {
		return nil, "", fmt.Errorf("could not list interference dice: %w", err)
	}
	if len(interferers) == 0 {
		return nil, "Nobody interfered with this roll, so there is no one to spread propaganda of their own.", nil
	}
	return interferers, "", nil
}

// applyCoOpt implements SP mar option (d): the top interferer spreads their
// own propaganda at the current row, resolving immediately.
func applyCoOpt(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
) error {
	interferers, reason, err := spCoOptInterferers(ctx, deps.Q, plan, resData)
	if err != nil {
		return err
	}
	if reason != "" {
		return errors.New(reason)
	}

	topCount := interferers[0].DiceCount
	topPlayerID, err := pickBestEsteemRanked(ctx, deps.Q, plan.GameID, interferers, topCount)
	if err != nil {
		return fmt.Errorf("could not determine top interferer: %w", err)
	}

	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return fmt.Errorf("could not load game: %w", err)
	}

	count, err := deps.Q.CountPlansOnRow(ctx, dbgen.CountPlansOnRowParams{
		GameID:    game.ID,
		RowNumber: new(game.CurrentRow),
	})
	if err != nil {
		count = 0
	}

	// Create the recursive SP plan.
	recursivePlan, err := deps.Q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:           game.ID,
		PlanType:         model.PlanSpreadPropaganda,
		Category:         model.CategoryEsteem,
		PreparerID:       topPlayerID,
		TargetPlayerID:   nil,
		TargetAssetID:    nil,
		RowNumber:        new(game.CurrentRow),
		RowOrder:         int16(count),
		PreparedAtRow:    game.CurrentRow,
		PreparationNotes: nil,
	})
	if err != nil {
		return fmt.Errorf("could not create recursive propaganda plan: %w", err)
	}

	// Mark it as resolving immediately (skips the pending phase).
	err = deps.Q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID:     recursivePlan.ID,
		Status: model.PlanResolving,
	})
	if err != nil {
		return fmt.Errorf("could not mark recursive plan as resolving: %w", err)
	}

	// Tag it in ResData so its own co-opt option is blocked.
	parentID := plan.ID
	recursiveResData := ResolutionData{
		// Born rolling (just below), so it is past its pre-roll step.
		SpreadPropaganda: &SpreadPropagandaResolutionData{OriginalPlanID: &parentID, PreRollDone: true},
	}
	if err = saveResolutionData(ctx, deps.Q, recursivePlan.ID, recursiveResData); err != nil {
		return fmt.Errorf("could not save recursive plan data: %w", err)
	}

	// Compute difficulty for the recursive plan (top interferer's esteem rank).
	difficulty, err := spHandler{}.ComputeDifficulty(ctx, deps.Q, &recursivePlan, &recursiveResData)
	if err != nil {
		return fmt.Errorf("could not compute recursive plan difficulty: %w", err)
	}

	// Create the dice roll. The normal leverage window then opens.
	if _, err = createPlanRoll(ctx, deps.Q, deps.Manager, &game, &recursivePlan, difficulty, topPlayerID); err != nil {
		return fmt.Errorf("could not create recursive dice roll: %w", err)
	}

	// Record the recursive plan ID in the parent's ResData.
	resData.EnsureSpreadPropaganda().RecursivePlanID = &recursivePlan.ID

	broadcastEvent(deps.Manager, game.ID, model.EventSPRecursivePlan, model.SPRecursivePlanPayload{
		ParentPlanID:    plan.ID,
		RecursivePlanID: recursivePlan.ID,
		PreparerID:      topPlayerID,
	})
	spLog(ctx, deps, plan, model.SeverityImportant,
		fmt.Sprintf("%s seized the moment and spread propaganda of their own — it resolves now.",
			playerDisplayName(ctx, deps.Q, topPlayerID)))

	return nil
}

// pickBestEsteemRanked selects the player with the lowest esteem rank
// (= highest status) among those tied at topCount interference dice.
func pickBestEsteemRanked(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	interferers []dbgen.ListInterferenceDiceByRollRow,
	topCount int64,
) (int64, error) {
	bestRank := int16(999)
	bestPlayerID := int64(0)

	for _, row := range interferers {
		if row.DiceCount < topCount {
			break // rows are ordered by dice_count DESC
		}
		rank, err := playerRankInCategory(ctx, q, gameID, row.PlayerID, model.CategoryEsteem)
		if err != nil {
			continue
		}
		if rank < bestRank {
			bestRank = rank
			bestPlayerID = row.PlayerID
		}
	}

	if bestPlayerID == 0 {
		if len(interferers) > 0 {
			return interferers[0].PlayerID, nil // fallback
		}
		return 0, errors.New("no interferers found")
	}
	return bestPlayerID, nil
}

// hasEsteemLockout lives in handler/eligibility.go (it queries Postgres).

// ── Give Peer (mar option a) ──────────────────────────────────────────────────

// spGivePeerHandler handles POST /api/plans/:planId/give-peer.
//
// Mar option (a): a peer leaves the preparer's retinue and is handed to
// another player. Only the preparer may call it, only after "give_peer" was
// chosen. Request body: {"peer_asset_id": A, "to_player_id": P}
func spGivePeerHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanSpreadPropaganda {
			respondErr(w, http.StatusBadRequest, "give-peer is only for Spread Propaganda")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
			return
		}

		var body struct {
			PeerAssetID int64 `json:"peer_asset_id"`
			ToPlayerID  int64 `json:"to_player_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PeerAssetID == 0 || body.ToPlayerID == 0 {
			respondErr(w, http.StatusBadRequest, "peer_asset_id and to_player_id are required")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		sp := resData.EnsureSpreadPropaganda()
		// Server-authoritative completion: a stale client (re-prompted after a
		// refresh) must not give away more peers than were picked.
		if sp.GivePeerDone >= pickedChoiceCount(&resData, spOptGivePeer) {
			respondErr(w, http.StatusConflict, "give-peer already completed for this plan")
			return
		}

		asset, err := deps.Q.GetAssetByID(ctx, body.PeerAssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "peer asset not found")
			return
		}
		if asset.GameID != plan.GameID || asset.OwnerID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "peer must be one of your own assets")
			return
		}
		if asset.AssetType != model.AssetPeer {
			respondErr(w, http.StatusBadRequest, "asset must be a peer")
			return
		}
		// A destroyed peer has already left the retinue, so it is not a target
		// this pick can be spent on — spEligibleGivePeers excludes it too, and
		// the two must agree or the forfeit hatch and the route disagree about
		// whether a target remains.
		if asset.IsDestroyed {
			respondErr(w, http.StatusConflict, "that peer is destroyed")
			return
		}
		if body.ToPlayerID == plan.PreparerID {
			respondErr(w, http.StatusBadRequest, "give the peer to another player, not yourself")
			return
		}
		recipient, err := deps.Q.GetPlayerByID(ctx, body.ToPlayerID)
		if err != nil || recipient.GameID != plan.GameID {
			respondErr(w, http.StatusBadRequest, "recipient must be a player at this table")
			return
		}

		if _, err := takeAssetEffect(
			ctx,
			deps.Q,
			deps.Manager,
			plan.GameID,
			asset.ID,
			plan.PreparerID,
			body.ToPlayerID,
		); err != nil {
			respondInternalErr(w, r, "could not transfer peer", err)
			return
		}

		sp.GivePeerDone++
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record give-peer", err)
			return
		}

		spLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s lost the peer %s to %s.",
				playerDisplayName(ctx, deps.Q, plan.PreparerID), assetMark(asset.Name),
				playerDisplayName(ctx, deps.Q, body.ToPlayerID)))

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"peer_asset_id": asset.ID,
			"to_player_id":  body.ToPlayerID,
		})
	}
}

// ── Break Self (mar option c) ─────────────────────────────────────────────────

// spBreakSelfHandler handles POST /api/plans/:planId/break-self.
//
// Mar option (c): "word of your laughable ideas gets around — break yourself."
// The preparer breaks one of their own assets: tearing one marginalia, or — on
// a blank asset — destroying it outright. Only the resolution actor may call
// it, only after "break_self" was chosen.
//
// Request body: {"asset_id": A, "marginalia_id": M}
//
// marginalia_id may be omitted (or 0) when the asset is blank; asset_id may be
// omitted when a marginalia_id is given, which names its asset anyway.
func spBreakSelfHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanSpreadPropaganda {
			respondErr(w, http.StatusBadRequest, "break-self is only for Spread Propaganda")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
			return
		}

		var body struct {
			AssetID      int64 `json:"asset_id"`
			MarginaliaID int64 `json:"marginalia_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
			(body.AssetID == 0 && body.MarginaliaID == 0) {
			respondErr(w, http.StatusBadRequest, "asset_id is required")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		sp := resData.EnsureSpreadPropaganda()
		// Server-authoritative completion: a stale client (re-prompted after a
		// refresh) must not break more assets than were picked.
		if sp.BreakSelfDone >= pickedChoiceCount(&resData, spOptBreakSelf) {
			respondErr(w, http.StatusConflict, "break-self already completed for this plan")
			return
		}

		// A client that sent only a marginalia_id (the route's original shape)
		// still names its asset unambiguously, so derive it rather than 400.
		if body.AssetID == 0 {
			m, mErr := deps.Q.GetMarginaliaByID(ctx, body.MarginaliaID)
			if mErr != nil {
				respondErr(w, http.StatusNotFound, "marginalia not found")
				return
			}
			body.AssetID = m.AssetID
		}

		asset, err := deps.Q.GetAssetByID(ctx, body.AssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}
		if asset.GameID != plan.GameID || asset.OwnerID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "you must break one of your own assets")
			return
		}
		if asset.IsDestroyed {
			respondErr(w, http.StatusConflict, "asset is already destroyed")
			return
		}

		// nil m = a blank asset: there is nothing to tear, so the break destroys
		// it outright (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md D3). Blank assets
		// are breakable by design, so the picker offers them and this route must
		// accept one with no marginalia_id.
		m, err := resolveBreakTarget(ctx, deps.Q, &asset, body.MarginaliaID)
		if err != nil {
			respondHTTPErr(w, r, err)
			return
		}

		destroyed, err := breakAsset(ctx, deps.Q, deps.Manager, &asset, m, player.ID)
		if err != nil {
			respondInternalErr(w, r, "could not break asset", err)
			return
		}

		sp.BreakSelfDone++
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record break-self", err)
			return
		}

		spLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s broke their own %s amid the backlash.%s",
				playerDisplayName(ctx, deps.Q, plan.PreparerID), assetMark(asset.Name),
				brokenAssetDetail(ctx, deps.Q, asset.OwnerID, m, destroyed)))

		var tornMargID *int64
		if m != nil {
			tornMargID = &m.ID
		}
		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"marginalia_id": tornMargID,
			"asset_id":      asset.ID,
			"destroyed":     destroyed,
		})
	}
}

// ── Forfeit Step (escape hatch for a depletable mar pick) ─────────────────────

// spForfeitStepHandler handles POST /api/plans/:planId/sp-forfeit-step.
//
// Discharges the remaining picks of a depletable mar step as a no-op when no
// valid target is left: give_peer with the preparer's retinue emptied, or
// break_self with nothing of theirs still breakable. Both are repeatable, so
// the mar budget can be spent on more of them than the preparer has targets
// for — and then CanComplete blocks forever on picks that can never be
// satisfied, leaving the panel a dead disabled button and the row
// uncompletable by anyone at the table. The picker now caps each option at its
// live target count, so an over-pick needs a stale client or a target lost
// between commit and resolution; this is the hatch for both.
//
// Mirrors Seek Answers' seek-forfeit-step and Spread Rumors' sr-forfeit-step,
// including the server-side re-verification: NO eligible target may exist
// before anything is discharged, so a stale or hand-rolled client cannot
// forfeit its way out of an obligation it could still meet. Nothing happens to
// any asset; the forfeit is logged for transparency.
//
// Request body: {"step": "give_peer" | "break_self"}
func spForfeitStepHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanSpreadPropaganda)
		if !ok {
			return
		}
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
			return
		}
		var body struct {
			Step string `json:"step"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		sp := resData.EnsureSpreadPropaganda()

		var remaining, eligible int
		var noun string
		var err error
		switch body.Step {
		case spOptGivePeer:
			remaining = pickedChoiceCount(&resData, spOptGivePeer) - sp.GivePeerDone
			eligible, err = spEligibleGivePeers(ctx, deps, plan)
			noun = "peer to give up"
		case spOptBreakSelf:
			remaining = pickedChoiceCount(&resData, spOptBreakSelf) - sp.BreakSelfDone
			eligible, err = spEligibleSelfBreaks(ctx, deps, plan)
			noun = "asset of their own to break"
		default:
			respondErr(w, http.StatusBadRequest, "step must be give_peer or break_self")
			return
		}
		if err != nil {
			respondInternalErr(w, r, "could not count eligible targets", err)
			return
		}
		if remaining <= 0 {
			respondErr(w, http.StatusConflict, "no remaining picks to forfeit for this step")
			return
		}
		if eligible > 0 {
			respondErr(w, http.StatusConflict, "valid targets remain — complete the step instead of forfeiting")
			return
		}

		switch body.Step {
		case spOptGivePeer:
			sp.GivePeerDone += remaining
		case spOptBreakSelf:
			sp.BreakSelfDone += remaining
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record forfeit", err)
			return
		}

		// The count stays unit-less ("2 forfeited"): Seek Answers' and Spread Rumors'
		// forfeit logs each spell out pick/picks, and a third copy of those literals
		// trips goconst package-wide.
		spLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
			"%s had no eligible %s — %d forfeited.",
			playerDisplayName(ctx, deps.Q, plan.PreparerID), noun, remaining))

		// The forfeit advances a done-counter inside resolution_data; nudge
		// non-actors to refetch the plan, since broadcastRowState alone won't
		// refresh resolution_data on their clients.
		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied,
			model.PlanChoiceAppliedPayload{PlanID: plan.ID})
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "step": body.Step, "forfeited": remaining})
	}
}

// spEligibleGivePeers counts the live peers still in the preparer's retinue —
// the targets a give_peer pick can be spent on. There is always another player
// to hand one to (a table seats at least two), so the retinue is the whole
// constraint. Mirror of the frontend preparerPeers picker.
func spEligibleGivePeers(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (int, error) {
	assets, err := deps.Q.ListAssetsByOwner(ctx, plan.PreparerID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, a := range assets {
		if a.GameID == plan.GameID && a.AssetType == model.AssetPeer && !a.IsDestroyed {
			count++
		}
	}
	return count, nil
}

// spEligibleSelfBreaks counts the preparer's own assets a break_self pick could
// still land on. A blank asset counts: the break destroys it outright
// (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md D3), which is why this goes through
// assetIsBreakable rather than an intact-marginalia test.
//
// It counts ASSETS, not available breaks — an asset with three intact
// marginalia absorbs three picks. That is enough here, because the count is
// only ever used as "is it zero?"; the picker's own cap (shared.ts
// breaksAvailable) is what has to be per-break exact.
func spEligibleSelfBreaks(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (int, error) {
	assets, err := deps.Q.ListAssetsByOwner(ctx, plan.PreparerID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, a := range assets {
		if a.GameID != plan.GameID || a.IsDestroyed {
			continue
		}
		breakable, err := assetIsBreakable(ctx, deps.Q, a.ID)
		if err != nil {
			return 0, err
		}
		if breakable {
			count++
		}
	}
	return count, nil
}
