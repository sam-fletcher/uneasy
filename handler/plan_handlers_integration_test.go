//go:build integration

package handler

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// ── PHASE 2: Plan Handler Integration Tests ─────────────────────────────────
// These tests validate the core business logic of plan handlers by calling
// their ValidatePreparation methods with various inputs.

// ── Make War Tests ───────────────────────────────────────────────────────────

func TestMakeWar_RejectsNoEnemies(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	vc := &ValidationContext{
		Q:              q,
		Game:           &tg.Game,
		Player:         &tg.Players[0],
		EnemyPlayerIDs: []int64{},
	}
	_, errMsg := mwHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
	assert.Contains(t, errMsg, "requires at least one")
}

func TestMakeWar_RejectedDuplicateEnemies(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	vc := &ValidationContext{
		Q:              q,
		Game:           &tg.Game,
		Player:         &tg.Players[0],
		EnemyPlayerIDs: []int64{tg.Players[1].ID, tg.Players[1].ID},
	}
	_, errMsg := mwHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
	assert.Contains(t, errMsg, "duplicates")
}

func TestMakeWar_AcceptsValidEnemies(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 4)
	ctx := context.Background()

	vc := &ValidationContext{
		Q:              q,
		Game:           &tg.Game,
		Player:         &tg.Players[0],
		EnemyPlayerIDs: []int64{tg.Players[1].ID, tg.Players[2].ID},
	}
	_, errMsg := mwHandler{}.ValidatePreparation(ctx, vc)
	assert.Empty(t, errMsg)
}

// ── Propose Duel Tests ───────────────────────────────────────────────────────

func TestProposeDuel_RejectsNoOpponent(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	vc := &ValidationContext{
		Q:      q,
		Game:   &tg.Game,
		Player: &tg.Players[0],
	}
	_, errMsg := pduelHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
}

func TestProposeDuel_RejectsSelfAsOpponent(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	opponentID := tg.Players[0].ID
	notes := "Courtyard duel"
	vc := &ValidationContext{
		Q:              q,
		Game:           &tg.Game,
		Player:         &tg.Players[0],
		TargetPlayerID: &opponentID,
		Notes:          notes,
	}
	_, errMsg := pduelHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
	assert.Contains(t, errMsg, "yourself")
}

func TestProposeDuel_RejectsNoNotes(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	opponentID := tg.Players[1].ID
	vc := &ValidationContext{
		Q:              q,
		Game:           &tg.Game,
		Player:         &tg.Players[0],
		TargetPlayerID: &opponentID,
	}
	_, errMsg := pduelHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
}

func TestProposeDuel_AcceptsValidDuel(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	opponentID := tg.Players[1].ID
	notes := "Courtyard duel at dawn"
	vc := &ValidationContext{
		Q:              q,
		Game:           &tg.Game,
		Player:         &tg.Players[0],
		TargetPlayerID: &opponentID,
		Notes:          notes,
	}
	_, errMsg := pduelHandler{}.ValidatePreparation(ctx, vc)
	assert.Empty(t, errMsg)
}

// ── Seek Answers Tests ───────────────────────────────────────────────────────

func TestSeekAnswers_RejectsNoNotes(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	vc := &ValidationContext{
		Q:      q,
		Game:   &tg.Game,
		Player: &tg.Players[0],
	}
	_, errMsg := saHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
}

func TestSeekAnswers_AcceptsWithNotes(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	notes := "Research tower origins in archives"
	vc := &ValidationContext{
		Q:      q,
		Game:   &tg.Game,
		Player: &tg.Players[0],
		Notes:  notes,
	}
	_, errMsg := saHandler{}.ValidatePreparation(ctx, vc)
	assert.Empty(t, errMsg)
}

// ── Spread Rumors Tests ──────────────────────────────────────────────────────

