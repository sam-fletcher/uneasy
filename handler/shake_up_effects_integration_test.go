//go:build integration

// handler/shake_up_effects_integration_test.go — characterization tests for the
// Shake-Up spend *effects* (Step 2 of the testability roadmap, see
// adr/TESTABILITY_AND_ENGINE_DECOUPLING_PLAN.md).
//
// These pin the current behavior of the DB-coupled engine logic that Option E
// will later move out of package handler: the rank-bump swap, asset take/break,
// and category→category→ended progression. They are deliberately
// characterization tests — they assert what the code does today so the E
// refactor can prove it preserved behavior. They lean on the new SeedShakeUp
// fixture, which replaces ~50 lines of hand-rolled setup per case.
//
// The effect functions read plain fields off a *dbgen.ShakeUpSpend and mutate
// the DB directly, so we drive them with constructed spend literals rather than
// the full announce→adjust→commit HTTP flow (token accounting is exercised
// separately by the currentShakeUpActor ordering test).
package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/gametest"
	"uneasy/hub"
	"uneasy/model"
)

// newShakeUpGame seeds a fresh shake_up game with n players via the gametest
// fixture and fresh usernames. Default board: esteem/rolling, zero tokens, all
// three tracks in seat order (player i → rank i+1).
func newShakeUpGame(t *testing.T, q *dbgen.Queries, n int, opts ...gametest.Option) gametest.SeededGame {
	t.Helper()
	usernames := make([]string, n)
	for i := range usernames {
		usernames[i] = fmt.Sprintf("su%d-%s", i+1, randSuffix())
	}
	seeded, err := gametest.SeedShakeUp(context.Background(), q, usernames, opts...)
	require.NoError(t, err)
	return seeded
}

// committedPosts returns the bodies of all shake_up.committed action-log posts
// for the game, in insertion order.
func committedPosts(t *testing.T, q *dbgen.Queries, gameID int64) []string {
	t.Helper()
	posts, err := q.ListGamePosts(context.Background(), gameID)
	require.NoError(t, err)
	var out []string
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "shake_up.committed" {
			out = append(out, p.Body)
		}
	}
	return out
}

// rankOf returns player's rank on the given category.
func rankOf(t *testing.T, q *dbgen.Queries, gameID int64, cat model.RankingCategory, playerID int64) int16 {
	t.Helper()
	rankings, err := q.ListRankingsByGame(context.Background(), gameID)
	require.NoError(t, err)
	for _, r := range rankings {
		if r.Category == cat && r.PlayerID != nil && *r.PlayerID == playerID {
			return r.Rank
		}
	}
	t.Fatalf("no %s rank for player %d", cat, playerID)
	return 0
}

// TestShakeUpBumpRank_SwapsWithDisplaced pins the core ranking mechanic: a
// bump moves the spender up one rank and pushes the player who held that rank
// down into the spender's old slot. bump_knowledge targets the knowledge track.
func TestShakeUpBumpRank_SwapsWithDisplaced(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	// Default knowledge ranks: p0=1, p1=2, p2=3.
	seeded := newShakeUpGame(t, q, 3)
	gameID := seeded.Game.ID

	// p2 (rank 3) bumps knowledge → climbs to rank 2; p1 is displaced to rank 3.
	spend := &dbgen.ShakeUpSpend{
		OptionKey: gamepkg.ShakeUpOptBumpKnowledge,
		PlayerID:  seeded.Players[2].ID,
	}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	assert.EqualValues(t, 2, rankOf(t, q, gameID, model.CategoryKnowledge, seeded.Players[2].ID),
		"spender climbs one rank")
	assert.EqualValues(t, 3, rankOf(t, q, gameID, model.CategoryKnowledge, seeded.Players[1].ID),
		"displaced player drops into the vacated slot")
	assert.EqualValues(t, 1, rankOf(t, q, gameID, model.CategoryKnowledge, seeded.Players[0].ID),
		"untouched player keeps their rank")

	posts := committedPosts(t, q, gameID)
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], "rise to rank 2 on Knowledge")
	assert.Contains(t, posts[0], "displacing")
}

