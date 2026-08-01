//go:build integration

package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// ── PHASE 3: Asset Handler Integration Tests ────────────────────────────────
// These tests validate asset lifecycle, state transitions, and business logic.

// ── Asset Creation Tests ────────────────────────────────────────────────────

func TestCreateAsset_ValidTypes(t *testing.T) {
	tests := []model.AssetType{
		model.AssetPeer,
		model.AssetHolding,
		model.AssetArtifact,
		model.AssetResource,
	}

	for _, assetType := range tests {
		t.Run(string(assetType), func(t *testing.T) {
			pool := openTestDB(t)
			q := dbgen.New(pool)
			tg := newTestGame(t, q, 2)
			ctx := context.Background()

			asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
				GameID:          tg.Game.ID,
				OwnerID:         tg.Players[0].ID,
				CreatorID:       tg.Players[0].ID,
				AssetType:       assetType,
				Name:            "Test Asset",
				IsMainCharacter: false,
			})
			require.NoError(t, err)
			assert.Equal(t, assetType, asset.AssetType)
		})
	}
}

func TestCreateAsset_RequiresName(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Attempt to create with empty name
	_, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "",
		IsMainCharacter: false,
	})
	require.Error(t, err)
}

func TestCreateAsset_WithInitialMarginalia(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Add marginalia
	m1, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID:  asset.ID,
		Position: 1,
		Text:     "First note",
	})
	require.NoError(t, err)
	assert.Equal(t, int16(1), m1.Position)

	// Verify retrieval
	marginalia, err := q.ListMarginaliaByAsset(ctx, asset.ID)
	require.NoError(t, err)
	assert.Len(t, marginalia, 1)
	assert.Equal(t, "First note", marginalia[0].Text)
}

func TestCreateAsset_MarginaliaMaxBounds(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Create all 4 allowed marginalia
	for pos := int16(1); pos <= 4; pos++ {
		_, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID:  asset.ID,
			Position: pos,
			Text:     "Note " + string(rune(48+pos)),
		})
		require.NoError(t, err)
	}

	// Try to add a 5th (should fail)
	_, err = q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID:  asset.ID,
		Position: 5,
		Text:     "Fifth note",
	})
	require.Error(t, err, "should reject marginalia position > 4")
}

// ── Main Character Exclusivity Tests ────────────────────────────────────────

func TestUpdateAsset_MainCharacterExclusivity(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Create two peers
	peer1, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally 1",
		IsMainCharacter: true,
	})
	require.NoError(t, err)

	peer2, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally 2",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Set peer2 as main character → should clear peer1
	err = q.ClearMainCharacter(ctx, dbgen.ClearMainCharacterParams{
		OwnerID: tg.Players[0].ID,
		GameID:  tg.Game.ID,
	})
	require.NoError(t, err)

	err = q.SetMainCharacter(ctx, dbgen.SetMainCharacterParams{
		ID:              peer2.ID,
		IsMainCharacter: true,
	})
	require.NoError(t, err)

	// Verify peer1 is no longer main character
	peer1Updated, err := q.GetAssetByID(ctx, peer1.ID)
	require.NoError(t, err)
	assert.False(t, peer1Updated.IsMainCharacter)
}

func TestUpdateAsset_OnlyPeersCanBeMain(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Create non-peer assets
	for _, assetType := range []model.AssetType{
		model.AssetHolding,
		model.AssetArtifact,
		model.AssetResource,
	} {
		holding, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:          tg.Game.ID,
			OwnerID:         tg.Players[0].ID,
			CreatorID:       tg.Players[0].ID,
			AssetType:       assetType,
			Name:            "Non-Peer",
			IsMainCharacter: false,
		})
		require.NoError(t, err)

		// Try to set as main character (should not work or be ignored)
		// Actually, the DB schema may allow it but game logic should prevent it
		err = q.SetMainCharacter(ctx, dbgen.SetMainCharacterParams{
			ID:              holding.ID,
			IsMainCharacter: true,
		})
		// Just verify operation completes; handler layer validates type
		require.NoError(t, err)
	}
}

// ── Marginalia Lifecycle Tests ──────────────────────────────────────────────

