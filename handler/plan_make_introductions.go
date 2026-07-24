package handler

// handler/plan_make_introductions.go — Make Introductions plan handler (Phase 3b).
//
// Make Introductions (knowledge, delay 3): The preparer brings 1–4 new peers
// into the game. Difficulty = 2 + peer_count.
//
// Peers named here are DRAFTS, not assets, until they turn up: nothing exists in
// any retinue until arrival, and arrival is creation (game.DraftPeer, and
// adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md D4). That is what keeps a peer who is
// still on the road — or who never arrives at all — off the table, where they
// used to be breakable, takeable, leverageable and stakeable for up to six rows.
//
// Pre-roll flow: the preparer names each peer one at a time via
// POST /api/plans/:planId/create-peer, which records a draft in
// resolution_data.make_introductions.drafts. Names only, exactly as printed (D5).
// Once peer_count drafts exist, POST /api/plans/:planId/finalize-peers creates
// the dice roll and resolution proceeds normally.
//
// Make: the peers arrive successfully, and the rule's "add marginalia to each"
// is paid one draft at a time via POST /api/plans/:planId/introductions-arrival —
// the arrival form. Each arrival materializes the draft into the plan's asset
// recipient's retinue (AssetRecipientForPlan, so a resolved Make Demands
// keep_assets winner claims them). Completion is gated until every draft has
// arrived.
//
// Mar (per-peer): on a mar the preparer resolves EACH introduced peer with one
// of four outcomes via POST /api/plans/:planId/introductions-mar:
//   - other_retinue  → the peer joins another player's retinue, and THAT player
//     writes their marginalia (D6) via introductions-marginalia, which is also
//     when they materialize
//   - broken_arrival → another player authors the peer's marginalia (again via
//     introductions-marginalia), then the peer materializes into the recipient's
//     retinue
//   - delayed        → the draft rides to a row d6 ahead on a synthetic plan and
//     materializes there; if that row exceeds the public record the draft is
//     dropped and no asset ever exists
//   - broken_journey → the preparer writes the peer's marginalia AND the mark of
//     the journey; the peer arrives carrying both, with the journey mark torn
//
// A synthetic delayed-arrival plan resolves on its row later: OnResolve returns
// no roll, the travelling draft arrives through the same arrival route, and
// submitting that form closes the plan — it never rolled, so there is no
// outcome for CompletePlan to read.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanMakeIntroductions, miHandler{})
}

type miHandler struct{}

func (miHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryKnowledge, Delay: 3}
}

func (miHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (*int16, string) {
	if v.PeerCount < 1 || v.PeerCount > 4 {
		return nil, "make_introductions requires peer_count between 1 and 4"
	}
	return nil, "" // fixed delay; target row computed from Metadata().Delay
}

func (miHandler) ComputeDifficulty(
	_ context.Context,
	_ *dbgen.Queries,
	_ *dbgen.Plan,
	resData *ResolutionData,
) (int16, error) {
	return gamepkg.MakeIntroductionsDifficulty(*resData), nil
}

// OnResolve defers the dice roll until the preparer has named each of
// the peer_count peers via /create-peer and called /finalize-peers. That
// matches the rule's "pre-roll: create new peer assets with names only"
// step. Synthetic delayed-arrival plans skip the roll entirely.
func (miHandler) OnResolve(_ context.Context, _ *PlanDeps, _ *dbgen.Plan) (*dbgen.DiceRoll, error) {
	return nil, nil
}

// CanComplete holds the plan until every peer it introduced has landed
// somewhere definite:
//
//   - a synthetic delayed-arrival plan, until its travelling peer has arrived;
//   - a made plan, until every draft has been through the arrival form (the
//     rule's "add marginalia to each");
//   - a marred plan, until every draft has a resolved per-peer outcome, and any
//     peer whose marginalia another player owes has had it written.
func (miHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	mi := resData.MakeIntroductions
	if mi == nil {
		return nil
	}
	if mi.DelayedArrival {
		if d := mi.DelayedDraft; d != nil && !mi.HasArrived(d.ID) {
			return fmt.Errorf("%s has not arrived yet", d.Name)
		}
		return nil
	}
	if mi.MakePending {
		for _, d := range mi.Drafts {
			if !mi.HasArrived(d.ID) {
				return fmt.Errorf("describe %s before completing — every newcomer needs a marginalia", d.Name)
			}
		}
		return nil
	}
	if !mi.MarPending {
		return nil
	}
	if int16(len(mi.MarOutcomes)) < mi.PeerCount {
		return fmt.Errorf("resolve all %d introduced peers before completing (%d resolved)",
			mi.PeerCount, len(mi.MarOutcomes))
	}
	for _, o := range mi.MarOutcomes {
		if !o.Done {
			return errors.New("a newcomer is still waiting for another player to write their marginalia")
		}
	}
	return nil
}

// ResolvingWaitees hands the waiting bar the players who owe a mar peer's
// marginalia — the receiving player of an "other_retinue" peer, the assigned
// author of a "broken_arrival" one. Neither is the preparer, and until they
// write, no asset exists and the plan cannot complete. Nothing outstanding →
// ride the generic PlanResolving case, which names the preparer.
func (miHandler) ResolvingWaitees(_ context.Context, _ *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	mi := loadResolutionData(plan.ResolutionData).MakeIntroductions
	if mi == nil {
		return model.RowState{}, false
	}
	authors := mi.PendingArrivalAuthors()
	if len(authors) == 0 {
		return model.RowState{}, false
	}
	return model.RowState{
		Kind:            model.RowStateAwaitIntroductionsMarginalia,
		ActingPlayerIDs: authors,
	}, true
}

func (miHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"create-peer":              createPeerHandler(deps),
		"finalize-peers":           finalizePeersHandler(deps),
		"introductions-arrival":    introductionsArrivalHandler(deps),
		"introductions-mar":        introductionsMarHandler(deps),
		"introductions-marginalia": introductionsMarginaliaHandler(deps),
	}
}

