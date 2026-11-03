# Destroyed asset names, live vs reload

## What changed and why
- `asset.destroyed` handler (`ws-handlers.ts`) now keeps the asset in `ctx.assets`
  with `is_destroyed: true`, instead of dropping it.
  The server's REST list already includes destroyed assets, so live now matches a
  reload, and panels that name an asset by id (Seek Answers "Broke bob's
  resource") keep the name.
- Audit found consumers that had relied on the old removal, now fixed:
  ChatPanel's "speak as" personae (extracted to `lib/controlledPeers.ts`; a
  destroyed peer was selectable and posting 400s), SceneDetailsPanel's present
  list, the duel RollPhase take list, ReAimPanel's pickers, and
  `findMainCharacter`. Server: replacing a destroyed main character now
  broadcasts `asset.updated` so its star clears live (the old comment said the
  destroy event covered it); fixed the stale `ListAssets` comment.
- Seek Answers e2e now asserts the full "Broke <name>" in both places; item file deleted.

## How it was verified
- `make check-fast` and `make test-integration` (private database) on this
  branch. New Vitest tests: handler, break-picker filter, `controlledPeers`,
  `findMainCharacter`, a SceneDetailsPanel render test (checked to fail without
  its filter). The server broadcast change is not pinned by a Go test.
- Full Playwright suite against a private server and database (see PR).
- Not separately checked by eye at 375px: the change is store-level and the
  filters are unit-tested.

## Behaviour changes to look at before merging
- Destroyed assets now stay in the live list, so the retinue shows the tombstone
  card immediately rather than after a reload; a replaced destroyed main
  character loses its star live.

## Open questions for the owner
None.

## Noticed, not fixed
- Live tombstone order (no `destroyed_at` on the event): `docs/work/next/tombstone-order-live.md`.
