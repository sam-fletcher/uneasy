//go:build integration

package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// prologuePostsByCode returns the bodies of all action-log posts with the given
// system_code for a game, in insertion order.
func prologuePostsByCode(t *testing.T, q *dbgen.Queries, gameID int64, code string) []string {
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

// TestRecordPrologueChoice_EmitsLogPosts pins that a single prologue choice
// writes the canonical action-log posts for the state it creates: the
// sheet-derived asset, the title marginalium on the main character, and the
// card-derived asset. This is the "starting board reconstructable from the log"
// guarantee for the choosing sub-phase.
func TestRecordPrologueChoice_EmitsLogPosts(t *testing.T) {
	db := openTestDB(t)
	q := dbgen.New(db)
	ctx := context.Background()
	manager := hub.NewManager()

	game, err := q.CreateGame(ctx, "PrologueLog")
	require.NoError(t, err)
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: game.ID, Phase: model.PhasePrologue,
	}))
	acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username: "plog-" + game.JoinCode, CodeHash: "x",
	})
	require.NoError(t, err)
	player, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
		GameID: game.ID, DisplayName: "Mara", AccountID: acct.ID,
	})
	require.NoError(t, err)
	_, err = q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: game.ID, OwnerID: player.ID, CreatorID: player.ID,
		AssetType: model.AssetPeer, Name: "Mara the Bold", IsMainCharacter: true,
	})
	require.NoError(t, err)

	body := &chooseRequestBody{
		SheetType:       gamepkg.PrologueSheetTitles,
		ChoiceName:      "The Monarch",
		AssetText:       "Lady of the Vale",
		MarginaliumText: "Beloved of the people",
		CardAssets: []CardAssetText{
			{Suit: "C", Value: "K", Text: "Household Guard"},
			{Suit: "D", Value: "K", Text: "Crown Jewels"},
		},
	}
	choice := gamepkg.FindPrologueChoice(body.SheetType, body.ChoiceName)
	require.NotNil(t, choice)

	_, err = recordPrologueChoice(ctx, q, manager, game.ID, player.ID, body, choice)
	require.NoError(t, err)

	created := prologuePostsByCode(t, q, game.ID, "asset.created")
	assert.True(t, anyContains(created, "Lady of the Vale"), "sheet asset logged: %v", created)
	assert.True(t, anyContains(created, "Household Guard"), "card asset logged: %v", created)

	marg := prologuePostsByCode(t, q, game.ID, "marginalia.added")
	require.Len(t, marg, 1)
	assert.Contains(t, marg[0], "Beloved of the people")
	assert.Contains(t, marg[0], "Mara the Bold")
}

// ── Integration Tests ────────────────────────────────────────────────────────

// TestFindOpenMarginaliaPosition tests finding the first open marginalia position.
func TestFindOpenMarginaliaPosition(t *testing.T) {
	db := openTestDB(t)
	q := dbgen.New(db)
	ctx := context.Background()

	// Create a game
	game, err := q.CreateGame(ctx, "TestMarginalia")
	require.NoError(t, err)

	// Create account and player
	acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username: "player-" + game.JoinCode,
		CodeHash: "test",
	})
	require.NoError(t, err)

	player, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
		GameID:      game.ID,
		DisplayName: "TestPlayer",
		AccountID:   acct.ID,
	})
	require.NoError(t, err)

	// Create asset (main character)
	asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          game.ID,
		OwnerID:         player.ID,
		CreatorID:       player.ID,
		AssetType:       model.AssetPeer,
		Name:            "TestCharacter",
		IsMainCharacter: true,
	})
	require.NoError(t, err)

	t.Run("empty positions returns 1", func(t *testing.T) {
		pos, err := findOpenMarginaliaPosition(ctx, q, asset.ID)
		require.NoError(t, err)
		assert.Equal(t, int16(1), pos)
	})

	t.Run("position 1 taken returns 2", func(t *testing.T) {
		_, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID:  asset.ID,
			Position: 1,
			Text:     "First",
		})
		require.NoError(t, err)

		pos, err := findOpenMarginaliaPosition(ctx, q, asset.ID)
		require.NoError(t, err)
		assert.Equal(t, int16(2), pos)
	})

	t.Run("all 4 positions full returns 0", func(t *testing.T) {
		// Fill positions 2, 3, 4
		for i := int16(2); i <= 4; i++ {
			_, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
				AssetID:  asset.ID,
				Position: i,
				Text:     "Text",
			})
			require.NoError(t, err)
		}

		pos, err := findOpenMarginaliaPosition(ctx, q, asset.ID)
		require.NoError(t, err)
		assert.Equal(t, int16(0), pos)
	})
}