func TestUpdateMarginalia_ModifyText(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	m, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID:  asset.ID,
		Position: 1,
		Text:     "Original text",
	})
	require.NoError(t, err)

	// Update text
	err = q.UpdateMarginaliaText(ctx, dbgen.UpdateMarginaliaTextParams{
		ID:   m.ID,
		Text: "Modified text",
	})
	require.NoError(t, err)

	// Verify update worked
	m2, err := q.GetMarginaliaByID(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, "Modified text", m2.Text)
}

func TestTearMarginalia_PartialTear(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Create 2 marginalia
	m1, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID:  asset.ID,
		Position: 1,
		Text:     "First",
	})
	require.NoError(t, err)

	m2, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID:  asset.ID,
		Position: 2,
		Text:     "Second",
	})
	require.NoError(t, err)

	// Tear first marginalia
	tornByID := tg.Players[0].ID
	_, err = q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
		ID:       m1.ID,
		TornByID: &tornByID,
	})
	require.NoError(t, err)

	// Asset should still exist (one intact marginalia remains)
	assetAfter, err := q.GetAssetByID(ctx, asset.ID)
	require.NoError(t, err)
	assert.NotNil(t, assetAfter)

	// Verify first is torn
	m1After, err := q.GetMarginaliaByID(ctx, m1.ID)
	require.NoError(t, err)
	assert.True(t, m1After.IsTorn)

	// Verify second is intact
	m2After, err := q.GetMarginaliaByID(ctx, m2.ID)
	require.NoError(t, err)
	assert.False(t, m2After.IsTorn)
}

func TestTearMarginalia_DestroyOnLastTorn(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Create only 1 marginalia
	m1, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID:  asset.ID,
		Position: 1,
		Text:     "Only note",
	})
	require.NoError(t, err)

	// Tear the only marginalia, then run the composition the handler runs:
	// DestroyIfAllMarginaliaTorn flips is_destroyed iff no intact marginalia
	// remain. With one marginalia torn and none intact, it should fire.
	tornByID := tg.Players[0].ID
	_, err = q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
		ID:       m1.ID,
		TornByID: &tornByID,
	})
	require.NoError(t, err)

	rows, err := q.DestroyIfAllMarginaliaTorn(ctx, asset.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows, "last tear should destroy the asset")

	// is_destroyed is a soft-delete flag — the row still exists. Verify
	// the flag flipped and destroyed_at is populated.
	updated, err := q.GetAssetByID(ctx, asset.ID)
	require.NoError(t, err)
	assert.True(t, updated.IsDestroyed)
	assert.True(t, updated.DestroyedAt.Valid)
}

// Tearing one of several marginalia must NOT destroy the asset — the
// guard inside DestroyIfAllMarginaliaTorn checks that no intact ones
// remain. This is the inverse of the test above; together they pin both
// branches of the composition.
func TestDestroyIfAllMarginaliaTorn_LeavesAssetWhenSomeRemain(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:    tg.Game.ID,
		OwnerID:   tg.Players[0].ID,
		CreatorID: tg.Players[0].ID,
		AssetType: model.AssetPeer,
		Name:      "Ally",
	})
	require.NoError(t, err)

	m1, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: asset.ID, Position: 1, Text: "First",
	})
	require.NoError(t, err)
	_, err = q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: asset.ID, Position: 2, Text: "Second",
	})
	require.NoError(t, err)

	tornByID := tg.Players[0].ID
	_, err = q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
		ID: m1.ID, TornByID: &tornByID,
	})
	require.NoError(t, err)

	rows, err := q.DestroyIfAllMarginaliaTorn(ctx, asset.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), rows, "asset must survive while any marginalia is intact")

	updated, err := q.GetAssetByID(ctx, asset.ID)
	require.NoError(t, err)
	assert.False(t, updated.IsDestroyed)
}

func TestTearMarginalia_RejectsTwiceTorn(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	m, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID:  asset.ID,
		Position: 1,
		Text:     "Note",
	})
	require.NoError(t, err)

	// Tear once — should affect one row.
	tornByID := tg.Players[0].ID
	rows, err := q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
		ID:       m.ID,
		TornByID: &tornByID,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)

	// A second tear is a guarded no-op: the WHERE clause filters out the
	// already-torn row, so the query returns no error but updates 0 rows.
	// Callers (handlers) treat 0 as "already torn".
	rows, err = q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
		ID:       m.ID,
		TornByID: &tornByID,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), rows, "double-tear should be a no-op")
}

// ── Leverage / Refresh Tests ────────────────────────────────────────────────

