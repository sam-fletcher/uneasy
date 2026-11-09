---
title: "The table page's error boundary swallows the error it catches"
order: 78
---

`<svelte:boundary>` in `routes/table/[id]/+page.svelte` has a `failed` snippet
("Something went wrong displaying this part of the table") but no `onerror`, so
the exception it caught goes nowhere: not the console, not the server. The
nightly e2e failure of 2026-11-08 (a duplicated war in `GET /wars` throwing
`each_key_duplicate` inside it) looked like a stale waiting-on bar, and it took
a CDP debugger hook to read the actual error.

An `onerror` that logs the error to the console, and ideally reports it through
whatever client-error path the feedback form already uses, would turn the next
one of these from an investigation into a one-line read. The Playwright specs
could also fail any test whose page shows the boundary's fallback.