// TestShakeUpBumpRank_TopRankIsNoOp pins that bumping from rank 1 does nothing.
func TestShakeUpBumpRank_TopRankIsNoOp(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 3)
	gameID := seeded.Game.ID

	// p0 is already knowledge rank 1.
	spend := &dbgen.ShakeUpSpend{
		OptionKey: gamepkg.ShakeUpOptBumpKnowledge,
		PlayerID:  seeded.Players[0].ID,
	}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	assert.EqualValues(t, 1, rankOf(t, q, gameID, model.CategoryKnowledge, seeded.Players[0].ID))
	assert.EqualValues(t, 2, rankOf(t, q, gameID, model.CategoryKnowledge, seeded.Players[1].ID))
	assert.EqualValues(t, 3, rankOf(t, q, gameID, model.CategoryKnowledge, seeded.Players[2].ID))

	// Even a no-op bump is logged — the rules dwell on spends that change nothing.
	posts := committedPosts(t, q, gameID)
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], "already at the top")
}

// TestShakeUpEffect_TakeAsset_TransfersOwnership pins that a take_* option
// reassigns the target asset to the spender.
func TestShakeUpEffect_TakeAsset_TransfersOwnership(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 2)
	gameID := seeded.Game.ID

	peer, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: gameID, OwnerID: seeded.Players[1].ID, CreatorID: seeded.Players[1].ID,
		AssetType: model.AssetPeer, Name: "Loyal guard",
	})
	require.NoError(t, err)
	// A secret authored by the current owner — the spender can't see it until the
	// take grants visibility (taking an asset lets you learn its secrets).
	secret, err := q.CreateSecret(ctx, dbgen.CreateSecretParams{
		AssetID: peer.ID, AuthorID: seeded.Players[1].ID, Text: "the guard's true allegiance",
	})
	require.NoError(t, err)

	spend := &dbgen.ShakeUpSpend{
		OptionKey:     gamepkg.ShakeUpOptTakePeer,
		PlayerID:      seeded.Players[0].ID,
		TargetAssetID: &peer.ID,
	}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	got, err := q.GetAssetByID(ctx, peer.ID)
	require.NoError(t, err)
	assert.Equal(t, seeded.Players[0].ID, got.OwnerID, "asset transfers to the spender")
	assert.False(t, got.IsDestroyed)

	// Regression: the spender now reads the secret on the asset they took.
	visible, err := q.ListVisibleSecrets(ctx, dbgen.ListVisibleSecretsParams{
		AssetID: peer.ID, PlayerID: seeded.Players[0].ID,
	})
	require.NoError(t, err)
	require.Len(t, visible, 1, "spender should see the taken asset's secret")
	assert.Equal(t, secret.ID, visible[0].ID)

	posts := committedPosts(t, q, gameID)
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], `to take **Loyal guard** (peer)`)
}

// postsByCode returns the bodies of all action-log posts with the given system
// code, in insertion order.
func postsByCode(t *testing.T, q *dbgen.Queries, gameID int64, code string) []string {
	t.Helper()
	posts, err := q.ListGamePosts(context.Background(), gameID)
	require.NoError(t, err)
	var out []string
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == code {
			out = append(out, p.Body)
		}
	}
	return out
}

