//go:build integration

// handler/accounts_rename_integration_test.go — coverage for propagating an
// account username change to the denormalized players.display_name snapshot.
//
// players.display_name is copied from the account at join time, so a Profile
// rename has to fan out to every seat the account holds across in-progress
// games. These tests drive updateAccountFields (the real update path used by
// the PATCH /api/accounts route) and assert the fan-out and its scoping.

package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
)

func TestUsernameRenamePropagatesToPlayerSeats(t *testing.T) {
	ctx := context.Background()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)

	tg := newTestGame(t, q, 2)
	renamed := tg.Players[0]
	unchanged := tg.Players[1]

	// Add the same account to a second game so we can prove the rename fans
	// out across every in-progress table, not just the one we seeded.
	game2, err := q.CreateGame(ctx, "RENAME-"+randSuffix())
	require.NoError(t, err)
	seat2, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
		GameID:        game2.ID,
		DisplayName:   renamed.DisplayName,
		AccountID:     renamed.AccountID,
		IsFacilitator: true,
	})
	require.NoError(t, err)

	const newName = "Renamed Royal"
	acct := &appMiddleware.Account{ID: renamed.AccountID, Username: renamed.DisplayName}
	newUsername := newName
	require.NoError(t, store.InTx(ctx, func(tx *dbgen.Queries) error {
		return updateAccountFields(ctx, tx, acct, &newUsername, nil, nil)
	}))

	// Both seats held by the renamed account now show the new name.
	got1, err := q.GetPlayerByID(ctx, renamed.ID)
	require.NoError(t, err)
	require.Equal(t, newName, got1.DisplayName)

	got2, err := q.GetPlayerByID(ctx, seat2.ID)
	require.NoError(t, err)
	require.Equal(t, newName, got2.DisplayName)

	// A different account's seat in the same game is untouched.
	gotOther, err := q.GetPlayerByID(ctx, unchanged.ID)
	require.NoError(t, err)
	require.Equal(t, unchanged.DisplayName, gotOther.DisplayName)
}