func TestSpreadRumors_RejectsNoTarget(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	vc := &ValidationContext{
		Q:      q,
		Game:   &tg.Game,
		Player: &tg.Players[0],
	}
	_, errMsg := srHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
}

func TestSpreadRumors_RejectsNoNotes(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Create target asset
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	vc := &ValidationContext{
		Q:             q,
		Game:          &tg.Game,
		Player:        &tg.Players[0],
		TargetAssetID: &asset.ID,
	}
	_, errMsg := srHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
}

func TestSpreadRumors_AcceptsWithTargetAndNotes(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Create target asset
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	notes := "Council betrayal rumor"
	vc := &ValidationContext{
		Q:             q,
		Game:          &tg.Game,
		Player:        &tg.Players[0],
		TargetAssetID: &asset.ID,
		Notes:         notes,
	}
	_, errMsg := srHandler{}.ValidatePreparation(ctx, vc)
	assert.Empty(t, errMsg)
}

// TestSpreadRumors_BreakTarget_DestroysAssetOnFinalMarginalium guards the rule
// that tearing an asset's last intact marginalia destroys it ("all 4 gone →
// the asset is destroyed"). The break-target route previously inlined
// TearMarginalia and skipped the destroy check, so the final tear left the
// asset alive. It now routes through breakMarginalia, which destroys.
func TestSpreadRumors_BreakTarget_DestroysAssetOnFinalMarginalium(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Target asset owned by another player, carrying all 4 marginalia with the
	// first 3 already torn — so a single break-target tear is the final one.
	targetOwnerIdx := 1
	target := h.seedPeer(targetOwnerIdx, "rumor target")
	var lastM dbgen.Marginalium
	for pos := int16(1); pos <= 4; pos++ {
		m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: target, Position: pos, Text: "a damning note",
		})
		require.NoError(t, err)
		if pos < 4 {
			_, err = h.q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{ID: m.ID})
			require.NoError(t, err)
		} else {
			lastM = m
		}
	}

	notes := "Council betrayal rumor"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSpreadRumors,
		TargetAssetID:    &target,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll, "Spread Rumors creates its roll on resolve")
	h.forceRoll(roll.ID, makeOutcome, 3)
	h.makeChoice(plan.ID, makeOutcome, []string{"break_target"})

	// The preparer tears the final intact marginalia via break-target.
	preparerIdx := -1
	for i, p := range h.tg.Players {
		if p.ID == plan.PreparerID {
			preparerIdx = i
		}
	}
	require.GreaterOrEqual(t, preparerIdx, 0, "preparer must be one of the seeded players")

	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-target"
	code, body := h.post(preparerIdx, breakPath, map[string]any{"marginalia_id": lastM.ID})
	require.Equalf(t, http.StatusOK, code, "break-target: %v", body)

	torn, err := h.q.GetMarginaliaByID(ctx, lastM.ID)
	require.NoError(t, err)
	assert.True(t, torn.IsTorn, "the final marginalia should be torn")

	destroyed, err := h.q.GetAssetByID(ctx, target)
	require.NoError(t, err)
	assert.True(t, destroyed.IsDestroyed,
		"tearing the final marginalia via break-target must destroy the asset")
}