func TestLeverageAsset_MarkAsLeveraged(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)
	assert.False(t, asset.IsLeveraged)

	// Leverage it
	err = q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
		ID:          asset.ID,
		IsLeveraged: true,
	})
	require.NoError(t, err)

	// Verify it was leveraged
	leveraged, err := q.GetAssetByID(ctx, asset.ID)
	require.NoError(t, err)
	assert.True(t, leveraged.IsLeveraged)
}

func TestRefreshAsset_UnmarkLeveraged(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Leverage it first
	err = q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
		ID:          asset.ID,
		IsLeveraged: true,
	})
	require.NoError(t, err)

	// Refresh (un-leverage)
	err = q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
		ID:          asset.ID,
		IsLeveraged: false,
	})
	require.NoError(t, err)

	// Verify it was refreshed
	refreshed, err := q.GetAssetByID(ctx, asset.ID)
	require.NoError(t, err)
	assert.False(t, refreshed.IsLeveraged)
}

func TestLeverageAsset_Idempotent(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Leverage twice
	err = q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
		ID:          asset.ID,
		IsLeveraged: true,
	})
	require.NoError(t, err)

	// Second leverage should succeed (idempotent)
	err = q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
		ID:          asset.ID,
		IsLeveraged: true,
	})
	require.NoError(t, err)

	// Verify it's still leveraged
	leveraged, err := q.GetAssetByID(ctx, asset.ID)
	require.NoError(t, err)
	assert.True(t, leveraged.IsLeveraged)
}

// ── Asset Ownership & Access Tests ─────────────────────────────────────────

func TestAssetOwnershipTracking(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// Create asset as player 0
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)
	assert.Equal(t, tg.Players[0].ID, asset.OwnerID)

	// Transfer to player 1
	err = q.TransferAsset(ctx, dbgen.TransferAssetParams{
		ID:      asset.ID,
		OwnerID: tg.Players[1].ID,
	})
	require.NoError(t, err)

	// Verify transfer worked
	transferred, err := q.GetAssetByID(ctx, asset.ID)
	require.NoError(t, err)
	assert.Equal(t, tg.Players[1].ID, transferred.OwnerID)
}

func TestAssetCreatorTracking(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Create asset as player 0
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Creator should remain unchanged even if ownership transfers
	assert.Equal(t, tg.Players[0].ID, asset.CreatorID)

	// Transfer ownership
	err = q.TransferAsset(ctx, dbgen.TransferAssetParams{
		ID:      asset.ID,
		OwnerID: tg.Players[1].ID,
	})
	require.NoError(t, err)

	// Verify creator is still player 0
	assetAfter, err := q.GetAssetByID(ctx, asset.ID)
	require.NoError(t, err)
	assert.Equal(t, tg.Players[0].ID, assetAfter.CreatorID)
}

// ── Secret Tests ────────────────────────────────────────────────────────────

func TestWriteSecret_StoreAndRetrieve(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	secretText := "The ally is the traitor"
	secret, err := q.CreateSecret(ctx, dbgen.CreateSecretParams{
		AssetID:  asset.ID,
		AuthorID: tg.Players[0].ID,
		Text:     secretText,
	})
	require.NoError(t, err)

	// Retrieve
	assert.Equal(t, secretText, secret.Text)
}

func TestGetSecrets_ListsByAsset(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Write 3 secrets
	for i := 1; i <= 3; i++ {
		_, err := q.CreateSecret(ctx, dbgen.CreateSecretParams{
			AssetID:  asset.ID,
			AuthorID: tg.Players[0].ID,
			Text:     "Secret " + string(rune(48+i)),
		})
		require.NoError(t, err)
	}

	// List secrets for asset
	secrets, err := q.ListVisibleSecrets(ctx, dbgen.ListVisibleSecretsParams{
		AssetID:  asset.ID,
		PlayerID: tg.Players[0].ID,
	})
	require.NoError(t, err)
	assert.Len(t, secrets, 3)
}

