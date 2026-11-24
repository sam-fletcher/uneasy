# Hand-over: before/after-board log-coverage guard

Branch `claude/board-log-guard-c15f94`.

## What changed and why

Two static/per-plan checks already look for missing action-log posts, but
neither can see a post that sits on a branch that doesn't run. This adds a
runtime guard: `serveGuarded` in
[handler/log_coverage_board_integration_test.go](../../../handler/log_coverage_board_integration_test.go)
brackets every mutating (non-GET/HEAD) request an integration harness sends with
two snapshots of the board. If a game's board changed and that game gained no
new action-log row, the test fails and names the route and the change
(`marginalia#2 changed: "… text=owes a debt" -> "… text=debt forgiven"`).

- **Wired into** the shared plan-lifecycle harness (`planLifecycle.post`, the
  route most of the suite goes through) and the other harnesses that drive
  board-changing routes: ending vote, assets chat log, posts, rolls, Shake-Up
  rolls/reaction gate, Make Demands, prologue closing/set-asides, focus pass,
  laws/rumors, length caps, title claim, turn, plan tokens. About 2,600
  requests per suite run are guarded.
- **One query per snapshot** (a UNION over the tables below), on the shared
  uncounted `harnessPool`, so the query-budget tests' statement counts are
  untouched.
- **Allow-list:** `silentBoardRoutes`, keyed `"METHOD /path"` with numeric
  segments as `{id}`, each entry with a reason and optionally narrowed to
  snapshot kinds. **It is empty today** — see below.
- **Self-test:** `TestBoardGuardCatchesASilentChange` proves a silent change
  fails, the same change with a post passes, and an allow-list entry is
  honoured only for its listed kinds.
- **What is snapshotted:** assets (owner, `is_destroyed`, `is_main_character`,
  name), marginalia (position, text, title, `is_torn`), secrets (existence and
  `is_revealed`, never text), rankings (player/category/rank), laws (text,
  addendum, `is_active`), rumors (text, target asset, `is_active`).
- **Left out, and why:** `is_leveraged` (a token flipped on every roll and
  refresh, not story); `secret_visibility` and secret text (access, and text is
  gated); plans, dice rolls, wars, duels, scenes and `games` (current_row, phase)
  rows (the machinery — each posts through its own code, and what it does to the
  board is caught through the tables above); `display_order` and `rev` on laws
  and rumors (reordering and bookkeeping).
- **What counts as a post:** a new `scene_posts` row with a `system_code`. A
  player's chat message doesn't. Prologue (row 0) and Shake-Up (row 14) need no
  phase rule: `logRow` returns nil there, so posts land unanchored rather than
  dropped; a board change in those phases must still post.
- Tracker: deleted `next/log-coverage-board-guard.md` and
  `next/one-main-character-index.md` (stale: migration 057 built that index).
  One line each added to `docs/DEVELOPMENT.md` and the `work-session` skill so
  sessions know the guard exists.

## How it was verified

Against this worktree's code, on a private database:

- `make check-fast` passes.
- `make test-integration` passes with the guard on (final run, all packages).
- **Real gaps found: none.** The whole handler suite ran with the guard on and
  nothing failed, so there are no `TODO(log-gap)` allow-list entries and no
  one-line log fixes. Of ~2,600 guarded requests, 325 changed the board (e.g.
  `keep-secret`, `make-choice`, `guest-choice`, `POST /assets`,
  `shake-up/commit`, `break-resource`, `enact-law`), and each also posted.
- **The guard bites.** I commented out the `EmitMarginaliaEdited` call in
  `handler/assets_marginalia.go` and re-ran `-run 'Marginalia|Assets|Plan'`:
  `TestChatLog_MarginaliaAddedAndEdited` failed with
  `PUT /api/assets/{id}/marginalia/{id} (HTTP 200) changed game 1's board and
  wrote no action-log post: marginalia#2 changed …`. Reverted; not committed.
- **Run time** (`go test -tags=integration -p 1 ./handler/`, same machine, same
  database): **211 s with the guard off, 215 s with it on** (~2%). One earlier
  guarded run took 251 s while the machine was busy; I take that as noise.
- Not run: e2e (Playwright) — no frontend or server code changed.

## Behaviour changes to look at before merging

None in the app: test code only, plus one doc line and one skill line.

Limits of the guard worth knowing before relying on it:

- **Coarse by design.** *Any* new system post for the game satisfies it. A
  request that changes the board, forgets its own post, but writes an unrelated
  one (a plan's `plan.resolving`, a row-advance post) passes. Tightening it to a
  kind→`system_code` map (asset changes need an `asset.*` or plan-narration
  post, etc.) is possible but needs the owner's wording rules; not done.
- **Only requests sent through the wired harnesses are checked.** Setup done
  directly through `q.` calls, and the account/lobby/feedback/activity tests, are
  not. A new harness must call `serveGuarded` in place of `router.ServeHTTP`.
- A failing request that rolled its writes back leaves no change and is not
  flagged; one that half-wrote then errored is, which is intended.

## Open questions for the owner

None.

## Noticed, not fixed

- `gofmt -l handler/` lists `plan_make_war_integration_test.go`,
  `push_notifications_integration_test.go` and `wait_state_integration_test.go`
  (already on `main`; not touched).
