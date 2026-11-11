# fix-nightly-e2e-runner

## What changed and why

The nightly e2e run of 2026-11-11 failed
([run 43037935260](https://github.com/sam-fletcher/uneasy/actions/runs/43037935260),
issue #72) at `Install Chromium`: GitHub's `ubuntu-latest` had moved to Ubuntu
26.04, and Playwright 1.60.0 refuses to install Chromium there ("does not support
chromium on ubuntu26.04-x64"). No spec ran, so this was never a regression in the
app. The 2026-11-10 run, on `ubuntu-24.04`, was green.

`.github/workflows/e2e-nightly.yml`: the `e2e` job now runs on `ubuntu-24.04`. The
`report` job stays on `ubuntu-latest`: it only calls `gh` and runs no browser, so
the image change cannot affect it. `ci.yml` and `db-backup.yml` also use
`ubuntu-latest` but never install Playwright, so they hit no such error.

Rejected: bumping Playwright now. No release is known to support 26.04, and it
would mix a dependency change into a CI repair.

Added `docs/work/next/e2e-runner-unpin.md` so the pin gets removed once Playwright
supports 26.04.

## How it was verified

Not run locally: the change is a runner label, and no local run uses GitHub's
images. The workflow runs itself as a self-test on any PR that edits it
(`pull_request: paths`), so this PR's own run is the proof; its result is in the PR.
`make check-fast` and the integration suite were not run: no Go, frontend or SQL
file changed.

## Behaviour changes to look at before merging

None to the app. Issue #72 stays open until the next scheduled run on `main` passes:
the `report` job closes it itself.

## Open questions for the owner

None.

## Noticed, not fixed

- Ubuntu 26.04 has not been tried for the other workflows. `ci.yml` last ran on
  `ubuntu-latest` before the switch, so the first PR after it is the first test of
  Go, Node and the Postgres service on 26.04. The nightly's build, server start and
  Postgres service steps did pass on 26.04, which is a good sign.
