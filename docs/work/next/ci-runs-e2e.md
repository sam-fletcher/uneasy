---
title: "CI does not run the Playwright e2e suite"
order: 77
---

CI runs `make check-fast` and the integration suite on every PR. Nothing runs
`frontend/tests/e2e/`, so three specs went stale (host festivity, make demands,
seek answers, all fixed alongside this note) and sat failing with no signal.
Any e2e spec can rot the same way until someone runs `make test-e2e` by hand.

Open choice for the owner: run the e2e suite on every PR (about 4 minutes on a
healthy stack, but it needs the docker dev stack in CI), or run it nightly on
`main` and report failures. See the open question in `docs/work/questions/`.
