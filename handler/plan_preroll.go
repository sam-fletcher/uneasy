package handler

// handler/plan_preroll.go — the pre-roll narration step shared by Spread
// Propaganda and Spread Rumors (adr/audit/FINDINGS.md Tier 3 #10).
//
// The rulebook gives both plans a pre-roll ("Summarize the message you want the
// public to take from your propaganda…", "State the rumor you plan to spread and
// explain how you're spreading it"). It is a gate for a reason, not for staging:
// the other players read it to decide whether to help or interfere, so it has to
// be on the table BEFORE the roll opens (DESIGN_PRINCIPLES.md principle 1), and it
// is the commitment moment for what the prepare-time note only sketched
// (principle 2).
//
// Shape, as in Seek Answers: OnResolve parks the plan in 'resolving' with no roll
// and no PreRoll flag; the preparer's cast-roll route records and posts the
// narration, creates the roll and flips the flag. The preparer casts even when a
// Make Demands perform_steps winner holds the post-roll steps — the narration is
// the preparer's own account of what they prepared, like Seek Answers'.

import (
	"context"
	"errors"
	"net/http"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// requirePreRollOpen is the shared preamble of the cast-roll routes: a resolving
// plan of the wanted type, the preparer (strictly — never a demand winner), and
// no roll yet. Writes the HTTP error and returns ok=false otherwise.
func requirePreRollOpen(
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	wantType model.PlanType,
) (*dbgen.Plan, *dbgen.Player, bool) {
	plan, player, ok := requirePlanForExtraRoute(w, r, q, wantType)
	if !ok {
		return nil, nil, false
	}
	if player.ID != plan.PreparerID {
		respondErr(w, http.StatusForbidden, "only the preparer can use this route")
		return nil, nil, false
	}
	// The roll's existence is the authority — it also covers plans that rolled
	// before the step existed, which carry no flag.
	if existing, err := q.GetDiceRollByPlanID(r.Context(), &plan.ID); err == nil && existing.ID != 0 {
		respondErr(w, http.StatusConflict, "the dice have already been cast")
		return nil, nil, false
	}
	return plan, player, true
}

// openPreRollRoll creates the plan's roll once its pre-roll narration is in.
func openPreRollRoll(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	difficulty int16,
) (*dbgen.DiceRoll, error) {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return nil, err
	}
	return createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
}

// postPlanNarration publishes a player's narration for a plan step as their own
// scene post, anchored to the plan's row. It is the "posted (and logged)" half of
// a narration step: the words stay the player's, in the unified chat feed, where
// the table reads them. A failed post is not worth failing the step over (the text
// is also kept in resolution_data), so errors are swallowed as Seek Answers does.
func postPlanNarration(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, authorID int64, body string) {
	post, err := deps.Q.CreatePlayerMessage(ctx, dbgen.CreatePlayerMessageParams{
		GameID:    plan.GameID,
		AuthorID:  &authorID,
		Body:      body,
		RowNumber: plan.RowNumber,
		PlanID:    &plan.ID,
	})
	if err != nil {
		return
	}
	if h, ok := deps.Manager.Get(plan.GameID); ok {
		h.BroadcastEvent(model.EventScenePostCreated, model.ScenePostCreatedPayload{Post: post})
	}
}

// ChoiceGate is an optional PlanHandler capability: a server-side precondition on
// make-choice for steps that must come first. MakeChoice consults it before
// anything is applied and answers with the returned error's status (a *httpError)
// or 409. roll is the plan's roll, nil when it has none — the pre-roll step is over
// exactly when it exists. By the time a gate runs the roll's outcome is settled:
// MakeChoice itself refuses every plan's choice while a roll is still open
// (errDiceNotSettled), so a gate never has to repeat that.
//
// It exists because make-choice otherwise accepts a submission with no roll at
// all, which would let a stale or hand-rolled client commit a result past a
// narration step nobody wrote.
type ChoiceGate interface {
	CheckChoice(
		ctx context.Context,
		q *dbgen.Queries,
		plan *dbgen.Plan,
		player *dbgen.Player,
		resData *ResolutionData,
		roll *dbgen.DiceRoll,
		result string,
	) error
}

// errDiceNotSettled is the refusal for a make/mar submitted while the roll is
// still open. MakeChoice applies it to every plan that has a roll.
var errDiceNotSettled = errors.New("the dice haven't settled yet — wait for the roll before choosing options")
