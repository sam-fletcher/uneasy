package handler

import (
	"context"
	"fmt"
	"strings"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// The sentences that end a plan's closing post: who currently has no main
// character, and what comes next and who acts. A plan used to resolve with one bare
// line ("X marred."), and with the confirming click gone nothing else told a player
// who had been away that this part was over or whose move it was.
//
// Who acts next is read from the server's own wait state (ComputeRowState), not
// written per plan: it already names a Make Demands stand-in, a parent plan's
// owed step or a replacement main character correctly, and a hand-written
// sentence per plan would drift from it. Everything here is a player's name and a
// generic phrase, so the post is safe to show the whole table: no secret text and
// no gated detail.

// nextPhrases is what the players a RowState names still have to do, phrased to
// follow "Next: <names> to ". The two plan kinds are handled in rowStateNextPhrase
// because they name a plan.
var nextPhrases = map[model.RowStateKind]string{
	model.RowStateAwaitDiceRoll:                   "take part in the dice roll",
	model.RowStateAwaitEndgameVote:                "vote on how the game ends",
	model.RowStateAwaitSurrenderClaim:             "claim a surrendered asset",
	model.RowStateAwaitBattleCost:                 "pay the cost of battle",
	model.RowStateAwaitMainCharacterChoice:        "choose a new main character before play goes on",
	model.RowStateAwaitDelayReveal:                "reveal their delay die",
	model.RowStateAwaitDemandCounter:              "answer the Make Demands",
	model.RowStateAwaitDemandDraftPick:            "pick in the Make Demands draft",
	model.RowStateAwaitDemandLeverage:             "settle the Make Demands leverage",
	model.RowStateAwaitDemandRetarget:             "keep or change the plan's target",
	model.RowStateAwaitFestivityGuestTurn:         "take their turn at the festivity",
	model.RowStateAwaitFestivityChallengeResponse: "answer the festivity challenge",
	model.RowStateAwaitDuelStaking:                "stake for the duel",
	model.RowStateAwaitDuelBout:                   "fight the next bout",
	model.RowStateAwaitTakeConsent:                "answer a request to take an asset",
	model.RowStateAwaitQuestionAnswer:             "answer a question",
	model.RowStateLiaiseResolving:                 "carry on with the liaison",
	model.RowStateAwaitCourtierResponse:           "respond to the exchange",
	model.RowStateAwaitPlanReAim:                  "choose a new target",
	model.RowStateAwaitIntroductionsMarginalia:    "write a newcomer's marginalia",
	model.RowStateAwaitChronicleChoices:           "make their Chronicle Histories choice",
	model.RowStateAwaitCounterPropaganda:          "answer the counter-propaganda",
	model.RowStateSceneSetting:                    "set the scene",
	model.RowStateSceneActive:                     "play out the scene",
	// The two plan kinds are phrased in rowStateNextPhrase (they name a plan);
	// these two have no one to name.
	model.RowStatePlanPending:       "",
	model.RowStatePlanResolving:     "",
	model.RowStateFinaleRowComplete: "",
	model.RowStatePhaseNotMainEvent: "",
	model.RowStatePostSceneAction:   "prepare a plan or refresh assets",
}

// rowStateNextPhrase returns the phrase for a RowState, or "" when there is
// nothing useful to say (a kind with no actor to name, or one we cannot phrase).
func rowStateNextPhrase(ctx context.Context, q *dbgen.Queries, rs model.RowState) string {
	if rs.Kind == model.RowStatePlanPending || rs.Kind == model.RowStatePlanResolving {
		label := "the next plan"
		if rs.PlanID != nil {
			if other, err := q.GetPlanByID(ctx, *rs.PlanID); err == nil {
				label = planLabel(other.PlanType)
			}
		}
		if rs.Kind == model.RowStatePlanPending {
			return "resolve " + label
		}
		return "carry on with " + label
	}
	return nextPhrases[rs.Kind]
}

// planAftermath returns the trailing sentences for a resolved plan's closing
// post: who currently has no main character (usually this plan's doing, though the
// table keeps no per-plan record of it), then "Next: <who> to <what>." drawn from
// the live wait state. "" when there is nothing to add.
func planAftermath(ctx context.Context, q *dbgen.Queries, plan dbgen.Plan) string {
	var parts []string

	var lost []int64
	if ids, err := q.ListPlayersMissingMainCharacter(ctx, plan.GameID); err != nil {
		loggerFromContext(ctx).WarnContext(ctx, "closing post: could not list lost main characters",
			"plan_id", plan.ID, "err", err)
	} else {
		lost = ids
	}
	rs, rsErr := ComputeRowState(ctx, q, plan.GameID)

	names := func(ids []int64) string {
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			out = append(out, playerDisplayName(ctx, q, id))
		}
		return joinWithAnd(out)
	}

	// The main-character gate names exactly the players who lack one, so when it is
	// the wait state the loss and the next step are one sentence, built from the
	// wait state itself so a failed lookup above can't leave it nameless.
	mcGate := rsErr == nil && rs.Kind == model.RowStateAwaitMainCharacterChoice
	if len(lost) > 0 && !mcGate {
		verb := "has no main character and will choose a replacement"
		if len(lost) > 1 {
			verb = "have no main character and will each choose a replacement"
		}
		parts = append(parts, fmt.Sprintf("%s %s.", names(lost), verb))
	}

	if rsErr == nil && len(rs.ActingPlayerIDs) > 0 {
		if phrase := rowStateNextPhrase(ctx, q, rs); phrase != "" {
			if mcGate {
				lostLine := "lost their main character"
				if len(rs.ActingPlayerIDs) > 1 {
					lostLine = "lost their main characters"
				}
				parts = append(parts, fmt.Sprintf("%s %s. Next: %s to %s.",
					names(rs.ActingPlayerIDs), lostLine, names(rs.ActingPlayerIDs), phrase))
			} else {
				parts = append(parts, fmt.Sprintf("Next: %s to %s.", names(rs.ActingPlayerIDs), phrase))
			}
		}
	}
	return strings.Join(parts, " ")
}
