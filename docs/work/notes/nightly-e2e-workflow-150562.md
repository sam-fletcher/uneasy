# Nightly Playwright e2e run on GitHub Actions

## What changed and why

CI gates PRs on `make check-fast` and the integration suite and never ran
the Playwright suite, so three specs went stale and sat failing with no
signal. The owner chose a non-blocking nightly run on `main`; this adds it.
Whether to also gate PRs stays open (see the question file below).

- **`.github/workflows/e2e-nightly.yml`** (new). Triggers: `schedule` (daily
  06:17 UTC), `workflow_dispatch`, and `pull_request` filtered to this file
  only (a self-test for the PR that edits it). Same conventions as `ci.yml`
  (`permissions`, `concurrency`, `timeout-minutes`, `go-version-file`, Node 24
  with npm cache, `postgres:16-alpine` service on `uneasy_test`, actions
  pinned by major tag). Two jobs:
  - `e2e` (read-only token): `make build` (production binary with the
    frontend embedded, no Vite), start `./server` on `:8090` with
    `UNEASY_DEV=1` against the service Postgres, wait with `curl --retry`
    until it answers (the new e2e preflight aborts without waiting), install
    Chromium, run `npm run test:e2e` with `CI=true`. Serial, `retries: 0`,
    kept that way on purpose to measure real flakiness. On failure it uploads
    `frontend/test-results` (traces plus the JSON report) and `server.log`
    for 14 days.
  - `report` (the only job with `issues: write`, plus `actions: read`; runs
    only when `github.event_name == 'schedule'`): downloads the artifact,
    then opens one issue labelled `nightly-e2e`, or comments on the open one,
    with the run link, commit, failed step, failing spec names (from the JSON
    report with `jq`) and a compare link from the last passing run on `main`.
    A passing scheduled run closes the open issue with a comment. It uses the
    preinstalled `gh` CLI and no third-party actions.
- **`frontend/playwright.config.ts`**: when `CI` is set the reporters are
  `list` + `json` (to `test-results/report.json`) + `github` (failure
  annotations); otherwise still just `list`. Nothing else in the config
  changed.
- **`docs/DEVELOPMENT.md`**, e2e section: what the nightly does, what a
  failure looks like, reading the artifacts (`npx playwright show-trace`),
  reproducing with the `test-worktree` recipe, running it by hand.
- **Tracker**: deleted `next/ci-runs-e2e.md` (the nightly satisfies it);
  rewrote `questions/ci-e2e-cadence.md` to record the owner's choice and the
  one open part, gating PRs.

Decisions worth knowing: the Chromium download is not cached (a few seconds
against one more thing that can go stale; the comment in the file says so).
The `report` job also files for a `cancelled` result, because a job-level
timeout may show up that way; a manual cancel of a scheduled run would
therefore file an issue (rare, and arguably worth knowing about).

## How it was verified

Against this worktree's own code, on a private database and port `:8173`
(no `uneasy-*` container or `:8080`/`:8090` touched; server stopped and
database dropped afterwards):

- `actionlint` v1.7.7 (via `go run`, nothing installed) is clean on the new
  file and `ci.yml`. `shellcheck` is not installed here, so the embedded shell
  was not linted by actionlint's shellcheck pass.
- Each `run:` block was extracted from the YAML and executed: `make build`;
  start the server and the curl wait (it rode out a refused connection, then
  passed; against a dead port it gave up at the 90 s cap, printed the log
  and exited 1); `npx playwright install --with-deps chromium`; the suite
  with `CI=true`: **159 passed (2.1 min)** with the three reporters, and
  `report.json` survived Playwright's start-of-run `test-results` cleanup.
- A throwaway failing spec (since deleted, never committed) produced traces
  and a JSON report with the nested-describe titles; the `report` script ran
  against it with a stand-in `gh` that records calls: new-issue,
  comment-on-open-issue, close-on-pass, nothing-to-close, a truncated report
  with a `cancelled` result, and more than 40 failures. The read-only `gh`
  calls (`issue list --label`, `run list`, `run view --json jobs`) were run
  for real against the repo to check their syntax.
- With `CI` unset, a run writes no `report.json` (local behaviour unchanged).
- `make check-fast`: see the PR body.

**Not verified:** the `report` job has never run on GitHub. It fires only
on a real *scheduled* failure, so the issue/label/close calls, the artifact
download layout and the job's `permissions` were checked by reading and with
the stand-in `gh`, not live. The PR's own self-test run covers the `e2e` job
on a real runner; its result is in the PR body. To prove the report path
live after merge: on a branch, temporarily add `workflow_dispatch` to the
`report` job's `if`, break a spec, dispatch the workflow on that branch, then
close the issue it opens.

## Behaviour changes to look at before merging

- After merge, a scheduled run starts at about 06:17 UTC each day. GitHub
  only runs schedules from the default branch, so nothing happens before
  the merge, and the first scheduled run follows within a day.
- A failing night opens a public issue with the label `nightly-e2e` (the
  label is created on first use). Failure artifacts (traces, server log) are
  downloadable by anyone signed in to GitHub, as with any public repo; they
  contain only dev-seeded game data.
- Runner minutes are free for a public repo, so this costs nothing.

## Open questions for the owner

- Should e2e also gate PRs? Recorded in
  `docs/work/questions/ci-e2e-cadence.md`: revisit after a couple of weeks of
  nightly results, perhaps path-filtered. I did not add a PR gate.

## Noticed, not fixed

- `ci.yml`'s header comment still says "Playwright e2e is not here yet"
  and points only at `make test-e2e`. It should mention the nightly workflow;
  I was told not to edit `ci.yml`.
- GitHub disables scheduled workflows after 60 days with no repository
  activity (`db-backup.yml` notes the same). Not a concern while the repo is
  active.
