package handler

// handler/plan_make_introductions.go — Make Introductions plan handler (Phase 3b).
//
// Make Introductions (knowledge, delay 3): The preparer brings 1–4 new peers
// into the game. Difficulty = 2 + peer_count.
//
// Make: peers arrive successfully (created + given marginalia during the
// pre-roll); the make step confirms their arrival.
//
// Pre-roll flow: the focus player names each peer one at a time via
// POST /api/plans/:planId/create-peer, which routes ownership through
// AssetRecipientForPlan (so a resolved Make Demands keep_assets
// winner claims them) and records each new asset ID in
// resolution_data.make_introductions.created_peer_ids. Once peer_count
// peers exist, POST /api/plans/:planId/finalize-peers creates the dice
// roll and resolution proceeds normally.
//
// Mar (per-peer): on a mar the focus player resolves EACH introduced peer
// with one of four outcomes via POST /api/plans/:planId/introductions-mar:
//   - other_retinue  → the peer joins another player's retinue (transfer)
//   - broken_arrival → another player authors the peer's marginalia (written
//     later via introductions-marginalia)
//   - delayed        → arrival rescheduled d6 rows ahead (synthetic plan; if
//     the row exceeds the public record the peer is lost)
//   - broken_journey → the focus player writes a marginalia, then breaks the peer
//
// A synthetic delayed-arrival plan resolves on its row later (OnResolve returns
// no roll; CanComplete allows immediate completion since MarPending is unset).

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

// OnResolve defers the dice roll until the focus player has named each of
// the peer_count peers via /create-peer and called /finalize-peers. That
// matches the rule's "pre-roll: create new peer assets with names only"
// step. Synthetic delayed-arrival plans skip the roll entirely.
func (miHandler) OnResolve(_ context.Context, _ *PlanDeps, _ *dbgen.Plan) (*dbgen.DiceRoll, error) {
	return nil, nil
}

// CanComplete gates a marred plan until every introduced peer has a resolved
// per-peer outcome (and any broken-arrival author has written the marginalia).
// Synthetic delayed-arrival child plans have no MarPending and complete freely.
func (miHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	mi := resData.MakeIntroductions
	if mi == nil || !mi.MarPending {
		return nil
	}
	if int16(len(mi.MarOutcomes)) < mi.PeerCount {
		return fmt.Errorf("resolve all %d introduced peers before completing (%d resolved)",
			mi.PeerCount, len(mi.MarOutcomes))
	}
	for _, o := range mi.MarOutcomes {
		if !o.Done {
			return errors.New("a broken-arrival peer is still waiting for another player to write its marginalia")
		}
	}
	return nil
}

func (miHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"create-peer":              createPeerHandler(deps),
		"finalize-peers":           finalizePeersHandler(deps),
		"delayed-arrival":          delayedArrivalHandler(deps),
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
		// Peers were created (and given marginalia) during the pre-roll naming
		// step; the make step just confirms their successful arrival.
		miLog(ctx, deps, plan, model.SeverityImportant, "The new peers arrived at court.")
		return nil
	}
	// Mar: the focus player resolves each introduced peer individually via the
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

// ── Pre-roll peer creation extra routes ──────────────────────────────────────

// createPeerHandler handles POST /api/plans/:planId/create-peer.
//
// Called once per peer during the pre-roll naming step. The focus player
// (= preparer) submits a peer name and optional marginalia; the server
// creates the peer asset (routed through AssetRecipientForPlan so a
// resolved Make Demands keep_assets winner claims it) and appends the new
// asset ID to resolution_data.make_introductions.created_peer_ids.
//
// Request body: {"name": "...", "marginalia": ["text", ...]}
//

func createPeerHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanMakeIntroductions {
			respondErr(w, http.StatusBadRequest, "create-peer is only for Make Introductions")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the preparer can name peers")
			return
		}

		var body struct {
			Name       string   `json:"name"`
			Marginalia []string `json:"marginalia"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" {
			respondErr(w, http.StatusBadRequest, "name is required")
			return
		}
		if len(body.Marginalia) > maxMarginalia {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("at most %d marginalia", maxMarginalia))
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		mi := resData.EnsureMakeIntroductions()
		if int16(len(mi.CreatedPeerIDs)) >= mi.PeerCount {
			respondErr(w, http.StatusConflict, "all peers have already been named")
			return
		}

		recipient, err := AssetRecipientForPlan(ctx, deps.Q, plan)
		if err != nil {
			respondInternalErr(w, r, "could not resolve asset recipient", err)
			return
		}

		var asset dbgen.Asset
		var marginalia []dbgen.Marginalium
		err = deps.InTx(ctx, func(q *dbgen.Queries) error {
			a, caErr := q.CreateAsset(ctx, dbgen.CreateAssetParams{
				GameID:    plan.GameID,
				OwnerID:   recipient,
				CreatorID: player.ID,
				AssetType: model.AssetPeer,
				Name:      body.Name,
			})
			if caErr != nil {
				return errors.New("could not create peer")
			}
			asset = a
			marginalia = make([]dbgen.Marginalium, 0, len(body.Marginalia))
			for i, text := range body.Marginalia {
				text = strings.TrimSpace(text)
				if text == "" {
					continue
				}
				m, mErr := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
					AssetID:  asset.ID,
					Position: int16(i + 1),
					Text:     text,
				})
				if mErr != nil {
					return errors.New("could not create marginalia")
				}
				marginalia = append(marginalia, m)
			}
			mi.CreatedPeerIDs = append(mi.CreatedPeerIDs, asset.ID)
			return saveResolutionData(ctx, q, plan.ID, resData)
		})
		if err != nil {
			respondInternalErr(w, r, "could not create peer", err)
			return
		}

		result := assetWithMarginalia{Asset: asset, Marginalia: marginalia}
		broadcastEvent(deps.Manager, plan.GameID, model.EventAssetCreated,
			model.AssetPayload{Asset: result})
		respond(w, http.StatusCreated, map[string]any{
			"plan_id":          plan.ID,
			"asset":            result,
			"created_peer_ids": mi.CreatedPeerIDs,
		})
	}
}

// finalizePeersHandler handles POST /api/plans/:planId/finalize-peers.
//
// Called once after all peer_count peers have been named via /create-peer.
// Creates the dice roll that drives the rest of MI resolution. Idempotent
// in the sense that calling it twice 409s the second time (the plan now
// has a roll).
func finalizePeersHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanMakeIntroductions {
			respondErr(w, http.StatusBadRequest, "finalize-peers is only for Make Introductions")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the preparer can finalize peers")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		mi := resData.EnsureMakeIntroductions()
		if int16(len(mi.CreatedPeerIDs)) != mi.PeerCount {
			respondErr(w, http.StatusConflict,
				fmt.Sprintf("expected %d peers named, got %d", mi.PeerCount, len(mi.CreatedPeerIDs)))
			return
		}
		if _, err := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID); err == nil {
			respondErr(w, http.StatusConflict, "plan roll already exists")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load game", err)
			return
		}
		difficulty := gamepkg.MakeIntroductionsDifficulty(resData)
		roll, err := createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
		if err != nil {
			respondInternalErr(w, r, "could not create dice roll", err)
			return
		}
		respond(w, http.StatusCreated, map[string]any{"plan_id": plan.ID, "roll": roll})
	}
}

// ── Delayed Arrival extra route ───────────────────────────────────────────────

// delayedArrivalHandler handles POST /api/plans/:planId/delayed-arrival.
//
// Called by the focus player during MI resolution when they choose the mar
// option "delayed" for a peer. The player calls this once per delayed peer.
//
// Request body: {"peer_asset_id": 123}
//
// Effects:
//   - Rolls d6 to determine delay.
//   - If current_row + d6 > 13: destroys the peer asset (lost in transit).
//   - Otherwise: creates a synthetic pending plan on the target row with
//     ResData.DelayedArrival = true, and records its ID in the parent
//     plan's ResData.DelayedPeerPlanIDs.
//

func delayedArrivalHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanMakeIntroductions {
			respondErr(w, http.StatusBadRequest, "delayed-arrival is only for Make Introductions")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the preparer can schedule delayed arrivals")
			return
		}

		var body struct {
			PeerAssetID int64 `json:"peer_asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PeerAssetID == 0 {
			respondErr(w, http.StatusBadRequest, "peer_asset_id is required")
			return
		}

		ctx := r.Context()

		// Validate: must be a peer asset in this game.
		asset, err := deps.Q.GetAssetByID(ctx, body.PeerAssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "peer asset not found")
			return
		}
		if asset.GameID != plan.GameID {
			respondErr(w, http.StatusBadRequest, "asset does not belong to this game")
			return
		}
		if asset.AssetType != model.AssetPeer {
			respondErr(w, http.StatusBadRequest, "target asset must be a peer")
			return
		}

		parentResData := loadResolutionData(plan.ResolutionData)
		delay, targetRow, syntheticPlanID, lost, err := scheduleDelayedArrival(
			ctx, deps, plan, &parentResData, body.PeerAssetID)
		if err != nil {
			respondInternalErr(w, r, "could not schedule delayed arrival", err)
			return
		}
		if lost {
			respond(w, http.StatusOK, map[string]any{
				"peer_asset_id": body.PeerAssetID,
				"delay":         delay,
				"target_row":    targetRow,
				"outcome":       "lost",
				"note":          "peer was lost — target row exceeds row 13",
			})
			return
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, parentResData); err != nil {
			respondInternalErr(w, r, "could not update parent plan data", err)
			return
		}

		respond(w, http.StatusCreated, map[string]any{
			"peer_asset_id":     body.PeerAssetID,
			"delay":             delay,
			"target_row":        targetRow,
			"synthetic_plan_id": syntheticPlanID,
		})
	}
}