func (miHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	_ []string,
	result string,
) error {
	if result == makeOutcome {
		// The pre-roll named the peers but created nothing — they are drafts
		// until they arrive (D4). The make step is where the rule's "add
		// marginalia to each" is paid, one arrival form per draft, gated by
		// CanComplete.
		resData.EnsureMakeIntroductions().MakePending = true
		miLog(ctx, deps, plan, model.SeverityImportant,
			"The new peers arrived at court — each must now be described.")
		return nil
	}
	// Mar: the preparer resolves each introduced peer individually via the
	// introductions-mar route. Flag it so completion is gated until all done.
	resData.EnsureMakeIntroductions().MarPending = true
	return nil
}

// miLog emits a Make Introductions action-log entry anchored to the plan row.
func miLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.make_introductions",
		severity, body, plan.RowNumber, &plan.ID, nil,
		map[string]any{"plan_id": plan.ID})
}

// miStoreResData stores peer_count in resolution_data during plan preparation.
func miStoreResData(ctx context.Context, q *dbgen.Queries, planID int64, peerCount int16) error {
	d := ResolutionData{
		MakeIntroductions: &MakeIntroductionsResolutionData{PeerCount: peerCount},
	}
	return saveResolutionData(ctx, q, planID, d)
}

// ── Pre-roll peer naming extra routes ────────────────────────────────────────

// createPeerHandler handles POST /api/plans/:planId/create-peer.
//
// Called once per peer during the pre-roll naming step. The preparer submits a
// name — names only, as printed (D5) — and the server records a DRAFT in
// resolution_data.make_introductions.drafts. No `assets` row is created: the
// peer does not exist in anybody's retinue until they arrive, which may be after
// the roll (make), after another player writes their marginalia (mar), several
// rows later (delayed), or never at all.
//
// Request body: {"name": "..."}
func createPeerHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanMakeIntroductions)
		if !ok {
			return
		}
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
			return
		}

		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		name, ok := textField(w, "name", body.Name, maxAssetNameLen)
		if !ok {
			return
		}
		if name == "" {
			respondErr(w, http.StatusBadRequest, "name is required")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		mi := resData.EnsureMakeIntroductions()
		if int16(len(mi.Drafts)) >= mi.PeerCount {
			respondErr(w, http.StatusConflict, "all peers have already been named")
			return
		}

		draft := DraftPeer{ID: gamepkg.NewDraftPeerID(), Name: name, CreatorID: player.ID}
		mi.Drafts = append(mi.Drafts, draft)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record the new peer", err)
			return
		}

		respond(w, http.StatusCreated, map[string]any{
			"plan_id": plan.ID,
			"draft":   draft,
			"drafts":  mi.Drafts,
		})
	}
}

