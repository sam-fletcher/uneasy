# frontend/src/lib

Everything the SvelteKit routes import: API client, components, and
cross-cutting browser-side logic. Most non-trivial files carry a short doc
comment at the top explaining the *why*; this is just the map.

## Layout

- **`api/`** — one file per backend resource (`accounts.ts`, `assets.ts`,
  `plans.ts`, `rolls.ts`, `tables.ts`, …), each a thin typed wrapper around
  `client.ts`'s `fetch` helper. `types.ts` holds the shared response/DTO
  types mirrored from the Go API. `index.ts` re-exports the lot.

- **`components/`** — Svelte components, split by scope:
  - top level — shared, cross-phase UI: `ChatPanel`, `RetinueView`,
    `PublicRecord`, `WaitingOnBar`, form/modal primitives, etc.
  - `phases/` — one view per game phase (`PrologueView`, `MainEventView`,
    `ShakeUpView`), plus `phases/prologue/` for Prologue-only sub-widgets
    (hand strip, set-aside placer, track board).
  - `plans/` — one panel per Main Event plan type (`SeekAnswersPanel`,
    `ProposeDecreePanel`, …), plus `registry.ts` mapping plan type →
    component, `shared.ts` for cross-plan helpers, and subdirs
    (`demand/`, `duel/`, `festivity/`, `war/`) for plans complex enough to
    need their own sub-components.
  - `shared/` — CSS partials (not components) reused across many
    components: `actionButton.css`, `modalShell.css`, `rankChip.css`, etc.

- **`plans/resolutionData/`** — one file per plan type, typed parsers/view
  helpers over that plan's `resolution_data` JSON blob. The frontend mirror
  of the per-plan `*ResolutionData` structs in `game/` on the backend.

- **`prologue/`** — Prologue-phase pure logic that isn't UI (currently just
  the heart-refund calculation).

- **Top-level `*.ts` files** — cross-cutting browser-side state and pure
  logic that doesn't belong to one component: `ws.ts` (WebSocket client +
  reconnect/resync), `chatFeed.ts` (unified chat/log feed assembly),
  `waitingOn.ts` (the "Waiting On" bar), `secretCounts.ts` /
  `succession.ts` (secret-existence and monarch-succession display rules),
  `severity.ts` / `textLimits.ts` (mirrors of Go-side constants — see each
  file's header comment for which one), `push.ts` (web push
  subscription flow), and a handful of small single-purpose helpers
  (`playerColor.ts`, `highlight.ts`, `warDrawer.ts`, `warStayOuts.ts`,
  `tableHeader.ts`, `assetRisk.ts`, `scenePrompts.ts`, `useWindowEvents.ts`).
  Most of these exist specifically so their logic is unit-testable
  (`*.test.ts` alongside) without mounting a component — see
  `tableHeader.ts`'s header comment for the pattern.
