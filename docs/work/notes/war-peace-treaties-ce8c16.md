# war-peace-treaties — agreed peace terms are kept as treaties

Branch `claude/war-peace-treaties-ce8c16`. Builds `docs/work/next/war-peace-treaties.md`
(owner's ruling, 2026-11-26).

## What changed and why

- **A treaty is an accepted peace proposal, read back.** No new table. Migration 061 adds
  `war_peace_proposals.rev`, drawn from `laws_rumors_rev_seq` (the same sequence as a law's rev)
  when the proposal is accepted, and a CHECK that an `accepted` row has one. The CHECK is the
  guard against a future path that accepts a proposal without drawing a rev: it would otherwise
  produce a treaty that never shows as new. The migration also numbers proposals that were already
  accepted (so old wars show their treaties) and moves caught-up players' markers past them, so no
  one gets a dot on deploy.
- **`ListTreaties`** (`db/queries/wars.sql`) returns a game's accepted proposals with the war's
  declaring plan (which names the war on the client, `warName`) and `agreed_by` (every accepting
  vote, the proposer's included). It rides with the laws: `GET /state` (`treaties`) and
  `GET /tables/{id}/laws`, so everyone at the table gets it. Peace terms were already public in
  the log.
- **One accept path.** `mwAgreePeace` (`handler/plan_make_war_peace.go`) is the only place a
  proposal is accepted: `AcceptPeaceProposal` (draws the rev; its error now aborts, where the old
  status write was best-effort), `EndWar`, the `war.ended` and new `treaty.agreed` events, and the
  closing line `All active participants agreed to peace terms: "…" — the war is over.` The terms
  are quoted with `%q`, as the decree's amended text is. I pulled the tally and the accept tail out
  of `mwVotePeaceHandler` into `mwPeaceTally` and `mwAgreePeace` because the handler went past the
  cognitive-complexity limit. The rejection line (~219) is untouched.
- **The "new" dot, in every place it is computed.** The marker is one number on one scale, so:
  `MarkLawsSeen` clamps to the newest law *or treaty*; `CreateLaw` and `UpdateLawText` no longer
  move their writer's marker past an unseen treaty (they already refused to skip an unseen law);
  on the client the page's `lawsNew`, the report-as-seen effect and the writer's marker re-read
  all read `lawsTabEntries` (laws and treaties); `LawsRumors` draws each treaty's own dot against
  the marker the list opened with.
- **Frontend.** `Treaty` type; `treaties` in the table state, snapshot, `GET /laws`, and a
  `treaty.agreed` WebSocket event so the Laws tab updates without a reload. `LawsRumors.svelte`
  draws a "Treaties" group below the laws, only when there is one: the terms, then "Ending <war>"
  (`WarName`) and "Agreed by" (`PlayerName label`). No Edit button. STYLE_GUIDE's Laws section
  describes it.

## How it was verified

Against this worktree's code, on private databases (`uneasy_claude_war_peace_treaties_ce8c16`,
and a second for the server, both dropped):

- `make check-fast`: passes (Go vet/lint/unit, svelte-check, Vitest 2414, hook tests, tracker).
- `make test-integration`: the full suite passes (handler package 265s).
- New integration tests in `handler/war_treaties_integration_test.go`: accepting a proposal puts
  the treaty (terms, war's plan, who agreed) in `GET /state` and `GET /laws` for all five seats,
  with a rev above the newest law and above every marker; the closing line quotes the terms; the
  marker clamp counts treaties; a later law doesn't carry its writer's marker over an unseen
  treaty; an open or rejected proposal is no treaty; the CHECK refuses an accepted row without a
  rev. **Proved to bite:** removing the clamp's treaty term and the `CreateLaw` treaty check
  failed the two marker tests, and I restored them.
- Component tests (`LawsRumors.test.ts`): group below laws, only when present, no Edit button for
  any of three viewers, war name and shaped names, no war named when its plan is missing, dot
  against the marker, terms never parsed. `ws-handlers.test.ts`: `treaty.agreed` adds, replay is
  harmless.
- 375px screenshot as a bystander from a real game state: Make War declared and its delay reveal
  answered over HTTP, a peace proposal made and accepted over HTTP, one law in the database.

Did **not** run: the Playwright e2e specs (`public_record.spec.ts` covers Laws; not run), the
`treaty.agreed` frame arriving in a live browser (the handler and the client reducer are each
tested, not the two together), a migration run over a database with already-accepted proposals
(the backfill is straightforward SQL but only ran on empty tables), and the Make Demands
stand-in / keep-assets "who acts" renderings (a treaty names no one as acting or receiving).

## Behaviour changes to look at before merging

- The closing line quotes the terms with straight double quotes via `%q`, as the decree's amended
  text does. A multi-paragraph proposal reads with literal `\n` in the log. The item said
  "quote it as other log lines quote player text", and that is what the others do; say if you
  want it prettier.
- The player whose accept closes the vote gets a "new" dot on Laws for the treaty, like everyone
  else. (A law's writer is caught up; for a treaty nobody wrote it, and the last accepter may
  not have the tab open.)
- Accepting now fails loudly (500, war stays active) if the rev can't be drawn, where it used to
  ignore a failed status write and end the war anyway.
- Wars that ended by peace before this migration show their treaties on the Laws tab.

## Open questions for the owner

None new. Whether terms may move assets, repair a broken one or resolve an unprepared plan is
still `questions/make-war-peace-proposal-loose-ends.md`.

## Noticed, not fixed

- A surrender doesn't re-tally an open peace proposal: if the only player who hadn't accepted
  surrenders, the remaining players have all agreed but nothing closes the vote until someone
  presses Accept again. This is the one other route near "resolving a proposal"; it ends no war
  by itself, so it creates no treaty.
- `plan_make_war.go` ends a war with reason `peace` when a delayed war falls off the record
  (`delayRevealOverflow`). No proposal exists there, so no treaty: correct, but the reason name
  is misleading.
- The item file `docs/work/next/war-peace-treaties.md` is on PR #141's branch, not here, so this
  branch doesn't delete it. Delete it when rebasing on `main` after #141 merges.