// TestSpreadRumors_TakeAsset_KeepAssetsDemandIntercepts proves a resolved Make
// Demands keep_assets winner intercepts the preparer's "take_asset" spoils on a
// made roll: the taken asset lands with the demander, not the preparer.
func TestSpreadRumors_TakeAsset_KeepAssetsDemandIntercepts(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	prepIdx := h.focusPlayerIdx()
	victimIdx := (prepIdx + 1) % 3
	demanderIdx := (prepIdx + 2) % 3

	// Rumor subject owned by the victim, plus a separate spoils asset the
	// preparer will take from the victim on a made roll.
	target := h.seedPeer(victimIdx, "rumor target")
	spoils, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: h.tg.Game.ID, OwnerID: h.tg.Players[victimIdx].ID,
		CreatorID: h.tg.Players[victimIdx].ID, AssetType: model.AssetArtifact, Name: "victim's signet",
	})
	require.NoError(t, err)

	notes := "a damaging whisper"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSpreadRumors,
		TargetAssetID:    &target,
		PreparationNotes: &notes,
	})
	require.Equal(t, h.tg.Players[prepIdx].ID, plan.PreparerID, "focus player is the preparer")
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll, "Spread Rumors creates its roll on resolve")
	h.forceRoll(roll.ID, makeOutcome, 6) // generous budget for one take_asset

	// A resolved, made demand against the rumor plan hands keep_assets to the demander.
	h.seedMadeDemand(demanderIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionKeepAssets: h.tg.Players[demanderIdx].ID,
	})

	// Preparer requests to take the victim's signet; the victim must consent.
	reqPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/request-take-consent"
	code, body := h.post(prepIdx, reqPath, map[string]any{
		"choices": []string{"take_asset"}, "result": makeOutcome, "take_asset_ids": []int64{spoils.ID},
	})
	require.Equalf(t, http.StatusOK, code, "request-take-consent: %v", body)

	respPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/respond-take-consent"
	code, body = h.post(victimIdx, respPath, map[string]any{"agree": true})
	require.Equalf(t, http.StatusOK, code, "respond-take-consent: %v", body)

	moved, err := h.q.GetAssetByID(ctx, spoils.ID)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[demanderIdx].ID, moved.OwnerID,
		"keep_assets demand winner should receive the taken asset, not the preparer")
}

// TestSpreadRumors_HideSource_IsIdempotent covers the post-commit "hide source"
// sub-flow: writing the source-secret records server-side completion, and a
// second attempt (a stale client re-prompted after a refresh/remount) is
// rejected so it can't write a duplicate secret under a different asset.
func TestSpreadRumors_HideSource_IsIdempotent(t *testing.T) {
	h := newPlanLifecycle(t, 2)
	ctx := context.Background()

	focusIdx := h.focusPlayerIdx()
	otherIdx := (focusIdx + 1) % 2

	target := h.seedPeer(otherIdx, "rumor target")  // rumor about someone else's asset
	holder := h.seedPeer(focusIdx, "Cover Story")   // preparer's own asset to hide under
	other := h.seedPeer(focusIdx, "Spare Identity") // a different own asset for the retry

	notes := "the chancellor cheats at cards"
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanSpreadRumors, TargetAssetID: &target, PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll)
	h.forceRoll(roll.ID, makeOutcome, 1)
	h.makeChoice(plan.ID, makeOutcome, []string{"hide_source"})

	hidePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/hide-source"

	// First hide-source: secret written, completion recorded.
	code, body := h.post(focusIdx, hidePath, map[string]any{"secret_asset_id": holder})
	require.Equalf(t, http.StatusOK, code, "hide-source: %v", body)

	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(refreshed.ResolutionData)
	require.NotNil(t, rd.SpreadRumors)
	assert.Equal(t, 1, rd.SpreadRumors.HideSourceDone)
	secrets, err := h.q.ListSecretsByAsset(ctx, holder)
	require.NoError(t, err)
	assert.Len(t, secrets, 1, "exactly one source-secret written")

	// Second attempt against a different asset is rejected; no duplicate secret.
	code, body = h.post(focusIdx, hidePath, map[string]any{"secret_asset_id": other})
	require.Equalf(t, http.StatusConflict, code, "duplicate hide-source must be rejected: %v", body)
	otherSecrets, err := h.q.ListSecretsByAsset(ctx, other)
	require.NoError(t, err)
	assert.Empty(t, otherSecrets, "rejected retry must not write a second secret")
}

// srPlanPreparedPost returns the body of the plan.prepared chat post.
func srPlanPreparedPost(t *testing.T, h *planLifecycle) string {
	t.Helper()
	posts, err := h.q.ListGamePosts(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "plan.prepared" {
			return p.Body
		}
	}
	t.Fatal("no plan.prepared post found")
	return ""
}