// scheduleDelayedArrival rolls d6 and either schedules a synthetic per-peer
// arrival plan d6 rows ahead, or — if that row is past the public record —
// destroys the peer (lost in transit). It appends the synthetic plan ID to
// parentResData.MakeIntroductions.DelayedPeerPlanIDs but does NOT persist
// parentResData; the caller saves it (so a caller updating other resData
// fields writes once). Returns the delay, target row, the synthetic plan ID
// (0 when lost), and whether the peer was lost.
func scheduleDelayedArrival(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	parentResData *ResolutionData,
	peerAssetID int64,
) (delay, targetRow int16, syntheticPlanID int64, lost bool, err error) {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return 0, 0, 0, false, err
	}
	delay = int16(rand.IntN(diceSides) + 1) // 1–6
	targetRow = game.CurrentRow + delay

	if targetRow > publicRecordRowCount {
		if err = deps.Q.DestroyAsset(ctx, peerAssetID); err != nil {
			return 0, 0, 0, false, err
		}
		broadcastEvent(deps.Manager, game.ID, model.EventAssetDestroyed,
			model.AssetIDPayload{AssetID: peerAssetID})
		return delay, targetRow, 0, true, nil
	}

	count, cErr := deps.Q.CountPlansOnRow(ctx, dbgen.CountPlansOnRowParams{
		GameID:    game.ID,
		RowNumber: new(targetRow),
	})
	if cErr != nil {
		count = 0
	}

	parentPlanID := plan.ID
	parentPeerCount := int16(0)
	if pmi := parentResData.MakeIntroductions; pmi != nil {
		parentPeerCount = pmi.PeerCount
	}
	syntheticResData := ResolutionData{
		MakeIntroductions: &MakeIntroductionsResolutionData{
			DelayedArrival:     true,
			DelayedPeerAssetID: &peerAssetID,
			OriginalPlanID:     &parentPlanID,
			PeerCount:          parentPeerCount,
		},
	}

	var syntheticPlan dbgen.Plan
	err = deps.InTx(ctx, func(q *dbgen.Queries) error {
		sp, txErr := q.CreatePlan(ctx, dbgen.CreatePlanParams{
			GameID:        game.ID,
			PlanType:      model.PlanMakeIntroductions,
			Category:      model.CategoryKnowledge,
			PreparerID:    plan.PreparerID,
			RowNumber:     new(targetRow),
			RowOrder:      int16(count),
			PreparedAtRow: game.CurrentRow,
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

	broadcastEvent(deps.Manager, game.ID, model.EventPlanDelayedArrival, model.PlanDelayedArrivalPayload{
		PlanID:      syntheticPlan.ID,
		PeerAssetID: peerAssetID,
		ArrivalRow:  targetRow,
	})
	return delay, targetRow, syntheticPlan.ID, false, nil
}

// ── Mar per-peer resolution ───────────────────────────────────────────────────

// miCreatedPeer reports whether assetID was one of the peers introduced by this
// plan (named during the pre-roll), and whether it has already been resolved.
func miCreatedPeer(mi *MakeIntroductionsResolutionData, assetID int64) (created, alreadyResolved bool) {
	for _, id := range mi.CreatedPeerIDs {
		if id == assetID {
			created = true
		}
	}
	for _, o := range mi.MarOutcomes {
		if o.PeerAssetID == assetID {
			alreadyResolved = true
		}
	}
	return created, alreadyResolved
}

// introductionsMarHandler handles POST /api/plans/:planId/introductions-mar.
//
// On a marred Make Introductions, the focus player resolves each introduced
// peer with one of four outcomes. Request body:
//
//	{"peer_asset_id": A, "outcome": "other_retinue", "target_player_id": P}
//	{"peer_asset_id": A, "outcome": "broken_arrival", "target_player_id": P}
//	{"peer_asset_id": A, "outcome": "delayed"}
//	{"peer_asset_id": A, "outcome": "broken_journey", "text": "..."}
//
// other_retinue transfers the peer to another player; broken_arrival assigns
// another player to author its marginalia (completed via introductions-marginalia);
// delayed reschedules arrival d6 rows ahead; broken_journey writes a marginalia
// then breaks the peer.
func introductionsMarHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanMakeIntroductions)
		if !ok {
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the preparer resolves the introductions")
			return
		}

		var body struct {
			PeerAssetID  int64  `json:"peer_asset_id"`
			Outcome      string `json:"outcome"`
			TargetPlayer *int64 `json:"target_player_id"`
			Text         string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PeerAssetID == 0 {
			respondErr(w, http.StatusBadRequest, "peer_asset_id and outcome are required")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		mi := resData.EnsureMakeIntroductions()
		if !mi.MarPending {
			respondErr(w, http.StatusConflict, "this plan is not resolving a mar")
			return
		}
		created, already := miCreatedPeer(mi, body.PeerAssetID)
		if !created {
			respondErr(w, http.StatusBadRequest, "that peer was not introduced by this plan")
			return
		}
		if already {
			respondErr(w, http.StatusConflict, "that peer has already been resolved")
			return
		}

		peer, err := deps.Q.GetAssetByID(ctx, body.PeerAssetID)
		if err != nil || peer.GameID != plan.GameID {
			respondErr(w, http.StatusNotFound, "peer not found in this game")
			return
		}

		outcome, status, clientMsg, err := applyMIPeerOutcome(
			ctx, deps, plan, &peer, &resData, body.Outcome, body.TargetPlayer, body.Text, player.ID)
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
		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"peer_asset_id": body.PeerAssetID,
			"outcome":       body.Outcome,
			"resolved":      len(mi.MarOutcomes),
			"peer_count":    mi.PeerCount,
		})
	}
}

