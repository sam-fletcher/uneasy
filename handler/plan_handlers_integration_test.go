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
// that tearing an asset's last intact marginalium destroys it ("all 4 gone →
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

	// The preparer tears the final intact marginalium via break-target.
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
	assert.True(t, torn.IsTorn, "the final marginalium should be torn")

	destroyed, err := h.q.GetAssetByID(ctx, target)
	require.NoError(t, err)
	assert.True(t, destroyed.IsDestroyed,
		"tearing the final marginalium via break-target must destroy the asset")
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
