---
title: "Should the e2e suite also gate PRs, now that it runs nightly?"
---

The owner chose a non-blocking nightly run on `main`
(`.github/workflows/e2e-nightly.yml`), so a stale spec or a bad merge shows
up within about a day. Still open: whether to *also* gate PRs on e2e. That
would catch a break before merge rather than after, at the cost of a slower
CI job on every PR (the 159 specs take 2 to 4 minutes; the job also builds
the binary and installs Chromium first).

Revisit after a couple of weeks of nightly results. They will show how flaky
the suite is (retries are off on purpose), and whether a PR job should run on
every PR or only when frontend or handler code changes (a `paths:` filter).
