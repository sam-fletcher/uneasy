//go:build integration

// handler/table_title_integration_test.go — the fields GET /api/accounts/me/tables
// sends so a profile row can wear the same title as the table's own title bar:
// current_row, plan_type, prologue_ranking_step, ending_vote_open and
// shake_up_category, in every phase a table can be in. The words themselves are
// built client-side (frontend/src/lib/tableTitle.ts); the plan pick is covered
// against the shared fixture in table_title_test.go.

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/gametest"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// titleRow is the slice of a ListMyTables entry this file cares about. The
// pointers and the raw message tell "sent as null" from "not sent at all".
type titleRow struct {
	GameID              int64                      `json:"game_id"`
	Phase               string                     `json:"phase"`
	CurrentRow          int16                      `json:"current_row"`
	PlanType            *string                    `json:"plan_type"`
	PrologueRankingStep *string                    `json:"prologue_ranking_step"`
	EndingVoteOpen      bool                       `json:"ending_vote_open"`
	ShakeUpCategory     *string                    `json:"shake_up_category"`
	Raw                 map[string]json.RawMessage `json:"-"`
}

// titleRowsFor calls ListMyTables as the account and returns its tables by game.
func titleRowsFor(t *testing.T, store *db.Store, accountID int64) map[int64]titleRow {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/accounts/me/tables", nil)
	req = req.WithContext(appMiddleware.AccountContext(req.Context(), &appMiddleware.Account{ID: accountID}))
	w := httptest.NewRecorder()
	ListMyTables(store, hub.NewManager())(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Tables []json.RawMessage `json:"tables"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	out := make(map[int64]titleRow, len(resp.Tables))
	for _, raw := range resp.Tables {
		var row titleRow
		require.NoError(t, json.Unmarshal(raw, &row))
		require.NoError(t, json.Unmarshal(raw, &row.Raw))
		out[row.GameID] = row
	}
	return out
}

// TestListMyTablesTitleFields_EveryPhase walks one table through every state its
// title can name and checks what the profile list sends at each.
func TestListMyTablesTitleFields_EveryPhase(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := t.Context()

	tg := newTestGame(t, q, 2)
	game, viewer := tg.Game, tg.Players[0]
	row := func() titleRow {
		t.Helper()
		got, ok := titleRowsFor(t, store, viewer.AccountID)[game.ID]
		require.True(t, ok, "the table must be listed")
		return got
	}
	setPhase := func(p model.GamePhase) {
		t.Helper()
		require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{ID: game.ID, Phase: p}))
	}
	setPrologueStep := func(step *string) {
		t.Helper()
		require.NoError(t, q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
			ID: game.ID, PrologueRankingStep: step,
		}))
	}

	t.Run("lobby sends the row, no plan and nothing else", func(t *testing.T) {
		setPhase(model.PhaseLobby)
		got := row()
		require.Equal(t, "lobby", got.Phase)
		require.Nil(t, got.PlanType)
		require.Nil(t, got.PrologueRankingStep)
		require.False(t, got.EndingVoteOpen)
		require.Nil(t, got.ShakeUpCategory)
		// Present as null, not absent: the client reads a missing field as undefined.
		for _, k := range []string{"plan_type", "prologue_ranking_step", "shake_up_category"} {
			require.Contains(t, got.Raw, k)
			require.JSONEq(t, "null", string(got.Raw[k]))
		}
	})

	t.Run("prologue sends each step", func(t *testing.T) {
		setPhase(model.PhasePrologue)
		for _, step := range []*string{
			nil, // choosing
			new("declare_power"), new("place_set_asides_power"),
			new("declare_knowledge"), new("place_set_asides_knowledge"),
			new("declare_esteem"), new("place_set_asides_esteem"),
			new("closing"),
		} {
			setPrologueStep(step)
			got := row()
			require.Equal(t, "prologue", got.Phase)
			require.Equal(t, step, got.PrologueRankingStep)
			require.Nil(t, got.PlanType, "a plan belongs to the Main Event")
		}
		setPrologueStep(nil)
	})

	t.Run("main event: a row with no plan", func(t *testing.T) {
		setPhase(model.PhaseMainEvent)
		require.NoError(t, q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{ID: game.ID, CurrentRow: 6}))
		got := row()
		require.Equal(t, "main_event", got.Phase)
		require.EqualValues(t, 6, got.CurrentRow)
		require.Nil(t, got.PlanType)
	})

	t.Run("main event: a pending plan, then resolving, then resolved", func(t *testing.T) {
		plan := createPlanOnRow(t, q, &game, &viewer, model.PlanSeekAnswers, model.CategoryKnowledge, 6)
		require.Equal(t, "seek_answers", *row().PlanType, "pending on the current row")

		require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{ID: plan.ID, Status: model.PlanResolving}))
		require.Equal(t, "seek_answers", *row().PlanType, "resolving")

		require.NoError(t, q.SetPlanResult(ctx, dbgen.SetPlanResultParams{ID: plan.ID, Result: new("make")}))
		require.Equal(t, "seek_answers", *row().PlanType, "resolved: the row keeps its name")
	})

	t.Run("main event: plans on other rows are not this row's", func(t *testing.T) {
		require.NoError(t, q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{ID: game.ID, CurrentRow: 7}))
		createPlanOnRow(t, q, &game, &viewer, model.PlanMakeWar, model.CategoryPower, 9)
		got := row()
		require.EqualValues(t, 7, got.CurrentRow)
		require.Nil(t, got.PlanType, "seek_answers sits on row 6 and make_war on row 9")
	})

	t.Run("main event: the ending vote", func(t *testing.T) {
		require.NoError(t, q.SetEndingVoteOpen(ctx, dbgen.SetEndingVoteOpenParams{ID: game.ID, EndingVoteOpen: true}))
		got := row()
		require.True(t, got.EndingVoteOpen)
		require.EqualValues(t, 7, got.CurrentRow)
		require.NoError(t, q.SetEndingVoteOpen(ctx, dbgen.SetEndingVoteOpenParams{ID: game.ID, EndingVoteOpen: false}))
		require.False(t, row().EndingVoteOpen)
	})

	t.Run("shake-up sends its category and no plan", func(t *testing.T) {
		setPhase(model.PhaseShakeUp)
		require.NoError(t, q.SetShakeUpStep(ctx, dbgen.SetShakeUpStepParams{
			ID: game.ID, ShakeUpCategory: new("esteem"), ShakeUpStep: new(int16(1)),
		}))
		got := row()
		require.Equal(t, "shake_up", got.Phase)
		require.Equal(t, new("esteem"), got.ShakeUpCategory)
		require.Nil(t, got.PlanType, "the Main Event's plans are over")
	})

	t.Run("game over", func(t *testing.T) {
		setPhase(model.PhaseEnded)
		got := row()
		require.Equal(t, "ended", got.Phase)
		require.Nil(t, got.PlanType)
	})
}

// TestListMyTablesTitleFields_AcrossTables seats one account at three Main Event
// tables whose rows and plans differ, so a title wearing another table's plan is
// unmissable. The plans are read in one batch and redistributed by game id, the
// shape a single-table test cannot fail.
func TestListMyTablesTitleFields_AcrossTables(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := t.Context()

	tgA := newTestGame(t, q, 2, gametest.WithCurrentRow(6), gametest.WithPlan(gametest.SeedPlan{
		PreparerIdx: 0, PlanType: model.PlanSeekAnswers, Category: model.CategoryKnowledge, Row: 6,
	}))
	tgB := newTestGame(t, q, 2, gametest.WithCurrentRow(3), gametest.WithPlan(gametest.SeedPlan{
		PreparerIdx: 0, PlanType: model.PlanProposeDecree, Category: model.CategoryPower, Row: 3,
	}))
	tgC := newTestGame(t, q, 2, gametest.WithCurrentRow(9)) // no plan
	viewer := tgA.Players[0]

	for _, tg := range []testGame{tgB, tgC} {
		_, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID: tg.Game.ID, DisplayName: viewer.DisplayName, AccountID: viewer.AccountID,
		})
		require.NoError(t, err)
	}
	// A plan on A's current row, but in another game: it must not leak into C.
	createPlanOnRow(t, q, &tgA.Game, &tgA.Players[1], model.PlanSpreadRumors, model.CategoryEsteem, 9)

	rows := titleRowsFor(t, store, viewer.AccountID)
	require.Len(t, rows, 3)

	a, b, c := rows[tgA.Game.ID], rows[tgB.Game.ID], rows[tgC.Game.ID]
	require.EqualValues(t, 6, a.CurrentRow)
	require.Equal(t, "seek_answers", *a.PlanType)
	require.EqualValues(t, 3, b.CurrentRow)
	require.Equal(t, "propose_decree", *b.PlanType)
	require.EqualValues(t, 9, c.CurrentRow)
	require.Nil(t, c.PlanType)
}

// TestListMyTablesTitleFields_FestivityUnderADuel pins the one pick the SQL
// cannot make: a Host Festivity paused under the duel it spawned names the duel,
// the plan the table is actually playing (planSuspendedByNested).
func TestListMyTablesTitleFields_FestivityUnderADuel(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := t.Context()

	tg := newTestGame(t, q, 2, gametest.WithCurrentRow(6))
	festivity := createPlanOnRow(t, q, &tg.Game, &tg.Players[0], model.PlanHostFestivity, model.CategoryEsteem, 6)
	duel := createPlanOnRow(t, q, &tg.Game, &tg.Players[1], model.PlanProposeDuel, model.CategoryEsteem, 6)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{ID: festivity.ID, Status: model.PlanResolving}))
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{ID: duel.ID, Status: model.PlanResolving}))
	require.NoError(t, q.SetPlanResolutionData(ctx, dbgen.SetPlanResolutionDataParams{
		ID:             festivity.ID,
		ResolutionData: new(`{"festivity":{"pending_duel_plan_id":` + itoa(duel.ID) + `}}`),
	}))

	got := titleRowsFor(t, store, tg.Players[0].AccountID)[tg.Game.ID]
	require.Equal(t, "propose_duel", *got.PlanType)
}

// TestTitlePlan_TwoPlansOnARow_FollowsTheFollowScene walks a row with two plans
// through the first plan's follow-scene turn. The table stays on the first plan
// until its setter passes focus (ComputeRowState's step 4.5 runs before "plan
// pending"), and only then does the title move to the second. The profile list
// and the row state are asserted together at every step: they are one answer.
func TestTitlePlan_TwoPlansOnARow_FollowsTheFollowScene(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := t.Context()

	tg := newTestGame(t, q, 2, gametest.WithCurrentRow(6))
	game, first, second := tg.Game, tg.Players[0], tg.Players[1]
	require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{ID: game.ID, FocusPlayerID: &first.ID}))

	planA := createPlanOnRow(t, q, &game, &first, model.PlanSeekAnswers, model.CategoryKnowledge, 6)
	planB := createPlanOnRow(t, q, &game, &second, model.PlanSpreadPropaganda, model.CategoryEsteem, 6)
	require.NoError(t, q.SetPlanRowAndOrder(ctx, dbgen.SetPlanRowAndOrderParams{ID: planB.ID, RowNumber: new(int16(6)), RowOrder: 1}))
	require.NoError(t, q.SetPlanResult(ctx, dbgen.SetPlanResultParams{ID: planA.ID, Result: new("make")}))

	both := func(want model.PlanType, why string) {
		t.Helper()
		rs, err := ComputeRowState(ctx, q, game.ID)
		require.NoError(t, err)
		require.NotNil(t, rs.TitlePlanType, why)
		require.Equal(t, want, *rs.TitlePlanType, "row state: "+why)
		listed := titleRowsFor(t, store, first.AccountID)[game.ID]
		require.NotNil(t, listed.PlanType, why)
		require.Equal(t, string(want), *listed.PlanType, "profile list: "+why)
	}

	both(model.PlanSeekAnswers, "plan 1 has resolved and its follow-scene is owed")

	scene, err := q.CreateScene(ctx, dbgen.CreateSceneParams{
		GameID: game.ID, RowNumber: 6, FocusPlayerID: first.ID,
		LocationCustom: new("the long gallery"), TimeElapsed: new(model.TimeHours),
		Prompt: "after the inquiry", ResolvedPlanID: &planA.ID,
	})
	require.NoError(t, err)
	both(model.PlanSeekAnswers, "the follow-scene is being played")

	require.NoError(t, q.EndScene(ctx, scene.ID))
	both(model.PlanSeekAnswers, "the follow-scene ended and its setter still holds focus (prepare a plan)")

	require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{ID: game.ID, FocusPlayerID: &second.ID}))
	both(model.PlanSpreadPropaganda, "focus has passed: plan 2 is next")
}