// TestShakeUpEffect_BreakAsset_TearsOneMarginalia pins the canonical break: a
// break_* option tears ONE marginalia (not the whole asset), leaving a
// multi-marginalia asset intact, revealing its secrets to the breaker, and
// writing the standard marginalia.torn log entry.
func TestShakeUpEffect_BreakAsset_TearsOneMarginalia(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 2)
	gameID := seeded.Game.ID

	res, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: gameID, OwnerID: seeded.Players[1].ID, CreatorID: seeded.Players[1].ID,
		AssetType: model.AssetResource, Name: "Granary",
	})
	require.NoError(t, err)
	m1, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: res.ID, Position: 1, Text: "well-stocked",
	})
	require.NoError(t, err)
	_, err = q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: res.ID, Position: 2, Text: "guarded",
	})
	require.NoError(t, err)
	// A secret the breaker can't see until the tear grants visibility.
	secret, err := q.CreateSecret(ctx, dbgen.CreateSecretParams{
		AssetID: res.ID, AuthorID: seeded.Players[1].ID, Text: "the granary is nearly empty",
	})
	require.NoError(t, err)

	spend := &dbgen.ShakeUpSpend{
		OptionKey:          gamepkg.ShakeUpOptBreakResource,
		PlayerID:           seeded.Players[0].ID,
		TargetAssetID:      &res.ID,
		TargetMarginaliaID: &m1.ID,
	}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	got, err := q.GetAssetByID(ctx, res.ID)
	require.NoError(t, err)
	assert.False(t, got.IsDestroyed, "tearing one of two marginalia must not destroy the asset")

	torn, err := q.GetMarginaliaByID(ctx, m1.ID)
	require.NoError(t, err)
	assert.True(t, torn.IsTorn, "the chosen marginalia is torn")

	// Regression: the breaker now reads the asset's secret (break grants sight).
	visible, err := q.ListVisibleSecrets(ctx, dbgen.ListVisibleSecretsParams{
		AssetID: res.ID, PlayerID: seeded.Players[0].ID,
	})
	require.NoError(t, err)
	require.Len(t, visible, 1, "breaker should see the broken asset's secret")
	assert.Equal(t, secret.ID, visible[0].ID)

	// Canonical marginalia.torn entry with the owner re-describe prompt.
	torns := postsByCode(t, q, gameID, "marginalia.torn")
	require.Len(t, torns, 1)
	assert.Contains(t, torns[0], "how has the asset changed?")

	// The shake-up ledger line still records the token spend; the asset survived,
	// so it does not say "destroying it".
	posts := committedPosts(t, q, gameID)
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], `to break`)
	assert.Contains(t, posts[0], `**Granary** (resource)`)
	assert.NotContains(t, posts[0], "destroying it")
}

// TestShakeUpEffect_BreakAsset_DestroysOnLastMarginalia pins that tearing the
// asset's final intact marginalia destroys it (4-of-4 gone → destroyed).
func TestShakeUpEffect_BreakAsset_DestroysOnLastMarginalia(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 2)
	gameID := seeded.Game.ID

	res, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: gameID, OwnerID: seeded.Players[1].ID, CreatorID: seeded.Players[1].ID,
		AssetType: model.AssetResource, Name: "Granary",
	})
	require.NoError(t, err)
	// Only one intact marginalia: tearing it removes the last one.
	m, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: res.ID, Position: 1, Text: "last plank",
	})
	require.NoError(t, err)

	spend := &dbgen.ShakeUpSpend{
		OptionKey:          gamepkg.ShakeUpOptBreakResource,
		PlayerID:           seeded.Players[0].ID,
		TargetAssetID:      &res.ID,
		TargetMarginaliaID: &m.ID,
	}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	got, err := q.GetAssetByID(ctx, res.ID)
	require.NoError(t, err)
	assert.True(t, got.IsDestroyed, "tearing the last marginalia destroys the asset")

	posts := committedPosts(t, q, gameID)
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], `to break`)
	assert.Contains(t, posts[0], `**Granary** (resource)`)
	assert.Contains(t, posts[0], "destroying it")
}

