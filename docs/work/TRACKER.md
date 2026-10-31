# Work tracker

What's in flight, what's next, and what's waiting on the owner. Each work
session updates this file **on its own branch**, so the tracker merges with
the code it describes. Hand-over notes go in [notes/](notes/), one file per
session, named after the session.

Older history — the 12-plan UX audit's findings, rulings and the hand-over
notes up to 2026-10-30 — is private and lives outside the repo (see the
private CLAUDE.md).

## Now

| Piece | Session | Status |
|---|---|---|
| Move sessions to `uneasy/` as the project root; CI on PRs | — | in review (`setup/repo-root`) |

## Next — the 12-plan UX audit, remaining work

Order matters only where noted; anything with disjoint files can run in
parallel.

1. **Review of the merged audit work (PRs #2–#12)** — waiting on the owner's
   decision whether to run it.
2. **Pass 4: shared components.** Merge the three duplicated stepper
   controls, adopt the shared Buffet picker where plans hand-roll one, wire
   `DifficultyMeter` into Make Introductions, and give plan resolution one
   shared announcement (`EmitPlanResolved`). Fold in the Make Introductions
   arrival-form fix below — same panel.
3. **Make Introductions: a demanded winner has no path.** The arrival forms
   are gated on `isPreparer`, but the server accepts the Make Demands
   `perform_steps` winner. With the Complete button gone, a winner-driven
   Make Introductions shows no way forward.
4. **Durable "choice applied" marker.** A make/mar commit with zero picks is
   indistinguishable from no commit (`MakeMarChoices` is `omitempty`), so the
   Waiting On bar keeps naming a player who has finished.
5. **e2e specs off the hard-coded `:8090`.** Every spec should honour
   `E2E_BASE_URL`, so a worktree run tests the worktree. Must land before 7.
6. **Proposed, not built:** a partial unique index enforcing one live main
   character per player; a before/after-board log-coverage guard.
7. **Phase 4: regression specs** for what Phase 3 fixed — one spec file per
   plan, so these fan out in parallel. After 2 and 5.
8. **Small:** the Shake-Up focus-pass log line doesn't name skipped players.

## Waiting on the owner

- Whether to run the review in item 1.
- **Abrupt transitions** — removing click-to-agree gates leaves stage changes
  with no beat or explanation. A design conversation, not code yet; the draft
  write-up is in the private notes (`transition-whiplash.md`).
- Propose Decree's make now resolves itself after the law is enacted — confirm.
- Make Demands draft picks and Host Festivity "earns a make" log at Minor
  severity (hidden by the default chat filter) — confirm or raise.

## Never audited

Chronicle Histories through the UI; the Make Demands overlay interacting
with other plans; Propose Decree's monarch case; tables of 4+ players.
