---
title: "Later: move the nightly e2e runner back to ubuntu-latest"
order: 210
---

`e2e-nightly.yml` pins its `e2e` job to `ubuntu-24.04` because Playwright 1.60.0
cannot install Chromium on Ubuntu 26.04, which `ubuntu-latest` began to mean on
2026-11-11. Ubuntu 24.04 runners will not last for ever, so this is a stopgap.

Done when: a Playwright release lists Ubuntu 26.04 as supported (the release notes
say so; `npx playwright install --with-deps chromium` stops printing "does not
support chromium on ubuntu26.04-x64"), the pin goes back to `ubuntu-latest`, and the
PR self-test run of the workflow is green. Bumping Playwright is a dependency
change, so run `make check-fast` and the e2e suite on that branch too.