// TestShakeUpEffect_BreakAsset_RequiresMarginalia pins the authoritative
// apply-time guard: a break spend with no marginalia chosen is rejected (and the
// asset is left untouched) rather than silently destroying the whole asset.
func TestShakeUpEffect_BreakAsset_RequiresMarginalia(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 2)
	gameID := seeded.Game.ID

	res, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: gameID, OwnerID: seeded.Players[1].ID, CreatorID: seeded.Players[1].ID,
		AssetType: model.AssetResource, Name: "Granary",
	})
	require.NoError(t, err)
	_, err = q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: res.ID, Position: 1, Text: "intact",
	})
	require.NoError(t, err)

	spend := &dbgen.ShakeUpSpend{
		OptionKey:     gamepkg.ShakeUpOptBreakResource,
		PlayerID:      seeded.Players[0].ID,
		TargetAssetID: &res.ID,
		// TargetMarginaliaID deliberately nil.
	}
	err = applyShakeUpEffect(ctx, q, manager, gameID, spend, 1)
	require.Error(t, err, "break without a marginalia target must be rejected")

	got, err := q.GetAssetByID(ctx, res.ID)
	require.NoError(t, err)
	assert.False(t, got.IsDestroyed, "the asset must be untouched when the break is rejected")
}

// titleMarginaliaCount counts intact title-bearing marginalia on an asset.
func marginaliaCountOf(t *testing.T, q *dbgen.Queries, assetID int64) int {
	t.Helper()
	ms, err := q.ListMarginaliaByAsset(context.Background(), assetID)
	require.NoError(t, err)
	return len(ms)
}

// TestShakeUpEffect_ClaimTitle_Monarch pins the ADR-007 Shake-Up claim-title
// rework: claiming the monarch title stamps a marginalia (no artifact) on the
// claimer's own peer, trips throne_established, and makes currentMonarch resolve
// to the claimer — exactly like a Prologue monarch claim.
func TestShakeUpEffect_ClaimTitle_Monarch(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 3)
	gameID := seeded.Game.ID
	claimer := seeded.Players[1]

	mcID, err := findMainCharacter(ctx, q, claimer.ID)
	require.NoError(t, err)
	assetsBefore, err := q.ListAssetsByGame(ctx, gameID)
	require.NoError(t, err)

	titleID := gamepkg.TitleMonarch
	flavor := "crowned at the Midwinter Accord"
	spend := &dbgen.ShakeUpSpend{
		OptionKey:     gamepkg.ShakeUpOptClaimTitle,
		PlayerID:      claimer.ID,
		TargetAssetID: &mcID,
		TargetTitleID: &titleID,
		TitleFlavor:   &flavor,
	}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	// No artifact (or any asset) was created — the role lives on the marginalia.
	assetsAfter, err := q.ListAssetsByGame(ctx, gameID)
	require.NoError(t, err)
	assert.Len(t, assetsAfter, len(assetsBefore), "claim-title must not create an asset")

	// The title marginalia is stamped on the claimer's peer with the right id.
	ms, err := q.ListMarginaliaByAsset(ctx, mcID)
	require.NoError(t, err)
	require.Len(t, ms, 1)
	require.NotNil(t, ms[0].Title)
	assert.Equal(t, gamepkg.TitleMonarch, *ms[0].Title)
	assert.Equal(t, flavor, ms[0].Text, "flavor text becomes the marginalia text")

	// The throne gate tripped and currentMonarch resolves to the claimer.
	g, err := q.GetGameByID(ctx, gameID)
	require.NoError(t, err)
	assert.True(t, g.ThroneEstablished)
	gotAsset, gotOwner, ok, err := currentMonarch(ctx, q, gameID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, mcID, gotAsset)
	assert.Equal(t, claimer.ID, gotOwner)

	posts := committedPosts(t, q, gameID)
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], "claim the title **The Monarch**")
}

