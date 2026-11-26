# Make War: the war-state 404, and the unlogged peace accept

## What changed and why

**The 404.** `MakeWarPanel.svelte` asked for `war-state` as soon as it showed, and the server
answers 404 until the war row exists. The panel read that as "no war yet", but the browser prints a
console error for any 4xx, so a normal state looked like a fault in every browser. The war row is
created in `OnPrepare` in the same step that writes `resolution_data.make_war.war_id`, so the panel
now fetches only once the plan carries that id (`warCanExist` in `lib/warGate.ts`). The refresh
effect is keyed on plan id *and* war id, so the fetch happens when the id arrives on a plan that was
shown a moment before. Chosen over answering `200` with no war: that changes the response's shape,
and a tab left open across a deploy would read it as a war. No API change, so no one-way door.
`mwLoadWar`'s 404 is untouched: only actions reach it, and they need a war.

**The accept line.** `vote-peace` logged a rejection and nothing for an accept. It now posts
"*voter* accepted *proposer*'s peace terms." at default severity, like the rejection. A player who
has already accepted (the proposer does, on proposing) gets no second line if they press Accept
again, which would otherwise double-post. The war's other wording is untouched, including the
rejection's "the cost of battle is still owed".

Both item files are deleted, and the `public_record.spec.ts` allow-list entry with them.

## How it was verified

All against this worktree's code, on a private database and a private server on :8167:
- `make check-fast`: passes.
- `make test-integration`: passes, whole suite.
- New `TestMakeWarHTTP_PeaceVote_AcceptIsLogged` fails with the handler change reverted ("no
  plan.make_war post contains \"accepted\"") and passes with it. It also checks the line names voter
  and proposer, sits on the row being played, and that a repeat accept posts no second line.
- The 404: with the allow-list entry removed and the panel change reverted, the `public_record.spec.ts`
  test fails in 11 of 20 repeats with the war-state 404; with the change, 30 of 30 pass. Then the
  whole of `make_war.spec.ts` and `public_record.spec.ts` (32 tests) pass.
- `warCanExist` has a unit test; the old unit pin that allowed the 404 now says it is allowed
  nowhere.

Not run: a look at the panel by eye at 375px. Nothing visible changed (the panel renders the same
once the war is fetched), and the e2e Make War specs exercise the declare, delay, cost and peace flows.

## Behaviour changes to look at before merging

- A peace accept now posts a line, so the last accepter produces two lines in a row: "*X* accepted
  *Y*'s peace terms." then the important "All active participants agreed to peace terms — the war is
  over." I kept both: the first says who closed it. If it reads as redundant, skip the accepter's
  line when the vote is unanimous.
- The proposer's own acceptance (made when they propose) is not logged separately: the "proposed
  peace terms" line already says it.

## Open questions for the owner

None.

## Noticed, not fixed

- A dev-seeded Make War plan (`/api/dev/seed` with `plan_type: make_war`) has no war row and no
  `war_id`, so its panel now shows no war at all rather than logging a 404. It always showed none;
  only the console error is gone.
