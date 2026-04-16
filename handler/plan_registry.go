package handler

// handler/plan_registry.go — Thin re-exports of the game package types.
//
// Domain types (PlanHandler, ResolutionData, etc.) now live in the game
// package. These aliases keep the handler package's internal references
// compiling without a mass-rename in one step.

import "uneasy/game"

// Type aliases — handler-internal code can use these unqualified.
type (
	PlanHandler       = game.PlanHandler
	OnPreparer        = game.OnPreparer
	PlanMetadata      = game.PlanMetadata
	PlanDeps          = game.PlanDeps
	ValidationContext = game.ValidationContext
	ResolutionData    = game.ResolutionData
	DraftChoice       = game.DraftChoice
)

// Registry delegates — handler-internal code calls these unqualified.
var (
	RegisterPlan = game.RegisterPlan
	GetHandler   = game.GetHandler
	AllHandlers  = game.AllHandlers
)

// Resolution data helpers — used throughout handler plan files.
var (
	loadResolutionData = game.LoadResolutionData
	saveResolutionData = game.SaveResolutionData
)

// Pure game-rule helpers — aliased so handler code can call them unqualified.
var (
	playerRankInCategory = game.PlayerRankInCategory
	playerHasPeers       = game.PlayerHasPeers
	checkPlanEligible    = game.CheckPlanEligible
	hasEsteemLockout     = game.HasEsteemLockout
)
