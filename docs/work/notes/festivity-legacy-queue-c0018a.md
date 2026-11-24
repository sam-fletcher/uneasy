# Hand-over: remove the legacy PendingHostMars queue

## What changed and why

Before 2026-10-28 a guest who insisted on a mar chose the option, and it waited
in `PendingHostMars` for the host to pick an asset. Since then the host chooses
the option, and the insistence waits in `UnchosenHostMars`. The old queue was
kept only so a game in flight across the change could finish. The owner's
read-only production query on 2026-11-23 found no open Host Festivity holding
`pending_host_mars`, so this removes the dead path.

- **Server.** `game/festivity.go`: the `PendingHostMars` field,
  `RemovePendingHostMar`, and `MarNeedsHostResolution` (its only caller was the
  legacy branch) are gone. `HostMarsOwed` counts only unchosen mars.
  `handler/plan_host_festivity_routes.go`: the legacy branch is gone. A
  resolve-host-mar request with no `insister_id` is refused with 400. The
  response no longer carries `pending_host_mars`.
- **Frontend.** The legacy type, return type, props and branches are removed
  from the listed files. `HostPendingMar.svelte` takes
  `insisterID = unchosenHostMars[0] ?? null`, has no legacy option, and always
  sends `insister_id`. `resolveHostMar` requires `insister_id`.
- **Tests.** The legacy load test and the legacy-settle integration step are
  removed. The legacy-settle step is replaced by a refusal check. The
  row-state test's legacy `PendingHostMars` steps are removed (the
  UnchosenHostMars step still covers "an insisted mar keeps the host in
  Waiting On").
- **Tracker.** The first bullet of `docs/work/next/code-cleanup-leftovers.md`
  is removed. The rest of the file is untouched.

Nothing else about how Host Festivity plays has changed.

## How it was verified

Against this worktree's own code:

- `make check-fast` exits 0. Frontend `svelte-check` reports 0 errors. Vitest
  passes 2298 tests.
- `make test-integration` passes on a private database
  (`uneasy_claude_festivity_legacy_queue_c0018a`, dropped afterwards). The
  `cmd/server` package needs the embedded frontend, so `make build` ran first.
- Host Festivity e2e (`frontend/tests/e2e/host_festivity.spec.ts`, all 10) ran
  against this worktree's server on port 8192, not `:8080` or `:8090`.
- **Bite check:** with the no-`insister_id` guard disabled,
  `TestHostFestivity_ResolveHostMar_Guards` fails with the wrong refusal (it
  reports "no insistence from that guest is waiting…", not "insister_id is
  required"). The guard is restored and the test passes.
- **375px view:** the host's pending-mar card after a guest insists looks the
  same on this worktree's server and on main's dev server (`:8080`). Both show
  "bob insists you take a Mar" with the four options. The screenshots are
  under `frontend/shots/` (gitignored).
- **Repo-wide grep**, excluding `docs/work/notes`:
  `git grep -n -i "pendinghostmars\|pending_host_mars" -- . ':!docs/work/notes'`
  finds nothing.

Not run: the full e2e suite (only the Host Festivity spec), and anything
against `main`'s code beyond the screenshot.

## Behaviour changes to look at before merging

- A resolve-host-mar request with no `insister_id` returns 400 and settles
  nothing. Before, it could settle a legacy `PendingHostMars` entry.
- The resolve-host-mar response no longer has a `pending_host_mars` key.
- The owner's 2026-11-23 production query found no game that needs either
  change.

## Open questions for the owner

None.

## Noticed, not fixed

- `docs/work/notes/code-cleanup-9c1556.md` on `main` mentions
  `PendingHostMars` in prose. It is a hand-over note, so it is history and was
  left alone.
- On this branch, `gofmt -l` also lists `handler/plan_make_war_integration_test.go`,
  `handler/push_notifications_integration_test.go` and
  `handler/wait_state_integration_test.go`. None are in this diff, so the
  unformatted state predates this change.