// TestSpreadRumors_Secret_HidesTextAndNamesAssets covers the "keep it secret for
// now" prep option: the rumor text is stashed as a hidden Secret on the
// preparer's own asset (not the public plan note), and the prepared-log post
// names the target + holding assets without leaking the text.
func TestSpreadRumors_Secret_HidesTextAndNamesAssets(t *testing.T) {
	h := newPlanLifecycle(t, 2)
	ctx := context.Background()

	focusIdx := h.focusPlayerIdx()
	otherIdx := (focusIdx + 1) % 2
	target := h.seedPeer(otherIdx, "Julius") // the rumor is about this asset
	holder := h.seedPeer(focusIdx, "Brutus") // the preparer's own asset holds the secret

	notes := "the king poisoned his brother"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSpreadRumors,
		TargetAssetID:    &target,
		PreparationNotes: &notes,
		SecretAssetID:    &holder,
	})

	// The public plan note is cleared so ListPlans can't leak the rumor.
	assert.Nil(t, plan.PreparationNotes, "secret rumor must clear the public plan note")

	// The rumor text lives as a Secret on the chosen own asset.
	secrets, err := h.q.ListSecretsByAsset(ctx, holder)
	require.NoError(t, err)
	require.Len(t, secrets, 1)
	assert.Equal(t, notes, secrets[0].Text)

	// resolution_data records the secret metadata.
	resData := loadResolutionData(plan.ResolutionData)
	require.NotNil(t, resData.SpreadRumors)
	assert.True(t, resData.SpreadRumors.IsSecret)
	require.NotNil(t, resData.SpreadRumors.SecretID)
	assert.Equal(t, secrets[0].ID, *resData.SpreadRumors.SecretID)

	// The prepared-log post names both assets but not the rumor text.
	body := srPlanPreparedPost(t, h)
	assert.Contains(t, body, "Julius")
	assert.Contains(t, body, "Brutus")
	assert.NotContains(t, body, notes)
}

// TestSpreadRumors_Secret_RejectsForeignAsset guards that the secret holder must
// be one of the preparer's own assets.
func TestSpreadRumors_Secret_RejectsForeignAsset(t *testing.T) {
	h := newPlanLifecycle(t, 2)

	focusIdx := h.focusPlayerIdx()
	otherIdx := (focusIdx + 1) % 2
	target := h.seedPeer(otherIdx, "Julius")
	foreign := h.seedPeer(otherIdx, "NotYours") // owned by someone else

	notes := "a damning whisper"
	path := "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + "/prepare-plan"
	code, body := h.post(focusIdx, path, PreparePlanRequest{
		PlanType:         model.PlanSpreadRumors,
		TargetAssetID:    &target,
		PreparationNotes: &notes,
		SecretAssetID:    &foreign,
	})
	require.Equalf(t, http.StatusBadRequest, code, "expected 400, got: %v", body)
}

// TestSpreadRumors_Secret_PublishesOnMake confirms a kept-secret rumor becomes
// the public rumor (with its real text) when the plan succeeds.
func TestSpreadRumors_Secret_PublishesOnMake(t *testing.T) {
	h := newPlanLifecycle(t, 2)
	ctx := context.Background()

	focusIdx := h.focusPlayerIdx()
	otherIdx := (focusIdx + 1) % 2
	target := h.seedPeer(otherIdx, "Julius")
	holder := h.seedPeer(focusIdx, "Brutus")

	notes := "the crown was bought, not earned"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSpreadRumors,
		TargetAssetID:    &target,
		PreparationNotes: &notes,
		SecretAssetID:    &holder,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll)
	h.forceRoll(roll.ID, makeOutcome, 0)
	h.makeChoice(plan.ID, makeOutcome, []string{})

	rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, rumors, 1)
	assert.Equal(t, notes, rumors[0].Text, "the spread rumor must carry the kept secret's text")
}

