---
title: "Fail an e2e test whose page shows the table's error-boundary fallback"
order: 79
---

The table page's `<svelte:boundary>` now logs what it catches to the console,
under the prefix `[uneasy:boundary]` (`logBoundaryError` in
`src/lib/boundaryError.ts`). A Playwright run could fail the test the moment
that line appears, or the moment a page shows the `.boundary-failed` fallback,
so the next `each_key_duplicate`-style crash turns a spec red instead of
passing with a half-rendered table (as the 2026-11-08 nightly did).

It wasn't done with the boundary fix because no single place sees every page:

- All 22 specs import `test` straight from `@playwright/test`, so a fixture
  that listens on every page needs that import swapped in all 22. The edit is
  mechanical but collides with any spec in flight.
- Pages come from the `page` fixture and from dozens of `browser.newContext()`
  calls in specs and helpers (24 files mention it). A fixture has to cover
  both, which means wrapping `browser` as well as `context`.
- The cheap alternative, an `afterEach` in `cleanupGameAfterEach` (20 of the 22
  specs call it) that scans `browser.contexts()` for `.boundary-failed`, only
  sees pages still open when it runs, and misses a fallback the player
  dismissed with "Try again". A check with silent gaps is worse than a known
  absence.

Do it as one pass while no spec PR is open, with a console listener rather than
a DOM scan. Run the whole suite once first to find specs that currently pass
with the fallback showing.