// finalizePeersHandler handles POST /api/plans/:planId/finalize-peers.
//
// Called once after all peer_count peers have been named via /create-peer.
// Creates the dice roll that drives the rest of MI resolution. Idempotent
// in the sense that calling it twice 409s the second time (the plan now
// has a roll).
//
// It also posts the named peers to the action log. Naming used to be visible
// through the assets it created; now that it only records drafts, this post is
// the table's one sight of who is being introduced before the dice decide their
// fate.
func finalizePeersHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanMakeIntroductions)
		if !ok {
			return
		}
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		mi := resData.EnsureMakeIntroductions()
		if int16(len(mi.Drafts)) != mi.PeerCount {
			respondErr(w, http.StatusConflict,
				fmt.Sprintf("expected %d peers named, got %d", mi.PeerCount, len(mi.Drafts)))
			return
		}
		if _, err := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID); err == nil {
			respondErr(w, http.StatusConflict, "plan roll already exists")
			return
		}

		g, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load game", err)
			return
		}
		difficulty := gamepkg.MakeIntroductionsDifficulty(resData)
		roll, err := createPlanRoll(ctx, deps.Q, deps.Manager, &g, plan, difficulty, plan.PreparerID)
		if err != nil {
			respondInternalErr(w, r, "could not create dice roll", err)
			return
		}
		miLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
			"%s sent word ahead of %s.",
			playerDisplayName(ctx, deps.Q, player.ID), miDraftList(mi.Drafts)))
		respond(w, http.StatusCreated, map[string]any{"plan_id": plan.ID, "roll": roll})
	}
}