// TestSpreadRumors_Open_StatesRumorInLog confirms the non-secret path states the
// rumor (and names the target asset) in the prepared-log post.
func TestSpreadRumors_Open_StatesRumorInLog(t *testing.T) {
	h := newPlanLifecycle(t, 2)

	focusIdx := h.focusPlayerIdx()
	otherIdx := (focusIdx + 1) % 2
	target := h.seedPeer(otherIdx, "Julius")

	notes := "the regent skims the treasury"
	h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSpreadRumors,
		TargetAssetID:    &target,
		PreparationNotes: &notes,
	})

	body := srPlanPreparedPost(t, h)
	assert.Contains(t, body, "Julius")
	assert.Contains(t, body, notes)
}

// ── PreparedDescriber: prepared-log naming for target-bearing plans ──────────
// These exercise the PreparedDescriptor methods directly (the formatting +
// name-resolution logic); the EmitPlanPrepared wiring is covered by the Spread
// Rumors tests above.

func TestPreparedDescriptor_MakeWar_NamesEnemies(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)

	plan := dbgen.Plan{PlanType: model.PlanMakeWar, PreparerID: tg.Players[0].ID}
	var rd ResolutionData
	rd.EnsureMakeWar().EnemyPlayerIDs = []int64{tg.Players[1].ID, tg.Players[2].ID}

	body, ok := mwHandler{}.PreparedDescriptor(context.Background(), q, plan, &rd)
	require.True(t, ok)
	assert.Contains(t, body, "declaring war on")
	assert.Contains(t, body, tg.Players[1].DisplayName)
	assert.Contains(t, body, tg.Players[2].DisplayName)
}

func TestPreparedDescriptor_ProposeDuel_NamesOpponentAndType(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)

	notes := "at dawn in the courtyard"
	plan := dbgen.Plan{
		PlanType:         model.PlanProposeDuel,
		PreparerID:       tg.Players[0].ID,
		TargetPlayerID:   &tg.Players[1].ID,
		PreparationNotes: &notes,
	}
	var rd ResolutionData
	rd.EnsureDuel().DuelType = "wits"

	body, ok := pduelHandler{}.PreparedDescriptor(context.Background(), q, plan, &rd)
	require.True(t, ok)
	assert.Contains(t, body, tg.Players[1].DisplayName)
	assert.Contains(t, body, "duel of wits")
	assert.Contains(t, body, notes) // notesSuffix appended
}

func TestPreparedDescriptor_ExchangeCourtiers_NamesPeer(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	peer, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: tg.Game.ID, OwnerID: tg.Players[1].ID, CreatorID: tg.Players[1].ID,
		AssetType: model.AssetPeer, Name: "Cassio",
	})
	require.NoError(t, err)

	plan := dbgen.Plan{PlanType: model.PlanExchangeCourtiers, PreparerID: tg.Players[0].ID, TargetAssetID: &peer.ID}
	body, ok := ecHandler{}.PreparedDescriptor(ctx, q, plan, nil)
	require.True(t, ok)
	assert.Contains(t, body, "Cassio")
	assert.Contains(t, body, "angling for the peer")
}

func TestPreparedDescriptor_Liaise_NamesBothPeers(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	mine, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: tg.Game.ID, OwnerID: tg.Players[0].ID, CreatorID: tg.Players[0].ID,
		AssetType: model.AssetPeer, Name: "Iago",
	})
	require.NoError(t, err)
	theirs, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: tg.Game.ID, OwnerID: tg.Players[1].ID, CreatorID: tg.Players[1].ID,
		AssetType: model.AssetPeer, Name: "Emilia",
	})
	require.NoError(t, err)

	plan := dbgen.Plan{PlanType: model.PlanClandestinelyLiaise, PreparerID: tg.Players[0].ID}
	var rd ResolutionData
	l := rd.EnsureLiaise()
	l.PreparerPeerID = &mine.ID
	l.PartnerPeerID = &theirs.ID

	body, ok := clHandler{}.PreparedDescriptor(ctx, q, plan, &rd)
	require.True(t, ok)
	assert.Contains(t, body, "secret meeting between")
	assert.Contains(t, body, "Iago")
	assert.Contains(t, body, "Emilia")
}