// TestShakeUpEffect_ClaimTitle_NonMonarchNoGate pins that claiming a title
// outside the line of succession stamps the marginalia but does NOT establish
// the throne (no monarch role appears).
func TestShakeUpEffect_ClaimTitle_NonMonarchNoGate(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 2)
	gameID := seeded.Game.ID
	claimer := seeded.Players[0]
	mcID, err := findMainCharacter(ctx, q, claimer.ID)
	require.NoError(t, err)

	titleID := gamepkg.TitleSpymaster
	spend := &dbgen.ShakeUpSpend{
		OptionKey:     gamepkg.ShakeUpOptClaimTitle,
		PlayerID:      claimer.ID,
		TargetAssetID: &mcID,
		TargetTitleID: &titleID,
	}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	ms, err := q.ListMarginaliaByAsset(ctx, mcID)
	require.NoError(t, err)
	require.Len(t, ms, 1)
	require.NotNil(t, ms[0].Title)
	assert.Equal(t, gamepkg.TitleSpymaster, *ms[0].Title)
	// No flavor → marginalia text defaults to the title's display name.
	assert.Equal(t, "The Spymaster", ms[0].Text)

	g, err := q.GetGameByID(ctx, gameID)
	require.NoError(t, err)
	assert.False(t, g.ThroneEstablished, "a non-line title must not establish the throne")
	_, _, ok, err := currentMonarch(ctx, q, gameID)
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestShakeUpEffect_ClaimTitle_AlreadyClaimedRejected pins game-wide title
// uniqueness: a title already claimed elsewhere (even on another peer) can't be
// re-minted, and the target peer is left untouched.
func TestShakeUpEffect_ClaimTitle_AlreadyClaimedRejected(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 2)
	gameID := seeded.Game.ID

	// Player 0 already holds the monarch title (claimed earlier, on their MC).
	p0mc, err := findMainCharacter(ctx, q, seeded.Players[0].ID)
	require.NoError(t, err)
	existingTitle := gamepkg.TitleMonarch
	_, err = q.CreateTitleMarginalia(ctx, dbgen.CreateTitleMarginaliaParams{
		AssetID: p0mc, Position: 1, Text: "The Monarch", Title: &existingTitle,
	})
	require.NoError(t, err)

	// Player 1 tries to claim the monarch title too → rejected.
	p1mc, err := findMainCharacter(ctx, q, seeded.Players[1].ID)
	require.NoError(t, err)
	titleID := gamepkg.TitleMonarch
	spend := &dbgen.ShakeUpSpend{
		OptionKey:     gamepkg.ShakeUpOptClaimTitle,
		PlayerID:      seeded.Players[1].ID,
		TargetAssetID: &p1mc,
		TargetTitleID: &titleID,
	}
	err = applyShakeUpEffect(ctx, q, manager, gameID, spend, 1)
	require.Error(t, err, "claiming an already-claimed title must be rejected")
	assert.Equal(t, 0, marginaliaCountOf(t, q, p1mc), "the target peer must be left untouched")
}

// TestShakeUpBumpRank_SkipsDummy pins the dummy-skip fix: a bump climbs PAST a
// dummy slot and swaps with the first real player above, leaving the dummy in
// place (matching the engrailed ranking update). Previously the bump swapped
// straight into the dummy's slot, corrupting the track.
func TestShakeUpBumpRank_SkipsDummy(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 3)
	gameID := seeded.Game.ID

	// Lay out the esteem track with a dummy between two real players:
	//   rank1 = p1 (real), rank2 = dummy, rank3 = p0 (the bumper).
	setEsteem := func(rank int16, pid *int64) {
		require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
			GameID: gameID, PlayerID: pid, Category: model.CategoryEsteem, Rank: rank,
		}))
	}
	p0, p1, p2 := seeded.Players[0].ID, seeded.Players[1].ID, seeded.Players[2].ID
	setEsteem(1, &p1)
	setEsteem(2, nil) // dummy
	setEsteem(3, &p0)
	setEsteem(4, &p2)

	spend := &dbgen.ShakeUpSpend{OptionKey: gamepkg.ShakeUpOptBumpEsteem, PlayerID: p0}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	assert.EqualValues(t, 1, rankOf(t, q, gameID, model.CategoryEsteem, p0), "bumper climbs past the dummy to rank 1")
	assert.EqualValues(t, 3, rankOf(t, q, gameID, model.CategoryEsteem, p1), "overtaken real player drops to the bumper's old slot")

	// The dummy slot is untouched (still nil at rank 2).
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	require.NoError(t, err)
	for _, rk := range rankings {
		if rk.Category == model.CategoryEsteem && rk.Rank == 2 {
			assert.Nil(t, rk.PlayerID, "the dummy slot stays a dummy")
		}
	}

	posts := committedPosts(t, q, gameID)
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], "rise to rank 1")
}

