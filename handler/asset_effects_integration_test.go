//go:build integration

// handler/asset_effects_integration_test.go — the blank-asset break backstop
// (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md, D3).
//
// An asset carrying no marginalia rows at all used to be invulnerable: every
// break site named a marginalia to tear, and DestroyIfAllMarginaliaTorn is
// guarded by an EXISTS on the same table, so nothing could remove it from play.
// These cover the fix at three levels — the shared helper, one plan break, and
// one shake-up break — plus the eligibility widening that has to accompany it,
// since a forfeit hatch refuses to discharge a pick while a target remains.

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// ── breakAsset ───────────────────────────────────────────────────────────────

// TestBreakAsset_BlankAssetIsDestroyedOutright is the core of the backstop: with
// m == nil the helper skips the tear entirely and destroys the asset, writing
// the same asset.destroyed post the last-tear path does.
func TestBreakAsset_BlankAssetIsDestroyedOutright(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)
	manager := hub.NewManager()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: tg.Game.ID, OwnerID: tg.Players[0].ID, CreatorID: tg.Players[0].ID,
		AssetType: model.AssetHolding, Name: "unwritten holding",
	})
	require.NoError(t, err)

	destroyed, err := breakAsset(ctx, q, manager, &asset, nil, tg.Players[1].ID)
	require.NoError(t, err)
	assert.True(t, destroyed, "breaking a blank asset always destroys it")

	after, err := q.GetAssetByID(ctx, asset.ID)
	require.NoError(t, err)
	assert.True(t, after.IsDestroyed, "the asset must be gone from play")
	assert.NotNil(t, after.DestroyedAt)

	// No marginalia were invented on the way — the row count stays zero, so the
	// asset is still blank as far as any later query is concerned.
	n, err := q.CountMarginalia(ctx, asset.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, n, "a blank break must not create or tear anything")

	assert.Contains(t, allPostBodies(t, q, tg.Game.ID), "destroyed",
		"the canonical asset.destroyed post is still written")
}

// A tear on an asset that still has notes left behaves exactly as before —
// pinned here so the m == nil branch can't be mistaken for a behaviour change.
func TestBreakAsset_TearPathUnchanged(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)
	manager := hub.NewManager()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: tg.Game.ID, OwnerID: tg.Players[0].ID, CreatorID: tg.Players[0].ID,
		AssetType: model.AssetHolding, Name: "two-note holding",
	})
	require.NoError(t, err)
	first, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: asset.ID, Position: 1, Text: "note one",
	})
	require.NoError(t, err)
	_, err = q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: asset.ID, Position: 2, Text: "note two",
	})
	require.NoError(t, err)

	destroyed, err := breakAsset(ctx, q, manager, &asset, &first, tg.Players[1].ID)
	require.NoError(t, err)
	assert.False(t, destroyed, "one note remains, so the asset survives")

	torn, err := q.GetMarginaliaByID(ctx, first.ID)
	require.NoError(t, err)
	assert.True(t, torn.IsTorn)
}

// assetIsBreakable is what every forfeit hatch and soft-lock guard consults. It
// must agree with breakAsset about the blank case, or a plan commits a pick it
// can never spend (or refuses a forfeit it should allow).
func TestAssetIsBreakable(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)

	newAsset := func(name string) dbgen.Asset {
		a, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID: tg.Game.ID, OwnerID: tg.Players[0].ID, CreatorID: tg.Players[0].ID,
			AssetType: model.AssetHolding, Name: name,
		})
		require.NoError(t, err)
		return a
	}

	blank := newAsset("blank")
	breakable, err := assetIsBreakable(ctx, q, blank.ID)
	require.NoError(t, err)
	assert.True(t, breakable, "a blank asset is breakable — the break destroys it")

	intact := newAsset("with a note")
	_, err = q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: intact.ID, Position: 1, Text: "a note",
	})
	require.NoError(t, err)
	breakable, err = assetIsBreakable(ctx, q, intact.ID)
	require.NoError(t, err)
	assert.True(t, breakable)

	// All-torn-but-alive: unreachable in a live game (the last tear destroys),
	// but the one shape that genuinely has nothing left to break.
	allTorn := newAsset("all torn")
	m, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: allTorn.ID, Position: 1, Text: "a note",
	})
	require.NoError(t, err)
	tornBy := tg.Players[1].ID
	_, err = q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{ID: m.ID, TornByID: &tornBy})
	require.NoError(t, err)
	breakable, err = assetIsBreakable(ctx, q, allTorn.ID)
	require.NoError(t, err)
	assert.False(t, breakable, "nothing intact and not blank — nothing to break")
}

// ── One plan break: Seek Answers ─────────────────────────────────────────────

// TestSeekAnswers_BreakResource_DestroysBlankResource proves the make-list break
// can target a resource with no marginalia at all: the route accepts an omitted
// marginalia_id, the asset is destroyed, and the log reads "destroyed" rather
// than "broke".
func TestSeekAnswers_BreakResource_DestroysBlankResource(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 2)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	// margs = 0 → a blank resource, the shape that used to be untouchable.
	resourceID, _ := saSeedResource(t, h, otherIdx, "unwritten ledger", 0)

	h.makeChoice(plan.ID, "make", []string{"break_resource"})

	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-resource"
	code, body := h.post(preparerIdx, breakPath, map[string]any{"asset_id": resourceID})
	require.Equalf(t, http.StatusOK, code, "break-resource on a blank asset: %v", body)
	assert.Equal(t, true, body["destroyed"], "a blank asset has nothing to tear — the break destroys it")
	assert.EqualValues(t, 0, body["marginalia_id"], "no marginalia was torn")

	destroyed, err := h.q.GetAssetByID(ctx, resourceID)
	require.NoError(t, err)
	assert.True(t, destroyed.IsDestroyed, "the blank resource must be destroyed")

	logs := saAllLogBodies(t, h)
	assert.Contains(t, logs, "destroyed", "breakVerb(true) wording")
	assert.Contains(t, logs, "unwritten ledger", "the plan log names the asset")
	assert.NotContains(t, logs, "The torn marginalia read",
		"nothing was torn, so no torn-text detail should be quoted")

	h.complete(plan.ID)
}

