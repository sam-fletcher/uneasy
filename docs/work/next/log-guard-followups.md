---
title: "Ideas, not yet approved: tighten the board log guard"
order: 87
---

Two follow-ups to the runtime log guard (`serveGuarded` in
`handler/log_coverage_board_integration_test.go`). Both are ideas — the owner
hasn't approved either, and the first needs log-wording rules only the owner can
give.

- **Match the post kind to the change kind.** Today any new system post for the
  game satisfies the guard, so a missing asset post can hide behind a
  `plan.resolving` or phase post in the same request. A map from change kind
  (asset, marginalia, ranking, law, rumor, secret, token) to the `system_code`s
  that may account for it would close that.
- **Guard the direct-call effect tests.** Tests that apply effects by calling
  the helpers directly never go through `serveGuarded`, so the guard skips them:
  `shake_up_effects_integration_test.go` (Shake-Up effects are mostly checked
  this way), `asset_effects_integration_test.go` and
  `plan_aftermath_integration_test.go`. Bracket those calls with the same
  snapshot diff.
