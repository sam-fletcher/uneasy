# Spread Rumors fixes (tracker item 1, Spread Rumors bullet)

## What changed and why

- **Consent screen on a mar.** The preparer (the take's victim) was told the
  mar's break and leverage "also hit" the plan's target asset, which belongs to
  the player making the demand. The server was right and the screen wrong: on a
  mar the rumor that spreads is about the preparer, so the options land on the
  preparer's main character (the existing Q1–Q2 rulings, and the rulebook's
  wording that the counter-rumor is about the preparer). The screen now names
  that main character. No server change for (a).
- **`request-take-consent` guards** (handler/plan_spread_rumors_routes.go,
  `srTakeConsentPreconditions`):
  - no roll yet → 409; unsettled roll → 409 (it used to skip the check
    whenever the outcome was empty);
  - options already chosen or a take already resolved → 409, so a repeat
    request can't commit the choices again and add a second rumor row;
  - a request already open → 409 instead of silently overwriting it.
  The respond route already 409s with no pending request.

## How it was verified

- Two new integration tests (`TestSpreadRumors_TakeConsent_*`); both fail with
  the route change reverted (200 instead of 409; a pending consent stashed),
  pass with it.
- Full `make test-integration` on a private database: handler suite passes;
  `cmd/server` embed tests failed only because the frontend wasn't built yet
  (`make build` fixes that; not re-run after).
- `make check-fast` passes.
- UI at 375px against my own server, mar with a two-option take + break +
  leverage: preparer sees "ravi's main character" in the also-hits line and the
  consent box; target and bystander see only the waiting line. Not run: e2e.

- Component test (`SpreadRumorsPanel.test.ts`, `svelte/server` render): the
  mar consent box as the preparer names the main character and not the target
  asset; fails against the old panel. Added a mar-path guard test too.
- The guards are read-check-write with no lock, so a concurrent double-tap
  could still slip through.

## Behaviour changes to look at before merging

- A second `request-take-consent` while one is open now 409s (before, it
  replaced the first). The UI never sends one.

## Open questions for the owner

None. (Rule was clear enough: mar → about the preparer.)

## Noticed, not fixed

- `respond-take-consent` commits the asset ids stashed at request time without
  re-checking owner or destroyed state (also in the tracker).
- `make test-integration` fails in `cmd/server` until `make build` has
  populated the embedded frontend.