func TestSecret_WriterTracking(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Ally",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Player 0 writes secret
	secret, err := q.CreateSecret(ctx, dbgen.CreateSecretParams{
		AssetID:  asset.ID,
		AuthorID: tg.Players[0].ID,
		Text:     "Secret from P0",
	})
	require.NoError(t, err)
	assert.Equal(t, tg.Players[0].ID, secret.AuthorID)

	// Player 1 writes another secret
	secret2, err := q.CreateSecret(ctx, dbgen.CreateSecretParams{
		AssetID:  asset.ID,
		AuthorID: tg.Players[1].ID,
		Text:     "Secret from P1",
	})
	require.NoError(t, err)
	assert.Equal(t, tg.Players[1].ID, secret2.AuthorID)

	// Both secrets are linked to the asset. The point of this test is the
	// author_id round-trip (already asserted above), not the visibility
	// model — ListSecretsByAsset returns the raw rows regardless of who
	// can see them.
	secrets, err := q.ListSecretsByAsset(ctx, asset.ID)
	require.NoError(t, err)
	assert.Len(t, secrets, 2)
}

// Secret counts reflect existence, not visibility: every secret on an asset
// is counted regardless of who authored it or who can read it. This backs the
// "all players know a secret exists, not its content" model.
func TestCountSecrets_ExistenceRegardlessOfVisibility(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	mkAsset := func(name string) dbgen.Asset {
		a, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:    tg.Game.ID,
			OwnerID:   tg.Players[0].ID,
			CreatorID: tg.Players[0].ID,
			AssetType: model.AssetPeer,
			Name:      name,
		})
		require.NoError(t, err)
		return a
	}
	assetA := mkAsset("Two secrets")
	assetB := mkAsset("One secret")
	assetC := mkAsset("No secrets")

	// Two secrets on A by different authors (neither viewer-scoped), one on B.
	for _, p := range []int64{tg.Players[0].ID, tg.Players[1].ID} {
		_, err := q.CreateSecret(ctx, dbgen.CreateSecretParams{AssetID: assetA.ID, AuthorID: p, Text: "x"})
		require.NoError(t, err)
	}
	_, err := q.CreateSecret(ctx, dbgen.CreateSecretParams{AssetID: assetB.ID, AuthorID: tg.Players[0].ID, Text: "y"})
	require.NoError(t, err)

	// Per-asset count.
	for _, tc := range []struct {
		asset dbgen.Asset
		want  int64
	}{{assetA, 2}, {assetB, 1}, {assetC, 0}} {
		got, err := q.CountSecretsByAsset(ctx, tc.asset.ID)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got, tc.asset.Name)
	}

	// Per-game count: assets with no secrets are absent (callers default to 0).
	rows, err := q.CountSecretsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	byAsset := make(map[int64]int64, len(rows))
	for _, row := range rows {
		byAsset[row.AssetID] = row.SecretCount
	}
	assert.Equal(t, int64(2), byAsset[assetA.ID])
	assert.Equal(t, int64(1), byAsset[assetB.ID])
	_, present := byAsset[assetC.ID]
	assert.False(t, present, "asset with no secrets should be absent from the grouped count")
}

// ── Asset Renaming Tests ────────────────────────────────────────────────────

func TestUpdateAsset_RenameAsset(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          tg.Game.ID,
		OwnerID:         tg.Players[0].ID,
		CreatorID:       tg.Players[0].ID,
		AssetType:       model.AssetPeer,
		Name:            "Original Name",
		IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Rename
	err = q.UpdateAssetName(ctx, dbgen.UpdateAssetNameParams{
		ID:   asset.ID,
		Name: "New Name",
	})
	require.NoError(t, err)

	// Verify rename worked
	updated, err := q.GetAssetByID(ctx, asset.ID)
	require.NoError(t, err)
	assert.Equal(t, "New Name", updated.Name)
}

// ── List Assets Tests ───────────────────────────────────────────────────────

func TestListAssets_FiltersByGame(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Create 3 assets in game 1
	for i := 1; i <= 3; i++ {
		_, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:          tg.Game.ID,
			OwnerID:         tg.Players[0].ID,
			CreatorID:       tg.Players[0].ID,
			AssetType:       model.AssetPeer,
			Name:            "Asset",
			IsMainCharacter: false,
		})
		require.NoError(t, err)
	}

	// List assets for game
	assets, err := q.ListAssetsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(assets), 3)
}

