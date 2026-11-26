---
title: "Make War: accepting a peace proposal isn't logged, though rejecting one is"
order: 83
---

`POST /api/plans/:planId/vote-peace` (`handler/plan_make_war_peace.go`) posts a log line when a
participant rejects a peace proposal, but nothing when they accept one. A returning player can't see
who has agreed so far, and the acceptance that makes peace arrives with no lead-up.

Moved here from the peace-proposal question on 2026-11-26: it is an inconsistency between two
branches of one route, not a ruling. The rest of that question (withdrawing a proposal, and paying a
cost while your own is open) is still the owner's.

**Fix:** log an accepting vote the way a rejection is logged, at the same severity, naming the voter.

**Done when:** an integration test reads the accept line; the board log guard still passes.