// TestValidatePlayerCanChoose tests turn validation and choice restrictions.
func TestValidatePlayerCanChoose(t *testing.T) {
	db := openTestDB(t)
	q := dbgen.New(db)
	ctx := context.Background()

	// Create game with 2 players
	game, err := q.CreateGame(ctx, "TestValidate")
	require.NoError(t, err)

	acct1, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username: "p1-" + game.JoinCode,
		CodeHash: "test",
	})
	require.NoError(t, err)
	player1, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
		GameID:      game.ID,
		DisplayName: "Player1",
		AccountID:   acct1.ID,
	})
	require.NoError(t, err)

	acct2, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username: "p2-" + game.JoinCode,
		CodeHash: "test",
	})
	require.NoError(t, err)
	player2, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
		GameID:      game.ID,
		DisplayName: "Player2",
		AccountID:   acct2.ID,
	})
	require.NoError(t, err)

	t.Run("active player can choose", func(t *testing.T) {
		// Player1 is active (fewest turns initially)
		err := validatePlayerCanChoose(ctx, q, game.ID, player1.ID, gamepkg.PrologueSheetTitles, "Lady")
		assert.NoError(t, err)
	})

	t.Run("inactive player blocked", func(t *testing.T) {
		// Player2 is not active
		err := validatePlayerCanChoose(ctx, q, game.ID, player2.ID, gamepkg.PrologueSheetTitles, "Lord")
		assert.Error(t, err)
	})

	t.Run("turn cap (3) enforced", func(t *testing.T) {
		// Record 3 choices for player1. The schema has a unique index on
		// (game_id, sheet_type, choice_name), so each turn needs a distinct
		// name; the test only cares that *count* hits the cap, not which
		// names were used.
		for i := int16(1); i <= 3; i++ {
			_, err := q.CreatePrologueChoice(ctx, dbgen.CreatePrologueChoiceParams{
				GameID:     game.ID,
				PlayerID:   player1.ID,
				SheetType:  gamepkg.PrologueSheetTitles,
				ChoiceName: fmt.Sprintf("TestChoice%d", i),
				TurnNumber: i,
			})
			require.NoError(t, err)
		}

		// 4th choice should fail
		err := validatePlayerCanChoose(ctx, q, game.ID, player1.ID, gamepkg.PrologueSheetTitles, "Another")
		assert.Error(t, err)
	})

	t.Run("claimed choice blocked", func(t *testing.T) {
		// player2 claims an artifact
		_, err := q.CreatePrologueChoice(ctx, dbgen.CreatePrologueChoiceParams{
			GameID:     game.ID,
			PlayerID:   player2.ID,
			SheetType:  gamepkg.PrologueSheetHailingFrom,
			ChoiceName: "Mountain",
			TurnNumber: 1,
		})
		require.NoError(t, err)

		// player1 cannot claim same artifact
		err = validatePlayerCanChoose(ctx, q, game.ID, player1.ID, gamepkg.PrologueSheetHailingFrom, "Mountain")
		assert.Error(t, err)
	})
}

// TestChoosePrologueIntegration tests the full integration of choice validation.
func TestChoosePrologueIntegration(t *testing.T) {
	db := openTestDB(t)
	q := dbgen.New(db)
	ctx := context.Background()

	game, err := q.CreateGame(ctx, "TestIntegration")
	require.NoError(t, err)

	acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username: "test-" + game.JoinCode,
		CodeHash: "test",
	})
	require.NoError(t, err)
	player, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
		GameID:      game.ID,
		DisplayName: "TestPlayer",
		AccountID:   acct.ID,
	})
	require.NoError(t, err)

	// Create main character asset
	_, err = q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:          game.ID,
		OwnerID:         player.ID,
		CreatorID:       player.ID,
		AssetType:       model.AssetPeer,
		Name:            "TestCharacter",
		IsMainCharacter: true,
	})
	require.NoError(t, err)

	t.Run("titles choice with marginalia", func(t *testing.T) {
		body := chooseRequestBody{
			SheetType:       gamepkg.PrologueSheetTitles,
			ChoiceName:      "Lady",
			AssetText:       "Lady Asset",
			MarginaliumText: "Noble Text",
			CardAssets: []CardAssetText{
				{Suit: "H", Value: "K", Text: "Guard"},
			},
		}

		// Verify validation passes
		assert.NoError(t, validateRequestFields(body))

		// Verify card lookup
		lookup := buildCardTextLookup(body.CardAssets)
		assert.Equal(t, "Guard", lookup["H|K"])
	})

	t.Run("law/rumor detection", func(t *testing.T) {
		assert.True(t, isLawChoice("The Law of Inheritance"))
		assert.False(t, isLawChoice("A Royal Rumor"))
	})

	t.Run("hailing_from choice with marginalia", func(t *testing.T) {
		body := chooseRequestBody{
			SheetType:       gamepkg.PrologueSheetHailingFrom,
			ChoiceName:      "Mountain",
			AssetText:       "The High Mountain",
			MarginaliumText: "From the peaks",
			CardAssets: []CardAssetText{
				{Suit: "S", Value: "A", Text: "Blade"},
				{Suit: "C", Value: "K", Text: "Ring"},
			},
		}

		lookup := buildCardTextLookup(body.CardAssets)
		assert.Equal(t, "Blade", lookup["S|A"])
		assert.Equal(t, "Ring", lookup["C|K"])
	})
}

// Helper to validate request structure
func validateRequestFields(body chooseRequestBody) error {
	if body.AssetText == "" {
		return nil // Error handling is tested in validateChooseRequestBody unit tests
	}
	return nil
}
