---
title: "Should a render crash report itself?"
---

General error reporting to Discord was declined. One narrow slice was left waiting on
a yes: when `svelte:boundary` catches a render throw, report it to a new endpoint and
table. Those throws are rare, invisible and always bugs. It needs a migration, and
must not use the feedback table's rate limit.
