---
name: work-session
description: The working agreement for a session doing one piece of work in its own worktree, usually started by the coordinating session. Covers the start-up check, scope, the house rules most often broken, the verification bar, the tracker and hand-over note, git, and the final report. Use at the start of any implementation task in this repo, and again before reporting it done.
---

# Doing one piece of work

## 0. Check your environment first

Before anything else, confirm all four. If any fails, **stop and tell the
owner** — don't work around it, because the missing context is exactly what
keeps the work faithful to the game and the owner's rulings.

1. A memory index (`MEMORY.md`) is loaded in your context.
2. You can read the private rules (the private CLAUDE.md, loaded from the
   folder above the main checkout, gives the path).
3. `git branch --show-current` is your own branch, not `main`, and `pwd` is
   your worktree.
4. Your branch starts from the latest `main`. The start-up hook
   fast-forwards a branch with no commits; if it says yours is behind,
   `git rebase origin/main` before you build.

## 1. Read before you build

- `CLAUDE.md` and [docs/work/TRACKER.md](../../../docs/work/TRACKER.md).
- The design principles (private; path in the private CLAUDE.md) before any
  design call. Decisions the owner has already ruled on aren't yours to
  re-open — check memory and the principles before proposing one.
- The rules for the plan or phase you're touching. **Ambiguous rule → ask the
  owner** (the `grilling` skill). Don't pick an interpretation and build it.
- **Fixing a bug** whose cause isn't obvious from one read: follow the
  `diagnosing-bugs` skill before changing code.
- **Code that looks wrong or odd** may be deliberate. Read its history
  (`git log --format='%h %s%n%b' -- <file>`, or `git blame` on the lines)
  before changing it.

## 2. Stay in scope

Change only what your brief covers. Another session may be editing other
files right now. If you find something wrong outside your scope, write it
under "Noticed, not fixed" in your note — don't fix it.

**Don't start sessions.** Work fans out one level, from the coordinator.

## 3. House rules most often broken

Each has burned this project before; the memory index has the detail.

- **Rules fidelity:** implement the plan's rules as written, not simplified.
- **Every state change posts to the action log**, including purely narrative
  effects. Raw effect helpers broadcast a WebSocket event but post nothing —
  pair them with an emitter (`handler/log_coverage_test.go` guards this).
  Anchor posts with `logRow(game)`.
- **Who must act:** a new wait must be wired into `ComputeRowState`,
  `ResolvingWaitees` and `frontend/src/lib/waitingOn.ts`. `ResolvingWaitees`
  *adds to* the acting set; it doesn't replace it.
- **Server-authoritative sub-flows:** done-state lives in `resolution_data`
  behind a backend guard, never in client-local `$state`.
- **Hidden information:** secret text never reaches the action log or a
  response another player can read. When the fiction hides what the players
  already know, log both levels.
- **Prepare-time references** (asset or player ids) must be re-validated at
  resolve time — the board changes in the rows between.
- **Mobile first:** design and check at 375px; tap targets ≥ 44px; colour
  only through the tokens in `app.css`.

## 4. Verify

All of these, against **your** code (the `test-worktree` skill shows how):

1. `make check-fast`.
2. `make test-integration` on a **private** database, never `uneasy_test`.
3. **Prove each new test bites:** undo the fix, watch the test fail, restore.
4. **UI changes:** look at 375px as the actor, the other party and a
   bystander (the `drive-plan` skill).
5. **e2e:** run any spec covering what you touched against your own server.

## 5. Record

On your branch, alongside the code:

- **Tracker:** update your row in `docs/work/TRACKER.md`, and add any open
  question for the owner under "Waiting on the owner".
- **Hand-over note:** `docs/work/notes/<session-name>.md`, with these
  sections: *What changed and why* · *How it was verified* (say what did
  **not** run against your code) · *Behaviour changes to look at before
  merging* · *Open questions for the owner* · *Noticed, not fixed*.

The repo is public: paraphrase the rulebook, never quote it, and copy
nothing in from the private folder.

## 6. Git

- Work only on your branch. Never commit to, merge into or push `main`.
- Once §4 passes, commit with the `commit` skill — logical commits whose
  messages keep the reason, the tracker and note last.
- `git fetch origin && git rebase origin/main`. If `main` moved, re-run
  `make check-fast` (and anything else main's changes could affect) on the
  rebased branch.
- Push your branch and open a **draft** PR with the `pr` skill.
- If verification didn't pass, don't open a PR — report what's blocking.
- Never merge or enable auto-merge; the owner merges.

## 7. Report

Your final message mirrors the note, in short: what changed, what was
verified against your code (and what wasn't), behaviour changes, open
questions, and the draft PR's URL. Be plain about anything unfinished or
unverified.