func TestPreparedDescriptor_MakeDemands_NamesTargetPlan(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	targetPlan := createPlanOnRow(t, q, &tg.Game, &tg.Players[1], model.PlanSpreadRumors, model.CategoryEsteem, 5)
	plan := dbgen.Plan{PlanType: model.PlanMakeDemands, PreparerID: tg.Players[0].ID, TargetedPlanID: &targetPlan.ID}

	body, ok := mdHandler{}.PreparedDescriptor(ctx, q, plan, nil)
	require.True(t, ok)
	assert.Contains(t, body, tg.Players[1].DisplayName)
	assert.Contains(t, body, "Spread Rumors")
}

// ── Spread Rumors: take-asset consent gate ──────────────────────────────────

// TestSpreadRumors_TakeConsent_AgreeTransfersAsset covers the happy path: the
// aggressor names an asset to take (any of the victim's, not just the rumor's
// target), the victim is asked, and on agreement the choices commit and the
// asset transfers. Also asserts the table blocks on the victim while pending,
// and that a non-victim cannot answer.
func TestSpreadRumors_TakeConsent_AgreeTransfersAsset(t *testing.T) {
	h := newPlanLifecycle(t, 2)
	ctx := context.Background()

	focusIdx := h.focusPlayerIdx()
	otherIdx := (focusIdx + 1) % 2
	preparerID := h.tg.Players[focusIdx].ID
	victimID := h.tg.Players[otherIdx].ID

	target := h.seedPeer(otherIdx, "Julius")     // the rumor's target asset
	loot := h.seedPeer(otherIdx, "Crown Jewels") // a DIFFERENT asset, still the victim's

	notes := "the king is a fraud"
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanSpreadRumors, TargetAssetID: &target, PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll)
	h.forceRoll(roll.ID, makeOutcome, 1)

	reqPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/request-take-consent"
	code, body := h.post(focusIdx, reqPath, map[string]any{
		"result": makeOutcome, "choices": []string{"take_asset"}, "take_asset_ids": []int64{loot},
	})
	require.Equalf(t, http.StatusOK, code, "request-take-consent: %v", body)
	assert.Equal(t, true, body["pending"])

	// Nothing committed before consent: no transfer, no rumor.
	a, err := h.q.GetAssetByID(ctx, loot)
	require.NoError(t, err)
	assert.Equal(t, victimID, a.OwnerID, "asset must not move before consent")
	rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	assert.Empty(t, rumors, "no rumor until the choices commit")

	// The row blocks on the victim.
	rs, err := ComputeRowState(ctx, h.q, h.tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitTakeConsent, rs.Kind)
	require.Len(t, rs.ActingPlayerIDs, 1)
	assert.Equal(t, victimID, rs.ActingPlayerIDs[0])

	respPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/respond-take-consent"

	// A non-victim (the aggressor) cannot answer their own request.
	code, body = h.post(focusIdx, respPath, map[string]any{"agree": true})
	require.Equalf(t, http.StatusForbidden, code, "non-victim respond: %v", body)

	// The victim agrees.
	code, body = h.post(otherIdx, respPath, map[string]any{"agree": true})
	require.Equalf(t, http.StatusOK, code, "respond agree: %v", body)

	// Asset transferred; choices recorded; take resolved; rumor created.
	a, err = h.q.GetAssetByID(ctx, loot)
	require.NoError(t, err)
	assert.Equal(t, preparerID, a.OwnerID, "asset must transfer to the aggressor on agreement")

	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(refreshed.ResolutionData)
	require.Len(t, rd.MakeMarChoices, 1)
	assert.Equal(t, "take_asset", rd.MakeMarChoices[0].Option)
	require.NotNil(t, rd.SpreadRumors)
	assert.True(t, rd.SpreadRumors.TakeResolved)
	assert.Nil(t, rd.SpreadRumors.PendingTakeConsent)
	rumors, err = h.q.ListRumors(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	assert.Len(t, rumors, 1, "agreement commits the rumor")
}

