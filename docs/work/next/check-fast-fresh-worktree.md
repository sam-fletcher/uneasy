---
title: "make check-fast fails in a fresh worktree until the frontend has been built"
order: 88
---

Six hand-over notes record the same stumble (ops-small, book-rule-fixes, make-demands-rest,
plan-choice-rulings, modest-edison, busy-joliot). In a fresh worktree, `make check-fast` fails three tests in
`cmd/server` ("no immutable .js asset in the embedded build", `frontend_test.go`). They read the
built frontend in `cmd/server/frontend_dist`, which is gitignored and only `make frontend` or
`make build` fills. CI runs `make frontend` before `make check-fast`, so it never sees this. Every
work session pays for one failed run and a rebuild.

**Fix:** give `check-fast` a prerequisite that runs `make frontend` when
`cmd/server/frontend_dist/_app` is missing, and does nothing when it exists. It doesn't check that the
build is fresh: these tests only need some built asset. CI's explicit `make frontend` step stays.
Update `docs/DEVELOPMENT.md` if it says to build first.

**Done when:** `make check-fast` passes in a fresh worktree with no other step first, and a second run
doesn't rebuild the frontend.
