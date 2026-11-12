package handler

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// The pick is a function of the row state's own snapshot, so it is tested the
// way the gates are: by building the snapshot. No database.

const (
	focusA int64 = 1 // the player who set the first plan's follow-scene
	focusB int64 = 2
)

func tPlan(id int64, t model.PlanType, status model.PlanStatus, row *int16, order int16) dbgen.Plan {
	return dbgen.Plan{ID: id, PlanType: t, Status: status, RowNumber: row, RowOrder: order}
}

//go:fix inline
func festivityDataPointingAt(duelID string) *string {
	s := `{"festivity":{"pending_duel_plan_id":` + duelID + `}}`
	return &s
}

// followScene is the turn-scene a focus player sets after plan 1 resolves,
// ended or not.
func followScene(focus int64, ended bool) dbgen.Scene {
	planID := int64(1)
	return dbgen.Scene{
		ID: 101, FocusPlayerID: focus, Kind: model.SceneKindTurn,
		ResolvedPlanID: &planID, EndedAt: pgtype.Timestamptz{Valid: ended},
	}
}

func TestTitlePlan(t *testing.T) {
	const row int16 = 6
	game := func(focus int64) *dbgen.Game {
		return &dbgen.Game{CurrentRow: row, FocusPlayerID: &focus}
	}
	seek := tPlan(1, model.PlanSeekAnswers, model.PlanResolved, new(row), 0)
	propaganda := tPlan(2, model.PlanSpreadPropaganda, model.PlanPending, new(row), 1)

	tests := []struct {
		name   string
		game   *dbgen.Game
		plans  []dbgen.Plan
		recent *dbgen.Plan
		scenes []dbgen.Scene
		want   *model.PlanType
	}{
		{
			name: "a row with no plan", game: game(focusA), want: nil,
		},
		{
			name: "the plan being resolved", game: game(focusA),
			plans: []dbgen.Plan{tPlan(1, model.PlanSeekAnswers, model.PlanResolving, new(row), 0)},
			want:  new(model.PlanSeekAnswers),
		},
		{
			name: "a pending plan before kickoff", game: game(focusA),
			plans: []dbgen.Plan{tPlan(1, model.PlanProposeDecree, model.PlanPending, new(row), 0)},
			want:  new(model.PlanProposeDecree),
		},
		{
			name: "two pending plans: the lowest row_order", game: game(focusA),
			plans: []dbgen.Plan{
				tPlan(1, model.PlanMakeIntroductions, model.PlanPending, new(row), 0),
				tPlan(2, model.PlanHostFestivity, model.PlanPending, new(row), 1),
			},
			want: new(model.PlanMakeIntroductions),
		},
		{
			name: "a resolving plan on another row is still the one being played", game: game(focusA),
			plans: []dbgen.Plan{
				tPlan(1, model.PlanMakeDemands, model.PlanResolved, new(row), 0),
				tPlan(2, model.PlanExchangeCourtiers, model.PlanResolving, new(int16(9)), 0),
			},
			want: new(model.PlanExchangeCourtiers),
		},
		{
			name: "a festivity paused under the duel it spawned names the duel", game: game(focusA),
			plans: []dbgen.Plan{
				{ID: 1, PlanType: model.PlanHostFestivity, Status: model.PlanResolving, RowNumber: new(row),
					ResolutionData: festivityDataPointingAt("2")},
				tPlan(2, model.PlanProposeDuel, model.PlanResolving, new(row), 1),
			},
			want: new(model.PlanProposeDuel),
		},
		{
			name: "a festivity whose duel has ended is the festivity again", game: game(focusA),
			plans: []dbgen.Plan{
				{ID: 1, PlanType: model.PlanHostFestivity, Status: model.PlanResolving, RowNumber: new(row),
					ResolutionData: festivityDataPointingAt("2")},
				tPlan(2, model.PlanProposeDuel, model.PlanResolved, new(row), 1),
			},
			want: new(model.PlanHostFestivity),
		},
		{
			name: "two plans resolving at once, neither suspended: the first", game: game(focusA),
			plans: []dbgen.Plan{
				tPlan(1, model.PlanSpreadPropaganda, model.PlanResolving, new(row), 0),
				tPlan(2, model.PlanSpreadPropaganda, model.PlanResolving, new(row), 1),
			},
			want: new(model.PlanSpreadPropaganda),
		},

		// Two plans on one row: the table stays on the first through its
		// follow-scene turn, then moves to the second (followSceneGate, step 4.5).
		{
			name: "plan 1 resolved, its follow-scene not yet set: still plan 1", game: game(focusA),
			plans: []dbgen.Plan{seek, propaganda}, recent: &seek,
			want: new(model.PlanSeekAnswers),
		},
		{
			name: "its follow-scene is being played: still plan 1", game: game(focusA),
			plans: []dbgen.Plan{seek, propaganda}, recent: &seek,
			scenes: []dbgen.Scene{followScene(focusA, false)},
			want:   new(model.PlanSeekAnswers),
		},
		{
			name:   "the follow-scene ended and its setter holds focus (prepare a plan): still plan 1",
			game:   game(focusA),
			plans:  []dbgen.Plan{seek, propaganda},
			recent: &seek,
			scenes: []dbgen.Scene{followScene(focusA, true)},
			want:   new(model.PlanSeekAnswers),
		},
		{
			name: "the setter passed focus: the gate clears and the table moves to plan 2", game: game(focusB),
			plans: []dbgen.Plan{seek, propaganda}, recent: &seek,
			scenes: []dbgen.Scene{followScene(focusA, true)},
			want:   new(model.PlanSpreadPropaganda),
		},
		{
			name: "a plan resolving beats a follow-scene gate", game: game(focusA),
			plans: []dbgen.Plan{
				seek, tPlan(2, model.PlanSpreadPropaganda, model.PlanResolving, new(row), 1),
			},
			recent: &seek,
			scenes: []dbgen.Scene{followScene(focusB, true)},
			want:   new(model.PlanSpreadPropaganda),
		},

		// With nothing pending, the row keeps the name of the plan it played.
		{
			name: "the only plan resolved and its follow-scene is running", game: game(focusA),
			plans: []dbgen.Plan{seek}, recent: &seek,
			scenes: []dbgen.Scene{followScene(focusA, false)},
			want:   new(model.PlanSeekAnswers),
		},
		{
			name: "every plan resolved and the turn passed on: the row keeps the last one's name", game: game(focusB),
			plans: []dbgen.Plan{seek}, recent: &seek,
			scenes: []dbgen.Scene{followScene(focusA, true)},
			want:   new(model.PlanSeekAnswers),
		},

		// What is never named.
		{
			name: "plans on other rows are not this row's", game: game(focusA),
			plans: []dbgen.Plan{
				tPlan(1, model.PlanSeekAnswers, model.PlanResolved, new(int16(5)), 0),
				tPlan(2, model.PlanMakeWar, model.PlanPending, new(int16(9)), 0),
			},
			want: nil,
		},
		{
			name: "a plan waiting on its delay reveal has no row", game: game(focusA),
			plans: []dbgen.Plan{
				tPlan(1, model.PlanMakeWar, model.PlanPending, nil, 0),
				tPlan(2, model.PlanClandestinelyLiaise, model.PlanPending, nil, 0),
			},
			want: nil,
		},
		{
			name: "a plan that fell through is never named", game: game(focusA),
			plans: []dbgen.Plan{tPlan(1, model.PlanMakeWar, model.PlanCancelled, new(row), 0)},
			want:  nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := titlePlan(tc.game, tc.plans, tc.recent, tc.scenes)
			if tc.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tc.want, *got)
		})
	}
}

// Row 13 under an Explosive Finale has no scenes, so no plan there owes a
// follow-scene turn (followSceneGate stands down): the second plan is named the
// moment the first resolves.
func TestTitlePlan_FinaleRowHasNoFollowScenes(t *testing.T) {
	finale := EndingModeExplosiveFinale
	game := &dbgen.Game{CurrentRow: publicRecordRowCount, EndingMode: &finale}
	row := new(int16(publicRecordRowCount))
	first := tPlan(1, model.PlanSeekAnswers, model.PlanResolved, row, 0)
	second := tPlan(2, model.PlanSpreadPropaganda, model.PlanPending, row, 1)

	got := titlePlan(game, []dbgen.Plan{first, second}, &first, nil)
	require.NotNil(t, got)
	require.Equal(t, model.PlanSpreadPropaganda, *got)
}
