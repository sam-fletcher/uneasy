# tombstone-order

## What changed and why
`asset.destroyed` now carries the server's `destroyed_at` (new
`model.AssetDestroyedPayload`; `assetDestroyedPayload` re-reads the row the
destroy just stamped). All three emit sites use it: `emitAssetDestruction`
(`asset_effects.go`), the last-tear path (`assets_marginalia.go`) and the
main-character replacement (`assets_main_character.go`). The client handler
uses that value for `destroyed_at`, falling back to its own clock only when the
payload has none. The payload holds only the id and a timestamp the asset list
already shows — nothing hidden. Without it, a live tombstone could sort
differently from the same retinue after a reload.

## How it was verified
- `make check-fast` passes (after `make build`; a fresh worktree has no
  embedded frontend, so `cmd/server`'s warmup test fails until it is built).
- `make test-integration` on a private DB passes.
- New tests: Go JSON-shape test, Go integration test for the payload
  timestamp, two Vitest cases. The JSON and Vitest tests were seen to fail with
  the fix undone. The integration test was not mutation-checked.
- No browser or e2e run; this is event-payload plumbing.

## Behaviour changes to look at before merging
None visible except tombstone order now agreeing live and after a reload.

## Open questions for the owner
None.

## Noticed, not fixed
Nothing.