// TestSpreadRumors_TakeConsent_DisagreeBlocksAndDisables covers the refusal
// path: nothing transfers, no rumor is written, the option is flagged denied,
// and the aggressor can re-pick other options without take_asset.
func TestSpreadRumors_TakeConsent_DisagreeBlocksAndDisables(t *testing.T) {
	h := newPlanLifecycle(t, 2)
	ctx := context.Background()

	focusIdx := h.focusPlayerIdx()
	otherIdx := (focusIdx + 1) % 2
	victimID := h.tg.Players[otherIdx].ID

	target := h.seedPeer(otherIdx, "Julius")
	loot := h.seedPeer(otherIdx, "Crown Jewels")

	notes := "a damning whisper"
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanSpreadRumors, TargetAssetID: &target, PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll)
	h.forceRoll(roll.ID, makeOutcome, 1)

	reqPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/request-take-consent"
	code, body := h.post(focusIdx, reqPath, map[string]any{
		"result": makeOutcome, "choices": []string{"take_asset"}, "take_asset_ids": []int64{loot},
	})
	require.Equalf(t, http.StatusOK, code, "request-take-consent: %v", body)

	respPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/respond-take-consent"
	code, body = h.post(otherIdx, respPath, map[string]any{"agree": false})
	require.Equalf(t, http.StatusOK, code, "respond disagree: %v", body)

	// No transfer, no rumor, nothing committed.
	a, err := h.q.GetAssetByID(ctx, loot)
	require.NoError(t, err)
	assert.Equal(t, victimID, a.OwnerID, "asset must not move on refusal")
	rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	assert.Empty(t, rumors, "no rumor on refusal")

	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolving, refreshed.Status, "plan stays resolving after refusal")
	rd := loadResolutionData(refreshed.ResolutionData)
	require.NotNil(t, rd.SpreadRumors)
	assert.True(t, rd.SpreadRumors.TakeAssetDenied, "take option must be flagged denied")
	assert.Nil(t, rd.SpreadRumors.PendingTakeConsent, "pending request must clear")
	assert.Empty(t, rd.MakeMarChoices, "no choices committed on refusal")

	// The aggressor re-picks a non-take option and it commits normally.
	h.makeChoice(plan.ID, makeOutcome, []string{"reveal_source"})
}

// TestSpreadRumors_MakeChoice_RejectsTakeAsset guards that take_asset can't be
// committed through the generic make-choice route — it must go through the
// consent gate.
func TestSpreadRumors_MakeChoice_RejectsTakeAsset(t *testing.T) {
	h := newPlanLifecycle(t, 2)

	focusIdx := h.focusPlayerIdx()
	otherIdx := (focusIdx + 1) % 2
	target := h.seedPeer(otherIdx, "Julius")

	notes := "rumor"
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanSpreadRumors, TargetAssetID: &target, PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll)
	h.forceRoll(roll.ID, makeOutcome, 1)

	mcPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/make-choice"
	code, body := h.post(focusIdx, mcPath, map[string]any{
		"result": makeOutcome, "choices": []string{"take_asset"},
	})
	require.Equalf(t, http.StatusBadRequest, code, "make-choice with take_asset must be rejected: %v", body)
}

