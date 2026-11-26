---
title: "Report a render crash to the server"
order: 90
---

Approved on 2026-11-26, at low priority. When a `svelte:boundary` catches a render throw, the page
reports it to a new endpoint that stores it in a new table. These throws are rare, invisible to
everyone but the player who hit them, and always bugs.

- A migration for the table (game, account, the error's message and stack, the route, the client's
  build); a login-gated endpoint that writes a row.
- Its own rate limit, not the feedback table's: a crash must never spend a player's feedback budget.
  Key it on the account, never on a client-supplied address.
- Nothing else is reported: transport failures, 4xx and 500s stay as they are.

Update the comment in `lib/boundaryError.ts` that calls this an open question.

**Done when:** an integration test writes and rate-limits a report; a boundary test in the frontend
shows the call made once per crash.