// miDraftList renders drafts as an English list of marked-up names, for log
// lines that name several newcomers at once.
func miDraftList(drafts []DraftPeer) string {
	names := make([]string, 0, len(drafts))
	for _, d := range drafts {
		names = append(names, assetMark(d.Name))
	}
	switch len(names) {
	case 0:
		return "nobody"
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// ── Arrival: the crossing from draft to asset ────────────────────────────────

// introductionsArrivalHandler handles POST /api/plans/:planId/introductions-arrival.
//
// The arrival form. It pays the rule's "*Make:* Peers arrive successfully; add
// marginalia to each" one peer at a time, and it is also how a delayed peer
// turns up on the row they were rescheduled to. Either way this is the moment
// the draft becomes a real asset in the plan recipient's retinue
// (AssetRecipientForPlan, so a Make Demands keep_assets winner claims them).
//
// The two mar outcomes whose marginalia another player owes arrive through
// introductions-marginalia instead — same crossing, different author and, for
// other_retinue, a different retinue.
//
// Request body: {"draft_id": "...", "name": "...", "marginalia": "..."}
// The name is pre-filled from the draft and may be corrected here; the
// marginalia is required, which is what keeps a materialized peer from ever
// being blank.
func introductionsArrivalHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanMakeIntroductions)
		if !ok {
			return
		}
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
			return
		}

		var body struct {
			DraftID    string `json:"draft_id"`
			Name       string `json:"name"`
			Marginalia string `json:"marginalia"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DraftID == "" {
			respondErr(w, http.StatusBadRequest, "draft_id is required")
			return
		}
		name, ok := textField(w, "name", body.Name, maxAssetNameLen)
		if !ok {
			return
		}
		margText, ok := textField(w, "marginalia", body.Marginalia, maxMarginaliaLen)
		if !ok {
			return
		}
		if margText == "" {
			respondErr(w, http.StatusBadRequest, "a newcomer needs one marginalia to arrive")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		mi := resData.EnsureMakeIntroductions()
		if !mi.MakePending && !mi.DelayedArrival {
			respondErr(w, http.StatusConflict, "this plan is not waiting on any arrivals")
			return
		}
		draft := mi.Draft(body.DraftID)
		if draft == nil {
			respondErr(w, http.StatusBadRequest, "that peer was not introduced by this plan")
			return
		}
		if mi.HasArrived(draft.ID) {
			respondErr(w, http.StatusConflict, "that peer has already arrived")
			return
		}

		recipient, err := AssetRecipientForPlan(ctx, deps.Q, plan)
		if err != nil {
			respondInternalErr(w, r, "could not resolve asset recipient", err)
			return
		}

		arriving := *draft
		if name != "" {
			arriving.Name = name
		}
		arriving.Marginalia = margText

		var asset dbgen.Asset
		err = deps.InTx(ctx, func(q *dbgen.Queries) error {
			a, _, mErr := materializeDraftPeer(ctx, q, deps.Manager, plan.GameID, arriving, recipient)
			if mErr != nil {
				return mErr
			}
			asset = a
			draft.Name = arriving.Name
			draft.Marginalia = margText
			mi.Arrivals = append(mi.Arrivals, MIArrival{DraftID: draft.ID, AssetID: a.ID})
			return saveResolutionData(ctx, q, plan.ID, resData)
		})
		if err != nil {
			respondInternalErr(w, r, "could not bring the newcomer to court", err)
			return
		}

		if mi.DelayedArrival {
			miLog(ctx, deps, plan, model.SeverityImportant,
				fmt.Sprintf("%s finally arrived at court.", assetMark(asset.Name)))
			// A synthetic arrival plan never rolled, so it has no outcome for
			// CompletePlan to read and could never be completed by hand — it would
			// sit resolving and hold the row. The arrival is its whole content, so
			// finish it here: nothing is left to decide.
			if err := finalizePlanResolution(ctx, deps.Q, deps.Manager, plan, makeOutcome); err != nil {
				respondInternalErr(w, r, "could not close the arrival", err)
				return
			}
		}
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusCreated, map[string]any{
			"plan_id":  plan.ID,
			"draft_id": draft.ID,
			"asset_id": asset.ID,
		})
	}
}

// ── Delayed arrival scheduling ───────────────────────────────────────────────

// scheduleDelayedArrival rolls d6 and either schedules a synthetic per-peer
// arrival plan d6 rows ahead — carrying the draft, which materializes on that
// row — or, if the target row is past the public record, reports the peer lost.
// A lost peer leaves nothing behind: no asset was ever created, so there is
// nothing to destroy and nothing that could have been broken, taken or staked
// in the meantime. That is the whole point of D4.
//
// It appends the synthetic plan ID to
// parentResData.MakeIntroductions.DelayedPeerPlanIDs but does NOT persist
// parentResData; the caller saves it (so a caller updating other resData
// fields writes once). Returns the delay, target row, the synthetic plan ID
// (0 when lost), and whether the peer was lost.
func scheduleDelayedArrival(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	parentResData *ResolutionData,
	draft DraftPeer,
) (delay, targetRow int16, syntheticPlanID int64, lost bool, err error) {
	g, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return 0, 0, 0, false, err
	}
	delay = int16(rand.IntN(diceSides) + 1) // 1–6
	targetRow = g.CurrentRow + delay

	if targetRow > publicRecordRowCount {
		return delay, targetRow, 0, true, nil
	}

	count, cErr := deps.Q.CountPlansOnRow(ctx, dbgen.CountPlansOnRowParams{
		GameID:    g.ID,
		RowNumber: new(targetRow),
	})
	if cErr != nil {
		count = 0
	}

	parentPlanID := plan.ID
	syntheticResData := ResolutionData{
		MakeIntroductions: &MakeIntroductionsResolutionData{
			DelayedArrival: true,
			DelayedDraft:   &draft,
			OriginalPlanID: &parentPlanID,
			// One peer is riding on this plan, whatever the parent introduced.
			PeerCount: 1,
		},
	}

	var syntheticPlan dbgen.Plan
	err = deps.InTx(ctx, func(q *dbgen.Queries) error {
		sp, txErr := q.CreatePlan(ctx, dbgen.CreatePlanParams{
			GameID:        g.ID,
			PlanType:      model.PlanMakeIntroductions,
			Category:      model.CategoryKnowledge,
			PreparerID:    plan.PreparerID,
			RowNumber:     new(targetRow),
			RowOrder:      int16(count),
			PreparedAtRow: g.CurrentRow,
		})
		if txErr != nil {
			return errors.New("could not create delayed arrival plan")
		}
		syntheticPlan = sp
		if sErr := saveResolutionData(ctx, q, syntheticPlan.ID, syntheticResData); sErr != nil {
			return errors.New("could not save delayed arrival data")
		}
		return nil
	})
	if err != nil {
		return 0, 0, 0, false, err
	}

	pmi := parentResData.EnsureMakeIntroductions()
	pmi.DelayedPeerPlanIDs = append(pmi.DelayedPeerPlanIDs, syntheticPlan.ID)

	broadcastEvent(deps.Manager, g.ID, model.EventPlanDelayedArrival, model.PlanDelayedArrivalPayload{
		PlanID:     syntheticPlan.ID,
		PeerName:   draft.Name,
		ArrivalRow: targetRow,
	})
	return delay, targetRow, syntheticPlan.ID, false, nil
}

// ── Mar per-peer resolution ───────────────────────────────────────────────────

// introductionsMarHandler handles POST /api/plans/:planId/introductions-mar.
//
// On a marred Make Introductions, the preparer resolves each introduced peer
// with one of four outcomes. Request body:
//
//	{"draft_id": D, "outcome": "other_retinue", "target_player_id": P}
//	{"draft_id": D, "outcome": "broken_arrival", "target_player_id": P}
//	{"draft_id": D, "outcome": "delayed"}
//	{"draft_id": D, "outcome": "broken_journey", "text": "...", "journey_text": "..."}
//
// The peers are still drafts here — only broken_journey materializes one on the
// spot. other_retinue and broken_arrival hand the marginalia (and so the
// arrival) to another player via introductions-marginalia; delayed sends the
// draft to a later row, or drops it entirely.
func introductionsMarHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanMakeIntroductions)
		if !ok {
			return
		}
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
			return
		}

		var body struct {
			DraftID      string `json:"draft_id"`
			Outcome      string `json:"outcome"`
			TargetPlayer *int64 `json:"target_player_id"`
			Text         string `json:"text"`
			JourneyText  string `json:"journey_text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DraftID == "" {
			respondErr(w, http.StatusBadRequest, "draft_id and outcome are required")
			return
		}
		text, ok := textField(w, "text", body.Text, maxMarginaliaLen)
		if !ok {
			return
		}
		journeyText, ok := textField(w, "journey_text", body.JourneyText, maxMarginaliaLen)
		if !ok {
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		mi := resData.EnsureMakeIntroductions()
		if !mi.MarPending {
			respondErr(w, http.StatusConflict, "this plan is not resolving a mar")
			return
		}
		draft := gamepkg.FindDraftPeer(mi.Drafts, body.DraftID)
		if draft == nil {
			respondErr(w, http.StatusBadRequest, "that peer was not introduced by this plan")
			return
		}
		if mi.MarOutcomeFor(draft.ID) != nil {
			respondErr(w, http.StatusConflict, "that peer has already been resolved")
			return
		}

		outcome, status, clientMsg, err := applyMIPeerOutcome(ctx, deps, plan, &resData, miPeerOutcomeRequest{
			draft:        *draft,
			kind:         body.Outcome,
			targetPlayer: body.TargetPlayer,
			text:         text,
			journeyText:  journeyText,
			actorID:      player.ID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not resolve peer", err)
			return
		}
		if clientMsg != "" {
			respondErr(w, status, clientMsg)
			return
		}

		mi.MarOutcomes = append(mi.MarOutcomes, outcome)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record outcome", err)
			return
		}
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id":    plan.ID,
			"draft_id":   draft.ID,
			"outcome":    body.Outcome,
			"resolved":   len(mi.MarOutcomes),
			"peer_count": mi.PeerCount,
		})
	}
}