// TestShakeUpBumpRank_DummyAboveIsNoOp pins that a real player with only a dummy
// (or nothing) above is effectively at the top: the bump is a logged no-op and
// must NOT swap them into the dummy's slot.
func TestShakeUpBumpRank_DummyAboveIsNoOp(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 3)
	gameID := seeded.Game.ID

	// rank1 = dummy, rank2 = p0 (bumper, top real player), rank3 = p1.
	setEsteem := func(rank int16, pid *int64) {
		require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
			GameID: gameID, PlayerID: pid, Category: model.CategoryEsteem, Rank: rank,
		}))
	}
	p0, p1, p2 := seeded.Players[0].ID, seeded.Players[1].ID, seeded.Players[2].ID
	setEsteem(1, nil) // dummy
	setEsteem(2, &p0)
	setEsteem(3, &p1)
	setEsteem(4, &p2)

	spend := &dbgen.ShakeUpSpend{OptionKey: gamepkg.ShakeUpOptBumpEsteem, PlayerID: p0}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	assert.EqualValues(t, 2, rankOf(t, q, gameID, model.CategoryEsteem, p0), "top real player must not climb into the dummy slot")
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	require.NoError(t, err)
	for _, rk := range rankings {
		if rk.Category == model.CategoryEsteem && rk.Rank == 1 {
			assert.Nil(t, rk.PlayerID, "the dummy slot stays a dummy")
		}
	}

	posts := committedPosts(t, q, gameID)
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], "already at the top")
}

// TestMaybeAdvanceShakeUpCategory_Progression pins the category machine: with
// every pool empty, the spending step advances esteem → knowledge → power →
// ended.
func TestMaybeAdvanceShakeUpCategory_Progression(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	// Seed at the spending step with zero tokens (every pool already empty).
	seeded := newShakeUpGame(t, q, 2, gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending))
	gameID := seeded.Game.ID

	requireCategory := func(wantCat string, wantStep int16) {
		g, err := q.GetGameByID(ctx, gameID)
		require.NoError(t, err)
		require.NotNil(t, g.ShakeUpCategory)
		require.NotNil(t, g.ShakeUpStep)
		assert.Equal(t, wantCat, *g.ShakeUpCategory)
		assert.Equal(t, wantStep, *g.ShakeUpStep)
	}

	requireCategory(gamepkg.ShakeUpCategoryEsteem, gamepkg.ShakeUpStepSpending)

	// esteem → knowledge (back to the rolling step).
	require.NoError(t, maybeAdvanceShakeUpCategory(ctx, q, manager, gameID))
	requireCategory(gamepkg.ShakeUpCategoryKnowledge, gamepkg.ShakeUpStepRolling)

	// knowledge → power.
	require.NoError(t, maybeAdvanceShakeUpCategory(ctx, q, manager, gameID))
	requireCategory(gamepkg.ShakeUpCategoryPower, gamepkg.ShakeUpStepRolling)

	// power → ended.
	require.NoError(t, maybeAdvanceShakeUpCategory(ctx, q, manager, gameID))
	g, err := q.GetGameByID(ctx, gameID)
	require.NoError(t, err)
	assert.Equal(t, model.PhaseEnded, g.Phase, "game ends after the power category")
}
