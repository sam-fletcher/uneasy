# adr-cleanup-followups

## What changed and why

The private decision records were cleaned up: the old plans and audits were read in
full and archived, and what's still true now sits in a private rulings file and a
private decisions file. This branch is the repo's side of that.

- **Skills:** `grilling`, `work-session` and `review-pr` now name the rulings and
  decisions files, and `grilling` says where a new ruling is recorded.
- **Tracker:** every thread that lived only in those old docs and is still open now
  has a file here, 7 in `next/` and 13 in `questions/`.
- **Stale comments:** three comments still described `CloseLeverage`, which was
  removed, and `PublicRecord.svelte` still gave the old 280px record width.

Code comments that cite `adr/X.md` weren't rewritten. The private CLAUDE.md now says
that means the archive.

## How it was verified

`make tracker` and `make check-fast` against this branch. No behaviour changes: Go and
Svelte edits are comments only. Skill wording has no automated check.

The tracker items came from a read of every old doc, checked against the code on
2026-11-07. Some of those checks were wrong in both directions, so
`plan-polish-leftovers` and `log-and-server-gaps` say to re-check before building.

## Behaviour changes to look at before merging

None.

## Open questions for the owner

All in `docs/work/questions/`. The two with the most hanging on them are
`make-demands-pre-roll` and `make-war-peace-proposal-loose-ends`.

## Noticed, not fixed

Everything found is in the tracker files above.
