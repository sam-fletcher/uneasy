// Package handler implements the Uneasy HTTP API: request parsing,
// authorization, persistence via db/gen, and WebSocket broadcast via hub.
// Game rules themselves live in the game package; handlers are the
// imperative shell around that functional core (see
// adr/TESTABILITY_AND_ENGINE_DECOUPLING_PLAN.md).
//
// The package is large (one of the biggest in the repo) but organized by a
// consistent file-naming convention rather than sub-packages, so that
// grep-by-prefix and your editor's fuzzy-file-open both work as navigation.
// Rough map, in the order a player would hit these phases:
//
//   - accounts.go, sessions.go, password_resets.go — signup/login/auth,
//     independent of any one game.
//   - tables.go, dev.go, config.go — creating/joining a table; dev-only
//     seed/login shortcuts gated behind UNEASY_DEV.
//   - prologue*.go — the Prologue phase (card picks, ranking, refunds).
//   - phases.go, turn.go, row_state.go, wait_state.go — cross-phase
//     bookkeeping: which phase/row a game is on and who it's waiting on
//     (ComputeRowState / ComputeWaitState feed the frontend's "Waiting On"
//     bar and phase gating).
//   - scenes.go, plan_scenes.go — scene setup/detail shared by all phases.
//   - assets*.go, asset_effects.go, asset_suggestions.go — the asset/
//     marginalia/secret data model shared by every phase.
//   - rolls*.go, dice_roll_guard.go — dice roll creation, staging, and the
//     one-open-roll-per-game invariant.
//   - plan_contract.go, plan_registry.go, plan_access.go,
//     plan_validation.go, plans.go, plan_resolution.go, eligibility.go,
//     demands.go — the shared PlanHandler contract (see game/plan.go for the
//     pure metadata types) plus the generic plan lifecycle that isn't
//     specific to one plan type: plans.go creates/lists/checks eligibility,
//     plan_resolution.go resolves/choice/completes.
//   - plan_<name>*.go — one (or a few) files per one of the 12 Main Event
//     plans, each implementing PlanHandler. Multi-file plans split by
//     concern, e.g. plan_host_festivity.go (contract) /
//     _options.go (host spoils/guest IOUs) / _routes.go (HTTP routes).
//   - shake_up*.go — the Shake-Up phase: round/roller lifecycle
//     (shake_up.go), roll open/finalize (shake_up_rolls.go), the
//     announce/adjust/pass/commit "pay or abandon" spend flow
//     (shake_up_pay_abandon.go, see adr/008-shake-up-spend-commitment.md),
//     and take/break/bump/claim-title effects (shake_up_effects.go).
//   - monarch.go, wars.go, laws_rumors.go, reveals.go, ranking.go,
//     rankings.go, endgame.go, tone.go — later-game and cross-plan
//     mechanics (succession, war state, decree/rumor records, secret
//     reveals, standings, ending a game, table tone settings).
//   - posts.go, system_posts.go, record.go, ws.go, notify.go,
//     push_notifications.go, push_subscriptions.go — the unified chat/
//     action-log feed, its WebSocket event types, and web-push delivery.
//   - feedback.go — in-app feedback form submission.
//   - helpers.go, respond.go — small generic helpers (JSON response
//     writers, field parsing) used throughout the package.
//
// helpers.go and respond.go have no dependency on anything else in this
// package, so they're the safe starting point if this package is ever
// split into real sub-packages.
package handler
