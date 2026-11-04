# Close raw asset take / tear routes

## What changed and why
`POST /api/assets/{id}/take` and `DELETE /api/assets/{id}/marginalia/{pos}` let
any game member take or tear any asset at any time, with no turn, ownership or
plan check. Nothing in the app called them (only tests and unused client
wrappers), so both routes, their handlers and the `takeAsset`/`tearMarginalia`
client wrappers are removed.

Tests that used them as shortcuts now use two dev-only routes
(`/api/dev/take-asset`, `/api/dev/tear-marginalia`, `UNEASY_DEV=1` only) that
run the same effects plans use (`takeAssetEffect`, `breakMarginalia`) plus the
same log posts. Documented in `docs/DEVELOPMENT.md`.

## How it was verified
- `make check-fast` — passes (against this worktree).
- `make test-integration` on a private DB — `handler` passes (incl. new
  `TestAssetRoutes_NoRawTakeOrTear`, which fails if a take route is remounted).
  `cmd/server` tests need `make build` first (embedded frontend), then pass.
- e2e `chat.spec.ts` and `exchange_courtiers.spec.ts` on a private server: 23 pass.
- Live: `POST /take` → 404, `DELETE /marginalia/1` → 405 on the built server.
- Not run: the rest of the e2e suite.

## Behaviour changes to look at before merging
None for players. Raw take/tear no longer exist.

## Open questions for the owner
None.

## Noticed, not fixed
`deadcode` reports `middleware.AccountContext` as unreachable (used only by tests); non-fatal and pre-existing.