// miPeerOutcomeRequest is one introduced peer's mar resolution as the preparer
// submitted it.
type miPeerOutcomeRequest struct {
	draft        DraftPeer
	kind         string
	targetPlayer *int64
	// text is the peer's own marginalia — who arrived — for broken_journey.
	text string
	// journeyText is the mark the road left on them, torn on arrival so the peer
	// lands broken rather than destroyed (see the broken_journey case).
	journeyText string
	actorID     int64
}

// applyMIPeerOutcome applies one introduced peer's mar outcome and returns the
// recorded outcome. On a caller error it returns a non-empty clientMsg with the
// HTTP status; on an internal failure it returns a non-nil err. The caller
// persists resData (the outcome is appended to MarOutcomes there).
//
// Only broken_journey materializes its draft here. other_retinue and
// broken_arrival record who owes the marginalia and leave the draft in limbo
// until they write it (introductions-marginalia); delayed hands the draft to a
// synthetic plan on a later row, or drops it.
func applyMIPeerOutcome(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	req miPeerOutcomeRequest,
) (MIMarOutcome, int, string, error) {
	draft := req.draft
	outcome := MIMarOutcome{DraftID: draft.ID, Outcome: req.kind}

	switch req.kind {
	case "other_retinue":
		recipient, vErr := miValidateOtherPlayer(ctx, deps.Q, plan, req.targetPlayer)
		if vErr != "" {
			return outcome, http.StatusBadRequest, vErr, nil
		}
		// D6: the receiving player writes the marginalia, so this peer arrives
		// only once they have — the owner describes their own assets. That makes
		// (a) gate completion the same way (b) does.
		outcome.AuthorPlayerID = &recipient
		outcome.OwnerPlayerID = &recipient
		miLog(
			ctx,
			deps,
			plan,
			model.SeverityDefault,
			fmt.Sprintf(
				"%s is joining %s's retinue instead — %s will describe them.",
				assetMark(draft.Name),
				playerDisplayName(ctx, deps.Q, recipient),
				playerDisplayName(ctx, deps.Q, recipient),
			),
		)

	case "broken_arrival":
		author, vErr := miValidateOtherPlayer(ctx, deps.Q, plan, req.targetPlayer)
		if vErr != "" {
			return outcome, http.StatusBadRequest, vErr, nil
		}
		recipient, err := AssetRecipientForPlan(ctx, deps.Q, plan)
		if err != nil {
			return outcome, 0, "", err
		}
		outcome.AuthorPlayerID = &author // Done stays false until the author writes
		outcome.OwnerPlayerID = &recipient
		miLog(
			ctx,
			deps,
			plan,
			model.SeverityDefault,
			fmt.Sprintf(
				"%s arrived broken — %s will define them.",
				assetMark(draft.Name),
				playerDisplayName(ctx, deps.Q, author),
			),
		)

	case "delayed":
		_, targetRow, _, lost, err := scheduleDelayedArrival(ctx, deps, plan, resData, draft)
		if err != nil {
			return outcome, 0, "", err
		}
		if lost {
			// Nothing to destroy: the peer never had an assets row, so being lost
			// is simply the draft going nowhere.
			miLog(
				ctx,
				deps,
				plan,
				model.SeverityDefault,
				fmt.Sprintf("%s was lost on the journey and never reached court.", assetMark(draft.Name)),
			)
		} else {
			miLog(
				ctx,
				deps,
				plan,
				model.SeverityDefault,
				fmt.Sprintf("%s was delayed and will arrive on row %d.", assetMark(draft.Name), targetRow),
			)
		}
		outcome.Done = true

	case "broken_journey":
		if req.text == "" || req.journeyText == "" {
			return outcome, http.StatusBadRequest,
				"broken_journey needs the peer's marginalia and the mark the journey left", nil
		}
		if err := applyMIBrokenJourney(ctx, deps, plan, resData, req, &outcome); err != nil {
			return outcome, 0, "", err
		}

	default:
		return outcome, http.StatusBadRequest,
			"outcome must be other_retinue, broken_arrival, delayed, or broken_journey", nil
	}

	return outcome, 0, "", nil
}

