---
title: "Phase 4: regression specs for the audit's fixes"
order: 70
---

One e2e spec file per plan pinning what Phase 3 fixed, so these fan out in
parallel — one session per plan. Specs honour `E2E_BASE_URL` (#23), so each
session can test its own worktree. Start after Pass 4.
