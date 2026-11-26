# Propose Duel's pairing after setup

An owner ruling (2026-11-25) on the plan-panel work in PR #127. It answered the question file
`duel-pairing-later-steps`, which this PR deletes. The same PR first also restyled the "What can happen?" toggle;
that moved to PR #130 (the shared explainer row), which also answers `disclosure-in-the-sub-heading-look`, so this PR
does not touch `Buffet.svelte`. This PR only deletes that question file.

## What changed and why

- **The duel's pairing, after setup, is plain text in a finished line.** The centred set piece stays in the
  setup step only. Once the duel is in its bouts, its roll or done, `ResolvingCard` draws "Stakes set: alice's
  *Ser Ott* faces bob's *Dame Orr*" among the finished lines, after the preparation line and its notes and before
  the roll. Setup had no finished line of its own, so this is the one added, not a second. It uses
  `PlayerName`/`AssetName` with the crest mark (`asset` family: the stakes are assets) and wraps like the
  preparation line, since its facts can't be shortened. `ResolvingCard` gained an optional `finished` snippet for a
  plan's own finished lines; Propose Duel is the only user. A player with no peer stands on their name alone, as
  in the set piece.
- `docs/STYLE_GUIDE.md` (Play steps) says it; `/dev/styleguide` gained a finished-line cell for the pairing.

## How it was verified

- `make check-fast` passes. New unit tests in `ProposeDuelPanel.test.ts`: the set piece in setup with no finished
  line; the plain line (once, no set piece) in bouts, roll and done for the preparer, the target and a bystander; a
  peerless player.
- 375px before/after screenshots as a watcher of a bout (see the PR).
- Playwright against this branch's own server (private database): the Propose Duel spec.
- Not run: `make test-integration` (no Go code changed).

## Behaviour changes to look at before merging

- A watcher of a duel's bouts or final roll now sees the pairing again, as a line under the preparation line.

## Open questions for the owner

None.

## Noticed, not fixed

- The finished line's mark is the crest, my pick; no duel mark exists. Say if the crossed swords (`seize`) would read better.