// TestSpreadRumors_CanComplete_BlockedWhilePendingTakeConsent proves the plan
// cannot be completed while a take-asset request is awaiting the victim's answer
// — completing would strand the in-flight transfer. Once the victim agrees, the
// plan completes. (Server-authoritative: CanComplete now blocks the pending take,
// not just the client.)
func TestSpreadRumors_CanComplete_BlockedWhilePendingTakeConsent(t *testing.T) {
	h := newPlanLifecycle(t, 2)

	focusIdx := h.focusPlayerIdx()
	otherIdx := (focusIdx + 1) % 2

	target := h.seedPeer(otherIdx, "Julius")
	loot := h.seedPeer(otherIdx, "Crown Jewels")

	notes := "the king is a fraud"
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanSpreadRumors, TargetAssetID: &target, PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll)
	h.forceRoll(roll.ID, makeOutcome, 1)

	reqPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/request-take-consent"
	code, body := h.post(focusIdx, reqPath, map[string]any{
		"result": makeOutcome, "choices": []string{"take_asset"}, "take_asset_ids": []int64{loot},
	})
	require.Equalf(t, http.StatusOK, code, "request-take-consent: %v", body)

	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"
	code, body = h.post(focusIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete must be blocked while consent is pending: %v", body)

	respPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/respond-take-consent"
	code, body = h.post(otherIdx, respPath, map[string]any{"agree": true})
	require.Equalf(t, http.StatusOK, code, "respond agree: %v", body)
	h.complete(plan.ID)
}

// TestSpreadRumors_ForfeitBreakTarget_DischargesWhenNoTarget proves a committed
// break_target pick with no intact marginalia left to tear blocks completion and
// can be forfeited as a no-op so the plan can resolve. Mirrors Seek Answers'
// forfeit escape hatch for the depletable break step.
func TestSpreadRumors_ForfeitBreakTarget_DischargesWhenNoTarget(t *testing.T) {
	h := newPlanLifecycle(t, 2)

	focusIdx := h.focusPlayerIdx()
	otherIdx := (focusIdx + 1) % 2

	// Target asset with NO intact marginalia — break_target has nothing to tear.
	target := h.seedPeer(otherIdx, "rumor target")

	notes := "an unfalsifiable whisper"
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanSpreadRumors, TargetAssetID: &target, PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll)
	h.forceRoll(roll.ID, makeOutcome, 1)
	h.makeChoice(plan.ID, makeOutcome, []string{"break_target"})

	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"
	code, body := h.post(focusIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code,
		"complete must be blocked with an unconsumed break_target pick: %v", body)

	forfeitPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/sr-forfeit-step"
	code, body = h.post(focusIdx, forfeitPath, map[string]any{"step": "break_target"})
	require.Equalf(t, http.StatusOK, code, "forfeit with no target should succeed: %v", body)
	assert.EqualValues(t, 1, body["forfeited"])

	h.complete(plan.ID)
}

// TestSpreadRumors_ForfeitBreakTarget_RejectedWhenTargetExists proves the server
// refuses to forfeit a break_target pick while the target asset still has intact
// marginalia to tear.
func TestSpreadRumors_ForfeitBreakTarget_RejectedWhenTargetExists(t *testing.T) {
	h := newPlanLifecycle(t, 2)
	ctx := context.Background()

	focusIdx := h.focusPlayerIdx()
	otherIdx := (focusIdx + 1) % 2

	target := h.seedPeer(otherIdx, "rumor target")
	_, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: target, Position: 1, Text: "a damning note",
	})
	require.NoError(t, err)

	notes := "a provable slight"
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanSpreadRumors, TargetAssetID: &target, PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll)
	h.forceRoll(roll.ID, makeOutcome, 1)
	h.makeChoice(plan.ID, makeOutcome, []string{"break_target"})

	forfeitPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/sr-forfeit-step"
	code, body := h.post(focusIdx, forfeitPath, map[string]any{"step": "break_target"})
	require.Equalf(t, http.StatusConflict, code,
		"forfeit must be rejected while a marginalia remains: %v", body)
}