// applyMIPeerOutcome applies one introduced peer's mar outcome and returns the
// recorded outcome. On a caller error it returns a non-empty clientMsg with the
// HTTP status; on an internal failure it returns a non-nil err. The caller
// persists resData (the outcome is appended to MarOutcomes there).
func applyMIPeerOutcome(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	peer *dbgen.Asset,
	resData *ResolutionData,
	kind string,
	targetPlayer *int64,
	text string,
	actorID int64,
) (MIMarOutcome, int, string, error) {
	outcome := MIMarOutcome{PeerAssetID: peer.ID, Outcome: kind}

	switch kind {
	case "other_retinue":
		recipient, vErr := miValidateOtherPlayer(ctx, deps.Q, plan, targetPlayer)
		if vErr != "" {
			return outcome, http.StatusBadRequest, vErr, nil
		}
		if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{ID: peer.ID, OwnerID: recipient}); err != nil {
			return outcome, 0, "", err
		}
		updated, _ := deps.Q.GetAssetByID(ctx, peer.ID)
		broadcastEvent(deps.Manager, plan.GameID, model.EventAssetTaken, model.AssetTakenPayload{
			Asset: updated, OldOwnerID: plan.PreparerID, NewOwnerID: recipient,
		})
		outcome.Done = true
		miLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%q joined %s's retinue instead.", peer.Name, playerDisplayName(ctx, deps.Q, recipient)))

	case "broken_arrival":
		author, vErr := miValidateOtherPlayer(ctx, deps.Q, plan, targetPlayer)
		if vErr != "" {
			return outcome, http.StatusBadRequest, vErr, nil
		}
		outcome.AuthorPlayerID = &author // Done stays false until the author writes
		miLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%q arrived broken — %s will define them.", peer.Name, playerDisplayName(ctx, deps.Q, author)))

	case "delayed":
		_, _, _, lost, err := scheduleDelayedArrival(ctx, deps, plan, resData, peer.ID)
		if err != nil {
			return outcome, 0, "", err
		}
		if lost {
			miLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%q was lost on the journey.", peer.Name))
		} else {
			miLog(
				ctx,
				deps,
				plan,
				model.SeverityDefault,
				fmt.Sprintf("%q was delayed and will arrive later.", peer.Name),
			)
		}
		outcome.Done = true

	case "broken_journey":
		if text == "" {
			return outcome, http.StatusBadRequest, "text is required for broken_journey", nil
		}
		pos, _ := deps.Q.CountMarginalia(ctx, peer.ID)
		if pos >= maxMarginalia {
			return outcome, http.StatusConflict, "that peer has no room for more marginalia", nil
		}
		m, err := deps.Q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: peer.ID, Position: int16(pos + 1), Text: text,
		})
		if err != nil {
			return outcome, 0, "", err
		}
		if _, err := breakMarginalia(ctx, deps.Q, deps.Manager, peer, &m, actorID); err != nil {
			return outcome, 0, "", err
		}
		outcome.Done = true
		miLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%q survived an arduous journey — arrived broken.", peer.Name))

	default:
		return outcome, http.StatusBadRequest,
			"outcome must be other_retinue, broken_arrival, delayed, or broken_journey", nil
	}

	return outcome, 0, "", nil
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
// For a "broken_arrival" peer, the assigned author writes the peer's marginalia.
// Request body: {"peer_asset_id": A, "text": "..."}
func introductionsMarginaliaHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanMakeIntroductions)
		if !ok {
			return
		}

		var body struct {
			PeerAssetID int64  `json:"peer_asset_id"`
			Text        string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).
			Decode(&body); err != nil || body.PeerAssetID == 0 ||
			strings.TrimSpace(body.Text) == "" {
			respondErr(w, http.StatusBadRequest, "peer_asset_id and text are required")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		mi := resData.EnsureMakeIntroductions()

		idx := -1
		for i, o := range mi.MarOutcomes {
			if o.PeerAssetID == body.PeerAssetID && o.Outcome == "broken_arrival" {
				idx = i
			}
		}
		if idx < 0 {
			respondErr(w, http.StatusConflict, "no broken-arrival peer awaits a marginalia here")
			return
		}
		out := mi.MarOutcomes[idx]
		if out.Done {
			respondErr(w, http.StatusConflict, "that peer's marginalia has already been written")
			return
		}
		if out.AuthorPlayerID == nil || *out.AuthorPlayerID != player.ID {
			respondErr(w, http.StatusForbidden, "only the assigned author may write this marginalia")
			return
		}

		peer, err := deps.Q.GetAssetByID(ctx, body.PeerAssetID)
		if err != nil || peer.GameID != plan.GameID {
			respondErr(w, http.StatusNotFound, "peer not found in this game")
			return
		}
		pos, _ := deps.Q.CountMarginalia(ctx, peer.ID)
		if pos >= maxMarginalia {
			respondErr(w, http.StatusConflict, "that peer has no room for more marginalia")
			return
		}
		m, err := deps.Q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: peer.ID, Position: int16(pos + 1), Text: strings.TrimSpace(body.Text),
		})
		if err != nil {
			respondInternalErr(w, r, "could not write marginalia", err)
			return
		}

		mi.MarOutcomes[idx].Done = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record marginalia", err)
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, model.EventAssetUpdated, model.AssetIDPayload{AssetID: peer.ID})
		miLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s defined the newcomer %q.", playerDisplayName(ctx, deps.Q, player.ID), peer.Name))

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"peer_asset_id": peer.ID,
			"marginalia_id": m.ID,
		})
	}
}
