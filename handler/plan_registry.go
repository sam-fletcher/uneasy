package handler

// handler/plan_registry.go — Thin re-exports of the game package types.
//
// Pure domain data/metadata types (ResolutionData, PlanMetadata, the per-plan
// *ResolutionData structs, …) live in the game package. These aliases keep the
// handler package's internal references unqualified. The storage-coupled plan
// contract (PlanHandler, PlanDeps, ValidationContext, the registry,
// saveResolutionData) lives in plan_contract.go in this package.

import (
	"errors"
	"log/slog"

	dbgen "uneasy/db/gen"
	"uneasy/game"
)

// Type aliases — handler-internal code can use these unqualified.
type (
	ResolutionData                   = game.ResolutionData
	DraftChoice                      = game.DraftChoice
	Choice                           = game.Choice
	KeptSecret                       = game.KeptSecret
	LiaiseResolutionData             = game.LiaiseResolutionData
	LiaisePhase                      = game.LiaisePhase
	SpreadPropagandaResolutionData   = game.SpreadPropagandaResolutionData
	SpreadRumorsResolutionData       = game.SpreadRumorsResolutionData
	MakeDemandsResolutionData        = game.MakeDemandsResolutionData
	ProposeDecreeResolutionData      = game.ProposeDecreeResolutionData
	MakeIntroductionsResolutionData  = game.MakeIntroductionsResolutionData
	MIMarOutcome                     = game.MIMarOutcome
	MIArrival                        = game.MIArrival
	DraftPeer                        = game.DraftPeer
	ExchangeCourtiersResolutionData  = game.ExchangeCourtiersResolutionData
	ChronicleHistoriesResolutionData = game.ChronicleHistoriesResolutionData
	ChronicleStep                    = game.ChronicleStep
	SeekAnswersResolutionData        = game.SeekAnswersResolutionData
	DuelResolutionData               = game.DuelResolutionData
	DuelPhase                        = game.DuelPhase
	MakeWarResolutionData            = game.MakeWarResolutionData
	FestivityResolutionData          = game.FestivityResolutionData
)

// Resolution data read helpers — used throughout handler plan files.
// (ParseResolutionData stays in game/; saveResolutionData lives in
// plan_contract.go because it performs the DB write.)
//
// They are the lenient read: whatever of the blob decodes is returned. A plan
// saved before one of its fields changed type (make_mar_choices was once a list
// of strings) is mistyped but fine — the rest decodes, as it always did. A blob
// that isn't JSON at all is a different matter, and three things keep it from
// doing damage:
//
//   - every plan route refuses it up front (requirePlanAccess), before it
//     changes anything, so a plan view of such a plan is a 500, not an empty plan;
//   - saveResolutionData refuses to write back a value that came from one, so a
//     read-modify-write elsewhere can't overwrite the stored state with an
//     empty copy;
//   - read-only callers outside the routes (the Waiting On computation, log
//     text) degrade to what decoded rather than taking the table page down.

// loadResolutionData reads a plan's resolution_data when the caller has only the
// column. It logs a blob that isn't JSON; with no plan in hand it can't say which
// one, so a caller that holds the plan should use loadPlanResolutionData, which
// can (and also reports an older field shape).
func loadResolutionData(raw *string) ResolutionData {
	rd, err := game.ParseResolutionData(raw)
	if errors.Is(err, game.ErrResolutionDataMalformed) {
		logResolutionDataProblem(nil, err)
	}
	return rd
}

// loadPlanResolutionData is loadResolutionData for a caller that holds the plan:
// the log line names it. Use it on paths that run again and again, like the
// row-state computation, where an unattributed error can't be traced.
func loadPlanResolutionData(plan *dbgen.Plan) ResolutionData {
	rd, err := game.ParseResolutionData(plan.ResolutionData)
	logResolutionDataProblem(plan, err)
	return rd
}

// logResolutionDataProblem reports what ParseResolutionData found: an Error for
// a blob that isn't JSON, a Warn for an older field shape (nothing is lost by
// reading it, but someone may want to know). plan may be nil.
func logResolutionDataProblem(plan *dbgen.Plan, err error) {
	if err == nil {
		return
	}
	attrs := []any{"err", err}
	if plan != nil {
		attrs = append(attrs, "plan_id", plan.ID, "game_id", plan.GameID, "plan_type", plan.PlanType)
	}
	if errors.Is(err, game.ErrResolutionDataMalformed) {
		slog.Default().Error("plan resolution_data is not valid JSON; reading it as empty", attrs...)
		return
	}
	slog.Default().Warn("plan resolution_data has a field in an older shape; reading what fits", attrs...)
}
