---
title: "Does a war or liaison line say \"Resolving\" while it is being played?"
---

A plan line on the Public Record carries at most one status word. Make War and Clandestinely Liaise
never get Make or Mar there, since the book gives them neither, so a finished war or liaison line
says nothing at all. The server does store "make" on both when they end, so the record ignores that
column for these two plans (`lib/recordLines.ts`, with a test).

What the ruling doesn't say is what the line shows *while* one of them is the plan being played. I
took "Resolving" to be where the table is, not an outcome, so it still shows. It matters most for
Clandestinely Liaise, which stays open through several steps (the war closes the moment it begins).

- **Show "Resolving" on them too (what is built).** The current row reads the same whatever plan is
  on it.
- **Show nothing on them, ever.** The reading that "no word on Make War or Clandestinely Liaise" is
  literal. It is one line in `planStatusWord` to change: `NO_OUTCOME_WORD` would also return null for
  `resolving`.
