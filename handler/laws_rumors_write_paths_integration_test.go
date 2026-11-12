//go:build integration

// handler/laws_rumors_write_paths_integration_test.go — every place a law or
// rumor is written tells the database who wrote it, so their own "new" marker
// follows the entry and nobody else's does (migration 058). One test per path,
// because each has its own idea of who the writer is, and a path that forgets to
// say would pass every other test: its entry would simply show as new to the
// person who wrote it.
//
// Also pinned here: that this catch-up cannot hurt the write it rides on, even
// inside a transaction.

package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// markers reads one seat's two markers.
func markers(t *testing.T, q *dbgen.Queries, player dbgen.Player) (laws, rumors int64) {
	t.Helper()
	m, err := q.GetLawsRumorsSeen(context.Background(), player.ID)
	require.NoError(t, err)
	return m.LawsSeenRev, m.RumorsSeenRev
}

// wantOnlyWriterCaughtUp asserts the entry's rev is the writer's marker and that
// every other seat still has it unseen (marker 0: they have seen nothing).
func wantOnlyWriterCaughtUp(t *testing.T, q *dbgen.Queries, players []dbgen.Player, writer int, rev int64, rumors bool) {
	t.Helper()
	for i, p := range players {
		l, r := markers(t, q, p)
		got := l
		if rumors {
			got = r
		}
		if i == writer {
			require.Equalf(t, rev, got, "seat %d wrote the entry: it is not news to them", i)
		} else {
			require.Zerof(t, got, "seat %d did not write the entry: it must still be new to them", i)
		}
	}
}

func lawsRumorsPlanDeps(t *testing.T) (*PlanDeps, testGame, *dbgen.Queries) {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	return &PlanDeps{Store: db.NewStore(pool), Manager: hub.NewManager()}, tg, q
}

// Propose Decree: the preparer drafted and finalized the text and enacts the law.
// The signatory is a different seat here and wrote only a rider, so they do not
// count as the writer.
func TestWritePath_ProposeDecree(t *testing.T) {
	deps, tg, q := lawsRumorsPlanDeps(t)
	ctx := context.Background()
	preparer, signatory := tg.Players[0], tg.Players[1]
	plan := createPlanOnRow(t, q, &tg.Game, &preparer, model.PlanProposeDecree, model.CategoryPower, 1)

	resData := &ResolutionData{}
	pd := resData.EnsureProposeDecree()
	pd.LawText = "No wine before noon."
	pd.SignatoryID = &signatory.ID
	pd.Outcome = marOutcome // a mar enacts the law without minting a resource asset
	pd.AddendumPlaced = true
	pd.Addendum = "salt is exempt"

	require.NoError(t, pdEnactLaw(ctx, deps, &plan, resData, "", ""))

	laws, err := q.ListLaws(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, laws, 1)
	wantOnlyWriterCaughtUp(t, q, tg.Players, 0, laws[0].Rev, false)
}

