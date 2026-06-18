package handler

// handler/plan_registry.go — Thin re-exports of the game package types.
//
// Pure domain data/metadata types (ResolutionData, PlanMetadata, the per-plan
// *ResolutionData structs, …) live in the game package. These aliases keep the
// handler package's internal references unqualified. The storage-coupled plan
// contract (PlanHandler, PlanDeps, ValidationContext, the registry,
// saveResolutionData) lives in plan_contract.go in this package.

import "uneasy/game"

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
	ExchangeCourtiersResolutionData  = game.ExchangeCourtiersResolutionData
	ChronicleHistoriesResolutionData = game.ChronicleHistoriesResolutionData
	SeekAnswersResolutionData        = game.SeekAnswersResolutionData
	DuelResolutionData               = game.DuelResolutionData
	DuelPhase                        = game.DuelPhase
	MakeWarResolutionData            = game.MakeWarResolutionData
	FestivityResolutionData          = game.FestivityResolutionData
)

// Resolution data read helper — used throughout handler plan files.
// (loadResolutionData stays in game/; saveResolutionData lives in
// plan_contract.go because it performs the DB write.)
var loadResolutionData = game.LoadResolutionData
