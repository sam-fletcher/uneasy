//go:build integration

// handler/monarch_integration_test.go — coverage for currentMonarch, the
// computed monarch role / line of succession (ADR-007 Phase A). Exercises the
// throne_established gate, the SUCCESSION_ORDER pick, deposition cascade,
// interregnum, the is_destroyed exclusion, and title hoarding.

package handler

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
)

// establishThrone trips the game-level gate, simulating a claimed monarch
// title (Phase B will do this at claim time; there is no query for it yet).
func establishThrone(t *testing.T, pool *pgxpool.Pool, gameID int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE games SET throne_established = TRUE WHERE id = $1`, gameID)
	require.NoError(t, err)
}

// seedTitledAsset creates a peer asset owned by ownerID bearing a single
// marginalia stamped with titleID, and returns the asset and marginalia ids.
// The raw title UPDATE stands in for Phase B's claim-time stamping.
func seedTitledAsset(
	t *testing.T, q *dbgen.Queries, pool *pgxpool.Pool,
	gameID, ownerID int64, name, titleID string,
) (assetID, marginaliaID int64) {
	t.Helper()
	ctx := context.Background()
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: gameID, OwnerID: ownerID, CreatorID: ownerID,
		AssetType: "peer", Name: name,
	})
	require.NoError(t, err)
	mID := stampTitleMarginalia(t, q, pool, asset.ID, 1, titleID)
	return asset.ID, mID
}

// stampTitleMarginalia adds a title-bearing marginalia at the given position
// and returns its id.
func stampTitleMarginalia(
	t *testing.T, q *dbgen.Queries, pool *pgxpool.Pool,
	assetID int64, position int16, titleID string,
) int64 {
	t.Helper()
	ctx := context.Background()
	m, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: assetID, Position: position, Text: "the " + titleID,
	})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE marginalia SET title = $1 WHERE id = $2`, titleID, m.ID)
	require.NoError(t, err)
	return m.ID
}

// TestCurrentMonarch_GateOff: a monarch title exists but the throne was never
// established → no monarch (a lone claim is a powerless pretender).
func TestCurrentMonarch_GateOff(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	seedTitledAsset(t, q, pool, tg.Game.ID, tg.Players[0].ID, "Crowned", game.TitleMonarch)

	_, _, ok, err := currentMonarch(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.False(t, ok, "no monarch while throne_established is false")
}

// TestCurrentMonarch_SingleMonarch: gate on, one monarch title → its owner.
func TestCurrentMonarch_SingleMonarch(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()
	establishThrone(t, pool, tg.Game.ID)

	assetID, _ := seedTitledAsset(t, q, pool, tg.Game.ID, tg.Players[0].ID, "Crowned", game.TitleMonarch)

	gotAsset, gotOwner, ok, err := currentMonarch(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, assetID, gotAsset)
	assert.Equal(t, tg.Players[0].ID, gotOwner)
}

// TestCurrentMonarch_DepositionCascade: monarch + true_heir; tearing the
// monarch's title marginalia promotes the true heir's owner.
func TestCurrentMonarch_DepositionCascade(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()
	establishThrone(t, pool, tg.Game.ID)

	_, monarchMarg := seedTitledAsset(t, q, pool, tg.Game.ID, tg.Players[0].ID, "King", game.TitleMonarch)
	heirAsset, _ := seedTitledAsset(t, q, pool, tg.Game.ID, tg.Players[1].ID, "Heir", game.TitleTrueHeir)

	// Monarch reigns first.
	_, gotOwner, ok, err := currentMonarch(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, tg.Players[0].ID, gotOwner)

	// Depose: tear the monarch title marginalia.
	n, err := q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{ID: monarchMarg, TornByID: &tg.Players[1].ID})
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	gotAsset, gotOwner, ok, err := currentMonarch(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, heirAsset, gotAsset, "true heir ascends")
	assert.Equal(t, tg.Players[1].ID, gotOwner)
}

// TestCurrentMonarch_Interregnum: every throne-line claim torn → no monarch,
// even though the throne was established.
func TestCurrentMonarch_Interregnum(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()
	establishThrone(t, pool, tg.Game.ID)

	_, monarchMarg := seedTitledAsset(t, q, pool, tg.Game.ID, tg.Players[0].ID, "King", game.TitleMonarch)
	_, heirMarg := seedTitledAsset(t, q, pool, tg.Game.ID, tg.Players[1].ID, "Heir", game.TitleTrueHeir)

	for _, m := range []int64{monarchMarg, heirMarg} {
		_, err := q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{ID: m, TornByID: &tg.Players[0].ID})
		require.NoError(t, err)
	}

	_, _, ok, err := currentMonarch(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.False(t, ok, "interregnum: throne vacant, falls back to power")
}

// TestCurrentMonarch_DestroyedUntornExcluded: the monarch's asset is destroyed
// while its title marginalia is still un-torn (the DestroyAsset hazard). The
// monarch must be excluded and the true heir ascend — the is_destroyed filter,
// not is_torn, is what saves us here.
func TestCurrentMonarch_DestroyedUntornExcluded(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()
	establishThrone(t, pool, tg.Game.ID)

	monarchAsset, monarchMarg := seedTitledAsset(t, q, pool, tg.Game.ID, tg.Players[0].ID, "King", game.TitleMonarch)
	heirAsset, _ := seedTitledAsset(t, q, pool, tg.Game.ID, tg.Players[1].ID, "Heir", game.TitleTrueHeir)

	// Destroy the asset directly WITHOUT tearing its title marginalia.
	require.NoError(t, q.DestroyAsset(ctx, monarchAsset))
	m, err := q.GetMarginaliaByID(ctx, monarchMarg)
	require.NoError(t, err)
	require.False(t, m.IsTorn, "precondition: title marginalia is un-torn")

	gotAsset, gotOwner, ok, err := currentMonarch(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, heirAsset, gotAsset, "ghost monarch excluded; true heir reigns")
	assert.Equal(t, tg.Players[1].ID, gotOwner)
}

// TestCurrentMonarch_DestroyedUntornOnlyClaim: same hazard with no heir behind
// it → no monarch at all (not a crowned ghost).
func TestCurrentMonarch_DestroyedUntornOnlyClaim(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()
	establishThrone(t, pool, tg.Game.ID)

	monarchAsset, _ := seedTitledAsset(t, q, pool, tg.Game.ID, tg.Players[0].ID, "King", game.TitleMonarch)
	require.NoError(t, q.DestroyAsset(ctx, monarchAsset))

	_, _, ok, err := currentMonarch(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.False(t, ok, "destroyed-but-untorn sole claim must not crown a ghost")
}

// TestCurrentMonarch_HoardingPicksHighest: one asset bearing several throne-line
// titles is reigned by its highest claim (monarch beats true_heir).
func TestCurrentMonarch_HoardingPicksHighest(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()
	establishThrone(t, pool, tg.Game.ID)

	// Player 0 hoards true_heir + monarch on a single character.
	hoarder, _ := seedTitledAsset(t, q, pool, tg.Game.ID, tg.Players[0].ID, "Hoarder", game.TitleTrueHeir)
	stampTitleMarginalia(t, q, pool, hoarder, 2, game.TitleMonarch)
	// Another player holds a lower claim.
	seedTitledAsset(t, q, pool, tg.Game.ID, tg.Players[1].ID, "Pretender", game.TitleClaimant)

	gotAsset, gotOwner, ok, err := currentMonarch(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, hoarder, gotAsset, "highest title on the hoarder wins")
	assert.Equal(t, tg.Players[0].ID, gotOwner)
}