// Spread Rumors: the preparer writes the rumor on a make; on a mar the target's
// owner writes the counter-rumor. A hidden source changes the public attribution,
// never who is told it is new.
func TestWritePath_SpreadRumors(t *testing.T) {
	setup := func(t *testing.T) (*PlanDeps, testGame, *dbgen.Queries, dbgen.Plan, int64) {
		deps, tg, q := lawsRumorsPlanDeps(t)
		subject, err := q.CreateAsset(context.Background(), dbgen.CreateAssetParams{
			GameID: tg.Game.ID, OwnerID: tg.Players[1].ID, CreatorID: tg.Players[1].ID,
			AssetType: model.AssetPeer, Name: "the Duchess",
		})
		require.NoError(t, err)
		plan := createPlanOnRow(t, q, &tg.Game, &tg.Players[0], model.PlanSpreadRumors, model.CategoryEsteem, 1)
		plan.TargetAssetID = &subject.ID
		return deps, tg, q, plan, subject.ID
	}

	t.Run("make: the preparer wrote it", func(t *testing.T) {
		deps, tg, q, plan, _ := setup(t)
		resData := &ResolutionData{}
		resData.EnsureSpreadRumors().RumorText = "She poisons her own wine."
		require.NoError(t, srHandler{}.ApplyChoice(context.Background(), deps, &plan, resData, nil, makeOutcome))

		rumors, err := q.ListRumors(context.Background(), tg.Game.ID)
		require.NoError(t, err)
		require.Len(t, rumors, 1)
		wantOnlyWriterCaughtUp(t, q, tg.Players, 0, rumors[0].Rev, true)
	})

	t.Run("make with the source hidden: still the preparer", func(t *testing.T) {
		deps, tg, q, plan, _ := setup(t)
		resData := &ResolutionData{}
		resData.EnsureSpreadRumors().RumorText = "She poisons her own wine."
		require.NoError(t, srHandler{}.ApplyChoice(context.Background(), deps, &plan, resData,
			[]string{srOptHideSource}, makeOutcome))

		rumors, err := q.ListRumors(context.Background(), tg.Game.ID)
		require.NoError(t, err)
		require.Len(t, rumors, 1)
		require.Nil(t, rumors[0].SourcePlayerID, "the source is hidden on the record")
		wantOnlyWriterCaughtUp(t, q, tg.Players, 0, rumors[0].Rev, true)
	})

	t.Run("mar: the counter-rumor's author wrote it, not the preparer", func(t *testing.T) {
		deps, tg, q, plan, _ := setup(t)
		resData := &ResolutionData{}
		sr := resData.EnsureSpreadRumors()
		sr.CounterRumorText = "It is the preparer who poisons the wine."
		sr.CounterRumorAuthorID = tg.Players[1].ID
		require.NoError(t, srHandler{}.ApplyChoice(context.Background(), deps, &plan, resData, nil, marOutcome))

		rumors, err := q.ListRumors(context.Background(), tg.Game.ID)
		require.NoError(t, err)
		require.Len(t, rumors, 1)
		wantOnlyWriterCaughtUp(t, q, tg.Players, 1, rumors[0].Rev, true)
	})
}

// Host Festivity: ActingPlayerID is who the effect lands on, not who typed the
// text, so the writer is the route's caller (AuthorID).
func TestWritePath_HostFestivity(t *testing.T) {
	// run applies a festivity rumor option as built from the game's seats, and
	// returns the one rumor it wrote.
	run := func(t *testing.T, build func(seats []dbgen.Player) festivityOptionInput) (*dbgen.Queries, testGame, dbgen.Rumor) {
		t.Helper()
		deps, tg, q := lawsRumorsPlanDeps(t)
		ctx := context.Background()
		plan := createPlanOnRow(t, q, &tg.Game, &tg.Players[0], model.PlanHostFestivity, model.CategoryEsteem, 1)
		in := build(tg.Players)
		in.RumorText = "The host waters the wine."
		require.NoError(t, hfApplyOption(ctx, deps, &plan, &ResolutionData{}, in))
		rumors, err := q.ListRumors(ctx, tg.Game.ID)
		require.NoError(t, err)
		require.Len(t, rumors, 1)
		return q, tg, rumors[0]
	}

	t.Run("make: a guest spreads a rumor and wrote it", func(t *testing.T) {
		q, tg, rumor := run(t, func(seats []dbgen.Player) festivityOptionInput {
			return festivityOptionInput{
				ActingPlayerID: seats[1].ID, AuthorID: seats[1].ID,
				Choice: gamepkg.FestivityMakeSpreadRumor, IsMake: true,
			}
		})
		require.NotNil(t, rumor.SourcePlayerID, "a made rumor names its spreader")
		wantOnlyWriterCaughtUp(t, q, tg.Players, 1, rumor.Rev, true)
	})

	t.Run("mar: a rumor about the guest is written by whoever is at the keyboard", func(t *testing.T) {
		// The rumor lands on seat 1 (it is about their main character); seat 2
		// typed it. Seat 1 is not its writer and has not seen it.
		q, tg, rumor := run(t, func(seats []dbgen.Player) festivityOptionInput {
			return festivityOptionInput{
				ActingPlayerID: seats[1].ID, AuthorID: seats[2].ID,
				Choice: gamepkg.FestivityMarRumorAboutYou, IsMake: false,
			}
		})
		require.Nil(t, rumor.SourcePlayerID, "a marred rumor has no named source")
		wantOnlyWriterCaughtUp(t, q, tg.Players, 2, rumor.Rev, true)
	})

	t.Run("a host's mar settled by another player: the host did not write it", func(t *testing.T) {
		// A Make Demands perform_steps winner (seat 2) drives the host's (seat 0)
		// insisted mar in the host's stead: the effect lands on the host, the text
		// is the winner's.
		q, tg, rumor := run(t, func(seats []dbgen.Player) festivityOptionInput {
			return festivityOptionInput{
				ActingPlayerID: seats[0].ID, AuthorID: seats[2].ID,
				Choice: gamepkg.FestivityMarRumorAboutYou, IsMake: false,
			}
		})
		wantOnlyWriterCaughtUp(t, q, tg.Players, 2, rumor.Rev, true)
	})
}

