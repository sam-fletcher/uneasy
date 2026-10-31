# Work tracker

What's next, and what's waiting on the owner. Each work session's PR
updates this file **on its own branch** — it removes or updates the item it
worked on and adds any open question for the owner — so the tracker merges
with the code it describes. Hand-over notes go in [notes/](notes/), one file
per session, named after its worktree folder.

Older history — the 12-plan UX audit's findings, rulings and the hand-over
notes up to 2026-10-30 — is private and lives outside the repo (see the
private CLAUDE.md).

## In flight

The open pull requests (`gh pr list`). There's no table of them here: a row
per piece went stale the moment its PR merged, and parallel sessions all
editing one table conflicted whenever they rebased.

## Next — the 12-plan UX audit, remaining work

Order matters only where noted; anything with disjoint files can run in
parallel.

1. **Fixes from the review of the merged audit PRs (#2–#12, 2026-10-31).**
   Each was checked against current `main`. Independent of each other:
   - **Rulebook text quoted word for word** in code comments and test
     strings (re-aim, Make Introductions, Seek Answers, Host Festivity,
     Spread Rumors and Spread Propaganda files). Paraphrase them; whether
     the same wording in UI labels is fine is a question for the owner.
   - **Spread Rumors:** on a mar the consent screen names the target asset,
     but the break lands on the preparer's main character. The
     take-consent route also skips the settled-roll check and accepts a
     second submission (a duplicate rumor), reachable only by a hand-built
     request.
   - **Spread Propaganda:** a Make Demands winner standing in for "break
     yourself" picks the preparer's asset and is granted its secrets. Who
     should learn them is the owner's call (below).
   - The Make Introductions winner soft-lock is item 3.
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
9. **Make Demands, the rest of its audit** (the audit itself is private;
   items D2, D5–D7). Retargeting a demand can fail with a server error and
   has no wait state to show who must act; choosing to leverage nothing has
   no UI; the leverage option is offered against target plans that never
   roll. Separately, the draft picker hides options that would do nothing
   against the chosen target instead of showing them disabled with the
   reason.
10. **Dependency security alerts** — GitHub's Dependabot reported 23 open
    alerts on `main` (2026-10-31: 5 high, 14 moderate, 4 low). Independent of
    the audit, so it can run beside anything else. Triage before upgrading:
    - **One is in the live Go server:** `github.com/go-chi/chi/v5` (moderate).
    - **The other 22 are npm packages** in `frontend/` — SvelteKit, Svelte,
      Vite, Vitest, devalue, postcss, nanoid and others. GitHub labels all of
      them "development" because they're devDependencies, but that label is
      misleading for a static SPA: Svelte, SvelteKit's client runtime and
      devalue are compiled *into* the bundle players download. Work out which
      advisories reach the browser or the server and which only touch build
      and test tooling, then upgrade in small, separately verified steps.
    - Keep the supply-chain rules in `docs/DEVELOPMENT.md`: exact pins,
      committed lockfile, `npm ci`.

## Waiting on the owner

- From the audit-PR review (item 1): when a demand winner stands in for
  Spread Propaganda's "break yourself", who learns the broken asset's
  secrets: the winner, or the preparer whose asset it is? Is Propose
  Duel's take after a mar compulsory, though the book says the winner
  *may* take? Should Exchange Courtiers stop forcing a re-aim when the
  targeted peer only changes hands, as Spread Rumors already lets its
  rumor follow the asset?
- **Abrupt transitions** — removing click-to-agree gates leaves stage changes
  with no beat or explanation. A design conversation, not code yet; the draft
  write-up is in the private notes (`transition-whiplash.md`).
- Propose Decree's make now resolves itself after the law is enacted — confirm.
- Make Demands draft picks and Host Festivity "earns a make" log at Minor
  severity (hidden by the default chat filter) — confirm or raise.

## Never audited

Chronicle Histories through the UI; the Make Demands overlay interacting
with other plans; Propose Decree's monarch case; tables of 4+ players.