// applyMIBrokenJourney materializes a peer who arrives broken from the road.
//
// The peer lands carrying two marginalia — who they are, and what the journey
// cost them — and only the second is torn, so they arrive broken rather than
// destroyed. Writing just the torn one would tear the asset's only marginalia,
// which is exactly the destroy-on-arrival bug this replaces (see the audit in
// adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md). It fills in outcome's owner and
// Done, and records the arrival on resData.
func applyMIBrokenJourney(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	req miPeerOutcomeRequest,
	outcome *MIMarOutcome,
) error {
	recipient, err := AssetRecipientForPlan(ctx, deps.Q, plan)
	if err != nil {
		return err
	}
	arriving := req.draft
	arriving.Marginalia = req.text

	var asset dbgen.Asset
	err = deps.InTx(ctx, func(q *dbgen.Queries) error {
		a, written, mErr := materializeDraftPeer(ctx, q, deps.Manager, plan.GameID, arriving, recipient)
		if mErr != nil {
			return mErr
		}
		asset = a
		m, cErr := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: a.ID, Position: int16(len(written) + 1), Text: req.journeyText,
		})
		if cErr != nil {
			return cErr
		}
		_, bErr := breakMarginalia(ctx, q, deps.Manager, &a, &m, req.actorID)
		return bErr
	})
	if err != nil {
		return err
	}

	mi := resData.EnsureMakeIntroductions()
	mi.Arrivals = append(mi.Arrivals, MIArrival{DraftID: req.draft.ID, AssetID: asset.ID})
	outcome.OwnerPlayerID = &recipient
	outcome.Done = true
	miLog(ctx, deps, plan, model.SeverityDefault,
		fmt.Sprintf("%s survived an arduous journey — arrived broken, marked by %q.",
			assetMark(asset.Name), req.journeyText))
	return nil
}

