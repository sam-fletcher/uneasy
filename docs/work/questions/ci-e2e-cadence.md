---
title: "Should the e2e suite gate PRs, or run on a schedule?"
---

The Playwright suite (159 specs, about 4 minutes against a healthy docker
stack) runs in no CI job, so stale specs went unnoticed. Option A: a PR job
that brings up the stack and gates merges. Option B: a nightly run on `main`
that reports failures without blocking. Option A catches drift earlier but
costs a slower, heavier CI job on every PR.
