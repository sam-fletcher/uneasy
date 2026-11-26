# fix-retinue-sheet-wait

## What changed and why

The nightly e2e run of 2026-11-26 (run #22, `337140a`) failed on one spec:
`retinue_pieces.spec.ts › your retinue: pieces, the at-risk count, 'Leveraged'
in words, and 44px controls`, with a secrets-eye height of 43.99993896484375
against a 44px floor (issue #131).

The app was fine: `.ctl` sets `min-height: 44px`. At the spec's 375px viewport
the retinue is a bottom sheet that slides up over 200ms (`slide-up` in
`RetinueSheet.svelte`), and the spec measured the buttons as soon as the sheet
title was visible. A box read mid-slide comes out of a transformed float32
matrix, so it can be a hair under the true size.

`openRetinue` now waits for the sheet's animations to finish
(`getAnimations({ subtree: true })`) before returning, so every measurement in
the spec is of a settled sheet.

Also: the Postgres service's health check in `ci.yml` and `e2e-nightly.yml`
now runs `pg_isready -U uneasy -d uneasy_test`. Without `-d` it probed a
database named `uneasy`, which does not exist, and the server logged
`FATAL: database "uneasy" does not exist` every 5s for the whole run (about
1300 lines in a nightly log, burying the real output). `docker-compose.yml`
is unchanged: its database really is called `uneasy`.

## How it was verified

Against this worktree's own server (port 8193, private database, dropped after):

- Reproduced the failure: with the wait removed, 1 of 25 runs (4 workers) failed
  with exactly `43.99993896484375`.
- With the wait: 36 of 36 runs, then 60 of 60 at 4 workers, passed.
- `make check-fast` passes.

The health-check change is only exercised by CI itself; the PR's CI run shows
whether the service still comes up healthy and the FATAL lines are gone.

## Not done

Other specs that measure right after an entrance animation could have the same
hairline. None of the 40-odd `boundingBox` reads elsewhere opens the retinue
sheet; I did not audit the rest.