// TestSeekAnswers_BreakResource_RejectsOmittedMarginaliaOnWrittenAsset proves the
// omitted-marginalia shorthand is scoped to blank assets: an asset with notes
// still has to name which one is torn, so a stale client can't destroy a
// four-note asset by dropping a field.
func TestSeekAnswers_BreakResource_RejectsOmittedMarginaliaOnWrittenAsset(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 2)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	resourceID, _ := saSeedResource(t, h, otherIdx, "annotated ledger", 2)

	h.makeChoice(plan.ID, "make", []string{"break_resource"})

	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-resource"
	code, body := h.post(preparerIdx, breakPath, map[string]any{"asset_id": resourceID})
	require.Equalf(t, http.StatusBadRequest, code,
		"an asset with notes must name the marginalia to tear: %v", body)

	survived, err := h.q.GetAssetByID(ctx, resourceID)
	require.NoError(t, err)
	assert.False(t, survived.IsDestroyed, "the rejected break must not destroy anything")
}

// TestSeekAnswers_ForfeitStep_RejectedWhenOnlyBlankTargetsRemain is the hatch
// half of the widening. Eligibility now counts blank resources, so the forfeit
// must refuse — the preparer has a target they can actually act on, and letting
// them skip would quietly drop a pick the rules owe.
func TestSeekAnswers_ForfeitStep_RejectedWhenOnlyBlankTargetsRemain(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 2)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	saSeedResource(t, h, otherIdx, "unwritten ledger", 0)

	h.makeChoice(plan.ID, "make", []string{"break_resource"})

	forfeitPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/seek-forfeit-step"
	code, body := h.post(preparerIdx, forfeitPath, map[string]any{"step": "break_resource"})
	require.Equalf(t, http.StatusConflict, code,
		"a blank resource is a valid break target, so the forfeit must be refused: %v", body)
}

// ── One shake-up break ───────────────────────────────────────────────────────

// TestShakeUpBreakAsset_DestroysBlankAsset proves the shake-up break spend also
// reaches a blank asset — the path a grandfathered blank from a pre-gate game is
// most likely to be removed through. The spend carries no target_marginalia_id
// because there is none to carry.
func TestShakeUpBreakAsset_DestroysBlankAsset(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	seeded := newShakeUpGame(t, q, 3)
	manager := hub.NewManager()

	owner, spender := seeded.Players[0], seeded.Players[1]
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: seeded.Game.ID, OwnerID: owner.ID, CreatorID: owner.ID,
		AssetType: model.AssetResource, Name: "unwritten cache",
	})
	require.NoError(t, err)

	spend := &dbgen.ShakeUpSpend{
		GameID:        seeded.Game.ID,
		PlayerID:      spender.ID,
		OptionKey:     gamepkg.ShakeUpOptBreakResource,
		TargetAssetID: &asset.ID,
		// TargetMarginaliaID deliberately nil — the asset is blank.
	}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, seeded.Game.ID, spend, 1))

	after, err := q.GetAssetByID(ctx, asset.ID)
	require.NoError(t, err)
	assert.True(t, after.IsDestroyed, "the blank resource must be destroyed by the break spend")

	committed := strings.Join(committedPosts(t, q, seeded.Game.ID), "\n")
	assert.Contains(t, committed, "to break", "the token spend is still logged for the ledger")
	assert.Contains(t, committed, "destroying it", "the ledger line records the destruction")
	assert.Contains(t, allPostBodies(t, q, seeded.Game.ID), "destroyed",
		"the canonical asset.destroyed post is written")
}

// The announce-time validator has to agree with the applier, or a spend that
// commits fine is refused before it is ever created.
func TestValidateShakeUpBreakTarget_AllowsBlankAssetWithoutMarginalia(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	seeded := newShakeUpGame(t, q, 2)

	blank, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: seeded.Game.ID, OwnerID: seeded.Players[0].ID, CreatorID: seeded.Players[0].ID,
		AssetType: model.AssetResource, Name: "unwritten cache",
	})
	require.NoError(t, err)
	written, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: seeded.Game.ID, OwnerID: seeded.Players[0].ID, CreatorID: seeded.Players[0].ID,
		AssetType: model.AssetResource, Name: "annotated cache",
	})
	require.NoError(t, err)
	_, err = q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: written.ID, Position: 1, Text: "a note",
	})
	require.NoError(t, err)

	info, err := gamepkg.ShakeUpOption(gamepkg.ShakeUpOptBreakResource)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	assert.True(t, validateShakeUpBreakTarget(ctx, w, q, info, &blank.ID, nil),
		"a blank target needs no marginalia id")

	w = httptest.NewRecorder()
	assert.False(t, validateShakeUpBreakTarget(ctx, w, q, info, &written.ID, nil),
		"an asset with notes must still name the marginalia to tear")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// allPostBodies concatenates every action-log body in a game, for coarse
// "did this wording land?" assertions.
func allPostBodies(t *testing.T, q *dbgen.Queries, gameID int64) string {
	t.Helper()
	posts, err := q.ListGamePosts(context.Background(), gameID)
	require.NoError(t, err)
	var sb strings.Builder
	for _, p := range posts {
		sb.WriteString(p.Body)
		sb.WriteByte('\n')
	}
	return sb.String()
}
