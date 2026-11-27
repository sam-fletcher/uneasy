# Festivity duel terms (festivity-duel-terms-bbd109)

## What changed and why

The owner ruled on 2026-11-26 that a Host Festivity challenge ("challenge somebody to a duel", the make that resolves Propose Duel immediately, p.61) skips the delay, not the preparation. Propose Duel's preparation names the duel's type (arms or wits) and its place (p.60), so the challenger now names both. Before this change a festivity duel carried only the optional note, so its heading and log line named no type and no place.

- **Server, the challenge.** `challenge-duel` (guest) and `host-choice` with `challenge_duel` (host) take `duel_type` and `preparation_notes` (the place). They are checked by the same two functions Propose Duel's own preparation uses (`pduelDuelTypeError`, `pduelPlaceError` in `handler/plan_propose_duel.go`), via `hfChallengeTermsError`. The check runs after the existing gates (roll, pause, roster, host makes) and before anything is spent, so a refused challenge changes nothing.
- **Server, the pending challenge.** `game/festivity.go` `PendingChallenge` gains `DuelType`, stored with the place in `Notes`.
- **Server, the spawned duel.** On accept, `hfStampDuelType` writes the type into the spawned Propose Duel's resolution data before its "prepared" line is posted, so the line and the duel's heading both name it. The place is the duel's `preparation_notes`, as it already was.
- **Server, Propose Duel's preparation route.** `handler/plans.go` now calls `pduelDuelTypeError` for the same check it made inline. The wording is unchanged.
- **Frontend.** The "Duel of" chips and the "Location" field are one component now, `plans/duel/DuelTermsFields.svelte`. Propose Duel's `PrepForm` and both festivity challenge forms (the guest's `SocializingTurn` and the host's `HostChoosing`) use it, so they word and limit the terms alike. The Make button stays disabled until a target and a place are chosen. The host's draft (`festivityDraft.svelte.ts`) keeps the type and place with the rest of the pick.
- **Frontend, the target.** `ChallengeBanner` says "to a duel of arms" or "of wits" under the challenge, so the target sees the type before accepting. The place was already shown as the quoted note.
- **Tests.** Integration tests in `handler/plan_host_festivity_integration_test.go`: the guest's and the host's challenges each name the type and place on the spawned duel and in its log line; a challenge missing or with a bad type, or a blank place, is refused and spends nothing. Existing challenge calls in the integration and e2e suites now send the terms.

Errors in the new checks use Propose Duel's wording: `duel_type must be 'arms' or 'wits'`, and `propose_duel requires preparation_notes (location and type of duel)`.

## How it was verified

Against this worktree's code:

- `make check-fast`: passes on the final tree (lint, `go vet`, unit tests, svelte-check, Vitest, guard tests, tracker check).
- Integration: the full `./handler/` package passes on a private database (`uneasy_claude_festivity_duel_terms_bbd109`, never `uneasy_test`). It was run twice; the first run had one failure (a host-side refusal ran while the guest's roll was open), fixed by reordering that test. The other integration packages (`cmd/server`, `db`, `game`, `gametest`) passed in the same `make test-integration` run.
- Prove it bites: with the spawned duel's type stamp disabled, the two naming tests fail (the type reads "" and the prepared line has no "a duel of wits"). With the guest's term check removed, the refusal test fails (a challenge with no type returns 201). Both restored before committing.
- Frontend pins (review round): `challengeReady` (options.test.ts), the draft's `clear` reset of the terms (festivityDraft.test.ts), and the banner's type wording and no-suffix case (festivity/ChallengeBanner.test.ts). Each was broken on purpose and its test failed.
- e2e: `frontend/tests/e2e/host_festivity.spec.ts`, 11 of 11, against a worktree server on port 8192 with its own database. The spec was run before the test reorder, which touched only Go test code.
- 375px screenshots (from the same server): the host's challenge form with type and place filled, and the spawned duel's heading as the duellist sees it.

Not run: the guest's own challenge form through the UI at 375px (its screenshot was not taken; the host form uses the same component); the full e2e suite beyond the festivity spec.

## Behaviour changes to look at before merging

- `challenge-duel` and `host-choice` `challenge_duel` now return 400 without a valid `duel_type` and a non-blank `preparation_notes`. Any client that sends only the target now fails. The web app is the only client, and it is updated.
- A pending challenge created before this change has no type. Its duel spawns with no type (the heading says "a duel"), and with the old fallback note "Duel triggered by festivity" if the note was empty. That fallback stays for these in-flight challenges.
- The challenge's log line ("threw down a challenge across the hall, calling out X to a duel.") does not name the type or place. The spawned duel's prepared line does.

## Open questions for the owner

- One phrase for the duel's type: "wits" in the chip, the log line and the heading, or "trial of wits" in the heading. Filed as [docs/work/questions/duel-type-wording.md](../questions/duel-type-wording.md), with a recommendation.

## Noticed, not fixed

- The duel's heading ("a trial of wits") and its prepared log line ("a duel of wits") use different words for wits. See the question above.
- On the spawned duel's panel at 375px, the roll line reads "Rolled MAR · 1 distinct face, difficulty 4" before the duel has rolled, and contradicts the panel's own "Difficulty 3". The likely cause is that the table passes one row-level `activeRoll` to every plan panel without checking its plan. Filed as [docs/work/next/duel-panel-shows-festivity-roll.md](../next/duel-panel-shows-festivity-roll.md).
