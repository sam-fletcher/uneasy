# Uneasy

A web adaptation of an unusual tabletop RPG, played asynchronously with a
semi-"play by post" feel. Go backend, Svelte 5 frontend. The owner has never
built a website before and is using this project to learn frontend and
web-server development, so explain the reasoning behind changes, not only
the changes.

If you encounter an ambiguous game rule, ask the owner to clarify it rather
than guessing — the `grilling` skill shows how to ask.

## Where to find things

- **Dev workflow** — [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md): setup, the
  dev stack, test layout, the `/api/dev/*` shortcuts, and the plan-testing
  tools (`POST /api/dev/force-roll`, `cmd/plandriver`, `frontend/tests/shot.mjs`).
  [README.md](README.md) is the public pitch, licence and quick start.
- **Visual language** — [docs/STYLE_GUIDE.md](docs/STYLE_GUIDE.md): colour
  tokens, typography, interaction-state recipes, shared components. Colour
  literals outside `app.css` fail the unit suite.
- **Active work** — [docs/work/](docs/work/): the tracker for work in flight
  and each session's hand-over note. They live in the repo so that progress
  lands in the same PR as the code it describes.
- **Game rules, design principles and historical ADRs are private** and live
  outside this repo. The private CLAUDE.md in the folder above the main
  checkout says where; it is loaded automatically, because every checkout
  and worktree sits beneath it.

## Git and GitHub

- This repository is **public**, and `main` auto-deploys to
  <https://uneasy.up.railway.app>.
- Work on a branch, normally in your own worktree. Never commit to `main`.
- When a piece of work passes its verification, its session commits, pushes
  its branch and opens a **draft** PR (the `pr` skill). The coordinator
  reviews the diff and marks it ready; the owner reviews and merges.
- Sessions never merge and never enable auto-merge. A hook
  ([.claude/hooks/guard_git.py](.claude/hooks/guard_git.py)) blocks
  `gh pr merge`, force pushes and pushes to `main`; branch protection on
  `main` requires a PR with passing CI.
- CI ([.github/workflows/ci.yml](.github/workflows/ci.yml)) runs
  `make check-fast` and the integration suite on every PR.
- Nothing private goes into the repo, its commits or its PR descriptions:
  never quote the rulebook verbatim (paraphrase it), and never copy files in
  from the private folder.

## Coordinating sessions

- **One coordinating session**, in the main checkout. It plans, starts one
  session per independent piece of work, and reviews each piece's draft PR
  before marking it ready (`gh pr ready`) for the owner. It doesn't write
  product code itself.
- **Each work session gets its own worktree and branch.** No session starts
  sessions of its own — work fans out one level, never two.
- **Work sessions follow the `work-session` skill** (in `.claude/skills/`):
  start-up check, house rules, verification bar, tracker and note, git.
  `test-worktree` and `drive-plan` cover how to verify. A coordinator's brief
  only needs the task, the files, and what done looks like.
- Pieces that touch disjoint files run in parallel. A pass that touches most
  of the plan files runs alone.
- **Each work session updates [docs/work/TRACKER.md](docs/work/TRACKER.md)
  and writes its hand-over note to `docs/work/notes/<session-name>.md` on
  its own branch**, so both merge with its code and can't drift from it.
  Open questions for the owner go in the note *and* the tracker.

## Testing from a worktree

The docker dev (`:8080`) and e2e (`:8090`) containers mount the **main**
checkout, so they never run a worktree's code. Test a worktree with its own
server and a private database — the recipe is in docs/DEVELOPMENT.md, under
"`make test-e2e` cannot validate a worktree branch". Never point an integration run at `uneasy_test` while
e2e is using it — the integration suite TRUNCATEs it.

## Command preferences

Prefer `make` targets over raw equivalents wherever one exists. They encode
the right flags (`-count=1`, `-p 1`, the integration build tag, the test DSN)
and keep behaviour consistent. Use `make test`, `make test-integration`,
`make check`, `make check-fast`, `make sqlc`, `make build`. Fall back to raw
`go test` / `golangci-lint` / `sqlc` only when no make target fits.

## Bash allowlist conventions (`.claude/settings.json`)

- Use the prefix form: `Bash(cmd subcmd:*)`, not `Bash(cmd subcmd *)`.
- Match the command family, not specific flags or test names.
  `Bash(go test:*)` covers any go test invocation — don't enumerate per-test
  entries.
- Read-only utilities (`grep`, `sort`, `ls`, `find`, `head`, `tail`, `echo`)
  are safe to allow broadly with `:*`.
- Anything that mutates external state (deploys, DB writes outside the test
  DSN) stays out of the allowlist so it prompts. `git push` is the
  exception: it's allowed because the guard hook and branch protection
  already keep pushes off `main`.
- Per-machine entries go in `.claude/settings.local.json`, which is
  gitignored.