// The catch-up is part of the write, so it cannot fail apart from it. Two
// consequences worth pinning: a transaction that creates a law still commits and
// its writer is caught up (the Prologue creates laws and rumors inside one), and
// a writer id that is not a seat at the game quietly catches nobody up instead
// of failing the insert on a foreign key (which would have aborted the
// transaction around it).
func TestWritePath_InsideATransaction(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)
	other := newTestGame(t, q, 2) // a different game's seat

	t.Run("the Prologue's law and rumor commit and catch up their writer", func(t *testing.T) {
		require.NoError(t, store.InTx(ctx, func(tx *dbgen.Queries) error {
			if err := addLawOrRumor(ctx, tx, hub.NewManager(), tg.Game.ID, tg.Players[0].ID, "Write a law", "No dogs."); err != nil {
				return err
			}
			return addLawOrRumor(ctx, tx, hub.NewManager(), tg.Game.ID, tg.Players[0].ID, "Spread a rumor", "Dogs talk.")
		}))
		laws, err := q.ListLaws(ctx, tg.Game.ID)
		require.NoError(t, err)
		rumors, err := q.ListRumors(ctx, tg.Game.ID)
		require.NoError(t, err)
		require.Len(t, laws, 1)
		require.Len(t, rumors, 1)
		wantOnlyWriterCaughtUp(t, q, tg.Players, 0, laws[0].Rev, false)
		wantOnlyWriterCaughtUp(t, q, tg.Players, 0, rumors[0].Rev, true)
	})

	t.Run("a writer who is not a seat here neither fails the insert nor poisons the transaction", func(t *testing.T) {
		stranger := other.Players[0].ID
		require.NoError(t, store.InTx(ctx, func(tx *dbgen.Queries) error {
			law, err := tx.CreateLaw(ctx, dbgen.CreateLawParams{
				GameID: tg.Game.ID, Text: "A law nobody caught up on.", WriterID: &stranger,
			})
			if err != nil {
				return err
			}
			require.Positive(t, law.Rev)
			// The transaction is still usable afterwards: had the catch-up been a
			// failed statement, this would error with "current transaction is aborted".
			_, err = tx.ListLaws(ctx, tg.Game.ID)
			return err
		}))
		l, r := markers(t, q, other.Players[0])
		require.Zero(t, l, "a seat at another game is not caught up on this game's law")
		require.Zero(t, r)
	})

	t.Run("no writer at all leaves everyone's marker alone", func(t *testing.T) {
		before, _ := markers(t, q, tg.Players[1])
		_, err := q.CreateLaw(ctx, dbgen.CreateLawParams{GameID: tg.Game.ID, Text: "Seeded."})
		require.NoError(t, err)
		after, _ := markers(t, q, tg.Players[1])
		require.Equal(t, before, after)
	})
}