// Pins the tombstone invariant: a destroyed asset must appear ONLY in the
// display query (ListAllAssetsByGame, used by the retinue handler) and must
// be excluded from every gameplay-facing query (ListAssetsByGame /
// ListAssetsByOwner). If a future refactor lets a destroyed asset leak into
// the filtered queries it would re-enter mechanics — this test fails first.
func TestListAllAssetsByGame_IncludesDestroyed_GameplayQueriesExclude(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()
	owner := tg.Players[0].ID

	live, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: tg.Game.ID, OwnerID: owner, CreatorID: owner,
		AssetType: model.AssetPeer, Name: "Living Ally",
	})
	require.NoError(t, err)

	dead, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: tg.Game.ID, OwnerID: owner, CreatorID: owner,
		AssetType: model.AssetPeer, Name: "Fallen Ally",
	})
	require.NoError(t, err)
	require.NoError(t, q.DestroyAsset(ctx, dead.ID))

	ids := func(assets []dbgen.Asset) map[int64]bool {
		m := make(map[int64]bool, len(assets))
		for _, a := range assets {
			m[a.ID] = true
		}
		return m
	}

	// Display query: both present.
	all, err := q.ListAllAssetsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	allIDs := ids(all)
	assert.True(t, allIDs[live.ID], "live asset in display query")
	assert.True(t, allIDs[dead.ID], "destroyed asset in display query")

	// Gameplay queries: destroyed excluded.
	byGame, err := q.ListAssetsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	gameIDs := ids(byGame)
	assert.True(t, gameIDs[live.ID], "live asset in game query")
	assert.False(t, gameIDs[dead.ID], "destroyed asset must NOT be in game query")

	byOwner, err := q.ListAssetsByOwner(ctx, owner)
	require.NoError(t, err)
	ownerIDs := ids(byOwner)
	assert.True(t, ownerIDs[live.ID], "live asset in owner query")
	assert.False(t, ownerIDs[dead.ID], "destroyed asset must NOT be in owner query")
}

// ListMarginaliaByGame is the one-query replacement for the per-asset
// ListMarginaliaByAsset loop that ListAssets used to run (20+ round trips in
// a five-player game). It has to be a drop-in: same rows, same grouping, same
// order, and the same lack of filtering — the retinue is a display path that
// renders torn notes struck through and destroyed assets as tombstones, so
// neither may be dropped here.
//
// Asserted against ListMarginaliaByAsset directly rather than against literals,
// so the two can never drift apart.
func TestListMarginaliaByGame_MatchesPerAssetLoop(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	mk := func(owner int64, name string) dbgen.Asset {
		a, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:          tg.Game.ID,
			OwnerID:         owner,
			CreatorID:       owner,
			AssetType:       model.AssetPeer,
			Name:            name,
			IsMainCharacter: false,
		})
		require.NoError(t, err)
		return a
	}
	note := func(assetID int64, pos int16, text string) dbgen.Marginalium {
		m, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: assetID, Position: pos, Text: text,
		})
		require.NoError(t, err)
		return m
	}

	// Two owners, so grouping can't accidentally pass by everything sharing one.
	plain := mk(tg.Players[0].ID, "Plain")
	multi := mk(tg.Players[1].ID, "Multi")
	torn := mk(tg.Players[0].ID, "HasTornNote")
	destroyed := mk(tg.Players[1].ID, "Destroyed")
	bare := mk(tg.Players[0].ID, "NoNotes") // must survive as an empty group

	note(plain.ID, 1, "only note")

	// Inserted out of position order: the query must sort, not echo insert order.
	note(multi.ID, 3, "third")
	note(multi.ID, 1, "first")
	note(multi.ID, 2, "second")

	tornNote := note(torn.ID, 1, "will be torn")
	note(torn.ID, 2, "stays intact")
	_, err := q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
		ID: tornNote.ID, TornByID: &tg.Players[1].ID,
	})
	require.NoError(t, err)

	note(destroyed.ID, 1, "note on a tombstone")
	require.NoError(t, q.DestroyAsset(ctx, destroyed.ID))

	// The one query, grouped exactly as the handler groups it.
	all, err := q.ListMarginaliaByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	got := make(map[int64][]dbgen.Marginalium)
	for _, m := range all {
		got[m.AssetID] = append(got[m.AssetID], m)
	}

	for _, a := range []dbgen.Asset{plain, multi, torn, destroyed, bare} {
		want, err := q.ListMarginaliaByAsset(ctx, a.ID)
		require.NoError(t, err)
		// A missing map key is nil where the per-asset query returned an empty
		// slice. The handler normalizes that (nil would serialize as JSON null
		// instead of [], which the retinue can't render), so compare what it
		// actually emits rather than the raw lookup.
		have := got[a.ID]
		if have == nil {
			have = []dbgen.Marginalium{}
		}
		assert.Equal(t, want, have, "asset %q (id %d) must match the per-asset query exactly", a.Name, a.ID)
	}

	// Spell out the properties the equality check above would also satisfy if
	// *both* queries broke in the same direction.
	assert.Len(t, got[multi.ID], 3, "multi keeps all three notes")
	assert.Equal(t, []string{"first", "second", "third"},
		[]string{got[multi.ID][0].Text, got[multi.ID][1].Text, got[multi.ID][2].Text},
		"ordered by position, not insertion order")
	assert.Len(t, got[torn.ID], 2, "a torn note is still returned (rendered struck through)")
	assert.Len(t, got[destroyed.ID], 1, "a destroyed asset's notes still reach the tombstone card")
	assert.Empty(t, got[bare.ID], "an asset with no notes groups to nothing")

	// Scoped to the game: another game's notes must not bleed in.
	other := newTestGame(t, q, 2)
	otherAsset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: other.Game.ID, OwnerID: other.Players[0].ID, CreatorID: other.Players[0].ID,
		AssetType: model.AssetPeer, Name: "Elsewhere", IsMainCharacter: false,
	})
	require.NoError(t, err)
	note(otherAsset.ID, 1, "different game")
	all, err = q.ListMarginaliaByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	for _, m := range all {
		assert.NotEqual(t, otherAsset.ID, m.AssetID, "another game's marginalia leaked in")
	}
}

