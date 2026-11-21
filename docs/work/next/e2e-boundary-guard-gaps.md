---
title: "Close the gaps in the e2e boundary guard: other page errors, other boundaries"
order: 81
---

`frontend/tests/e2e/fixtures.ts` fails any test whose page logs `[uneasy:boundary]`
(what the table's `<svelte:boundary>` logs through `logBoundaryError`). It hears
that one line and nothing else. Two ways a render crash can still pass over a
half-drawn page, and one smaller gap:

- **The wider net.** A throw that escapes every boundary (an event handler, a
  `$effect`, a promise nobody awaits) shows up as `page.on('pageerror')`, not as
  the boundary line, and an app `console.error` is not looked at at all. The same
  fixture could fail a test on `pageerror`, and maybe on `console.error`. Before
  turning either on, run the suite once with the listener only *recording* them:
  the app may log things on purpose today (a failed fetch the page retries, the
  ws reconnect), and each of those needs a decision (fix it, or allow it by
  name) before it can fail a test.
- **A boundary that doesn't log through `logBoundaryError`.**
  `src/lib/boundaryError.test.ts` already requires every `<svelte:boundary>` to
  have an `onerror`, but not that it call `logBoundaryError`. A future boundary
  that logs some other way, or with `onerror={() => {}}`, is invisible to the
  fixture. Tighten that test to require the call (or have the fixture match
  whatever the boundaries share).
- **`beforeAll` / `afterAll`.** A page opened there is outside any one test, so
  the guard (a test-scoped fixture) doesn't watch it. No spec uses either hook
  today; if one ever does, give it a worker-scoped listener or open its pages
  inside a test.