// miValidateOtherPlayer checks that target points at a player at this table who
// is not the preparer. Returns the player ID, or a non-empty error message.
func miValidateOtherPlayer(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan, target *int64) (int64, string) {
	if target == nil {
		return 0, "target_player_id is required for this outcome"
	}
	if *target == plan.PreparerID {
		return 0, "must be another player, not the preparer"
	}
	p, err := q.GetPlayerByID(ctx, *target)
	if err != nil || p.GameID != plan.GameID {
		return 0, "target_player_id must be a player at this table"
	}
	return *target, ""
}

// introductionsMarginaliaHandler handles POST /api/plans/:planId/introductions-marginalia.
//
// The other-player arrival form. Two mar outcomes put a peer's marginalia in
// somebody else's hands — "broken_arrival" (another player defines them) and
// "other_retinue" (the receiving player does, D6) — and since arrival is
// creation, writing it is also what brings the peer into a retinue: the one
// named in the outcome's OwnerPlayerID.
//
// Request body: {"draft_id": D, "text": "..."}
func introductionsMarginaliaHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanMakeIntroductions)
		if !ok {
			return
		}

		var body struct {
			DraftID string `json:"draft_id"`
			Text    string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).
			Decode(&body); err != nil || body.DraftID == "" ||
			strings.TrimSpace(body.Text) == "" {
			respondErr(w, http.StatusBadRequest, "draft_id and text are required")
			return
		}
		text, ok := textField(w, "text", body.Text, maxMarginaliaLen)
		if !ok {
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		mi := resData.EnsureMakeIntroductions()

		out := mi.MarOutcomeFor(body.DraftID)
		if out == nil || out.AuthorPlayerID == nil {
			respondErr(w, http.StatusConflict, "no newcomer awaits a marginalia here")
			return
		}
		if out.Done {
			respondErr(w, http.StatusConflict, "that peer's marginalia has already been written")
			return
		}
		if *out.AuthorPlayerID != player.ID {
			respondErr(w, http.StatusForbidden, "only the assigned author may write this marginalia")
			return
		}
		draft := gamepkg.FindDraftPeer(mi.Drafts, body.DraftID)
		if draft == nil {
			respondErr(w, http.StatusNotFound, "that peer was not introduced by this plan")
			return
		}
		owner := plan.PreparerID
		if out.OwnerPlayerID != nil {
			owner = *out.OwnerPlayerID
		}

		arriving := *draft
		arriving.Marginalia = text

		var asset dbgen.Asset
		err := deps.InTx(ctx, func(q *dbgen.Queries) error {
			a, _, mErr := materializeDraftPeer(ctx, q, deps.Manager, plan.GameID, arriving, owner)
			if mErr != nil {
				return mErr
			}
			asset = a
			draft.Marginalia = text
			out.Done = true
			mi.Arrivals = append(mi.Arrivals, MIArrival{DraftID: draft.ID, AssetID: a.ID})
			return saveResolutionData(ctx, q, plan.ID, resData)
		})
		if err != nil {
			respondInternalErr(w, r, "could not write marginalia", err)
			return
		}

		miLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s defined the newcomer %s, who joined %s's retinue.",
				playerDisplayName(ctx, deps.Q, player.ID),
				assetMark(asset.Name),
				playerDisplayName(ctx, deps.Q, owner)))
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":  plan.ID,
			"draft_id": draft.ID,
			"asset_id": asset.ID,
		})
	}
}