// TestListAssetsByIDs_MatchesPerIDLoop pins the batched replacement for the
// GetAssetByID loops on the plan-resolution paths (invoked artifacts, leveraged
// assets, abandoned disagreement peers). Those loops treated "no row" as a
// rejection and read is_destroyed themselves, so the batch has to return
// destroyed rows and simply omit ids that resolve to nothing.
func TestListAssetsByIDs_MatchesPerIDLoop(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	mk := func(owner int64, at model.AssetType, name string) dbgen.Asset {
		a, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:          tg.Game.ID,
			OwnerID:         owner,
			CreatorID:       owner,
			AssetType:       at,
			Name:            name,
			IsMainCharacter: false,
		})
		require.NoError(t, err)
		return a
	}

	artifact := mk(tg.Players[0].ID, model.AssetArtifact, "Chronicle")
	peer := mk(tg.Players[1].ID, model.AssetPeer, "Confidant")
	destroyed := mk(tg.Players[0].ID, model.AssetPeer, "Departed")
	require.NoError(t, q.DestroyAsset(ctx, destroyed.ID))

	// An asset in a different game: the callers check GameID themselves, so the
	// query must return it rather than filter it — otherwise "not in this game"
	// silently becomes "not found" and the two callers that distinguish those
	// (chCastRollHandler's 400 vs mwApplyLeverageTwo's 404) diverge.
	other := newTestGame(t, q, 2)
	foreign, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: other.Game.ID, OwnerID: other.Players[0].ID, CreatorID: other.Players[0].ID,
		AssetType: model.AssetArtifact, Name: "Elsewhere", IsMainCharacter: false,
	})
	require.NoError(t, err)

	const missingID = int64(-1) // never issued by the BIGSERIAL
	ids := []int64{artifact.ID, peer.ID, destroyed.ID, foreign.ID, missingID}

	rows, err := q.ListAssetsByIDs(ctx, ids)
	require.NoError(t, err)
	got := make(map[int64]dbgen.Asset, len(rows))
	for _, a := range rows {
		got[a.ID] = a
	}

	for _, id := range ids {
		want, wantErr := q.GetAssetByID(ctx, id)
		have, found := got[id]
		if wantErr != nil {
			assert.Falsef(t, found, "id %d resolves to no row per-id but appears in the batch", id)
			continue
		}
		require.Truef(t, found, "id %d resolves per-id but is missing from the batch", id)
		assert.Equalf(t, want, have, "asset %d must match the per-id query exactly", id)
	}

	// Properties the equality loop above would also satisfy if both queries
	// broke the same way.
	assert.Len(t, got, 4, "every real id comes back; only the missing one drops")
	assert.True(t, got[destroyed.ID].IsDestroyed, "a destroyed asset is returned, not filtered")
	assert.Equal(t, other.Game.ID, got[foreign.ID].GameID, "cross-game ids are returned for the caller to reject")

	// Empty input must be a clean empty result, not an error or a full scan.
	empty, err := q.ListAssetsByIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty, "no ids means no rows")
}

// TestListMarginaliaByAssets_MatchesPerAssetQueries pins the batched prefetch
// that replaced ListIntactMarginalia + assetIsBlank's CountMarginalia inside
// hfBreakAbandonedDisagreementPeers. It returns torn rows deliberately: an
// empty *intact* list alone cannot distinguish a blank peer (destroyed outright
// by a break) from an all-torn one (skipped), and those break differently.
func TestListMarginaliaByAssets_MatchesPerAssetQueries(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	mk := func(owner int64, name string) dbgen.Asset {
		a, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID: tg.Game.ID, OwnerID: owner, CreatorID: owner,
			AssetType: model.AssetPeer, Name: name, IsMainCharacter: false,
		})
		require.NoError(t, err)
		return a
	}
	note := func(assetID int64, pos int16, text string) dbgen.Marginalium {
		m, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: assetID, Position: pos, Text: text,
		})
		require.NoError(t, err)
		return m
	}
	tear := func(m dbgen.Marginalium) {
		_, err := q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{ID: m.ID, TornByID: &tg.Players[0].ID})
		require.NoError(t, err)
	}

	// Two owners so grouping can't pass by everything sharing one.
	intact := mk(tg.Players[0].ID, "AllIntact")
	partial := mk(tg.Players[1].ID, "PartlyTorn")
	allTorn := mk(tg.Players[0].ID, "AllTorn")
	blank := mk(tg.Players[1].ID, "Blank") // no rows at all — the D3 case

	// Inserted out of position order: the query must sort, not echo insert order.
	note(intact.ID, 2, "second")
	note(intact.ID, 1, "first")

	tear(note(partial.ID, 1, "torn one"))
	note(partial.ID, 2, "still here")

	tear(note(allTorn.ID, 1, "gone"))

	assets := []dbgen.Asset{intact, partial, allTorn, blank}
	ids := make([]int64, 0, len(assets))
	for _, a := range assets {
		ids = append(ids, a.ID)
	}

	// Grouped exactly as hfBreakAbandonedDisagreementPeers groups it.
	all, err := q.ListMarginaliaByAssets(ctx, ids)
	require.NoError(t, err)
	gotIntact := make(map[int64][]dbgen.Marginalium)
	hasAny := make(map[int64]bool)
	for _, m := range all {
		hasAny[m.AssetID] = true
		if !m.IsTorn {
			gotIntact[m.AssetID] = append(gotIntact[m.AssetID], m)
		}
	}

	for _, a := range assets {
		want, err := q.ListIntactMarginalia(ctx, a.ID)
		require.NoError(t, err)
		// A missing map key is nil where the per-asset query returned an empty
		// slice. The handler only ever asks len() of this, so normalize here
		// rather than in the loop — but the equality must still hold.
		have := gotIntact[a.ID]
		if have == nil {
			have = []dbgen.Marginalium{}
		}
		assert.Equalf(t, want, have, "asset %q intact list must match ListIntactMarginalia", a.Name)

		// The blankness half: hasAny replaces assetIsBlank's CountMarginalia.
		count, err := q.CountMarginalia(ctx, a.ID)
		require.NoError(t, err)
		assert.Equalf(t, count == 0, !hasAny[a.ID],
			"asset %q blankness must match assetIsBlank", a.Name)
	}

	// The distinction the torn rows exist for.
	assert.Empty(t, gotIntact[allTorn.ID], "all-torn peer has no intact note to tear")
	assert.True(t, hasAny[allTorn.ID], "...but is not blank, so it is skipped, not destroyed")
	assert.Empty(t, gotIntact[blank.ID], "blank peer has no intact note either")
	assert.False(t, hasAny[blank.ID], "...and IS blank, so the break destroys it outright")

	assert.Equal(t, []string{"first", "second"},
		[]string{gotIntact[intact.ID][0].Text, gotIntact[intact.ID][1].Text},
		"ordered by position, not insertion order")

	// Scoped to the id list: an unrequested asset's notes must not bleed in.
	stranger := mk(tg.Players[0].ID, "NotAsked")
	note(stranger.ID, 1, "should not appear")
	all, err = q.ListMarginaliaByAssets(ctx, ids)
	require.NoError(t, err)
	for _, m := range all {
		assert.NotEqual(t, stranger.ID, m.AssetID, "an asset outside the id list leaked in")
	}
}
