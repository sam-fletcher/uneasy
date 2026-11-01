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
- **Active work** — [docs/work/](docs/work/): the tracker (one file per
  item in `next/`, per owner question in `questions/`; `make tracker` prints
  it) and each session's hand-over note. They live in the repo so that progress
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
  reviews it (the `review-pr` skill) and marks it ready; the owner reviews
  and merges.
- **Everything stays on the latest `main`.** A hook
  ([.claude/hooks/sync_main.sh](.claude/hooks/sync_main.sh)) runs when a
  session starts and whenever the owner sends a message. It fast-forwards
  the main checkout, so the dev containers run it; fast-forwards any
  worktree branch with no commits yet; and tells each session which PRs
  have merged since it last looked ("sync_main: merged into main …"). Act
  on that notice — a coordinator's piece is done, a work session rebases.
- Sessions never merge and never enable auto-merge. A hook
  ([.claude/hooks/guard_git.py](.claude/hooks/guard_git.py)) blocks
  `gh pr merge` and any push to `main`; branch protection on
  `main` requires a PR with passing CI.
- CI ([.github/workflows/ci.yml](.github/workflows/ci.yml)) runs
  `make check-fast` and the integration suite on every PR.
- PRs merge with a merge commit, so branch commits stay in `main`'s
  history. Commit messages carry the reason for each change (the `commit`
  skill); tidy and force-push your own branch freely before it merges.
- Nothing private goes into the repo, its commits or its PR descriptions:
  never quote the rulebook verbatim (paraphrase it), and never copy files in
  from the private folder.

## Coordinating sessions

- **One coordinating session**, in the main checkout. It plans, starts one
  session per independent piece of work, and reviews each piece's draft PR
  (`review-pr`) before marking it ready for the owner. It doesn't write
  product code itself.
- **`retro`** looks back at a finished session and proposes changes to
  this setup, when the owner asks for one.
- **Each work session gets its own worktree and branch.** No session starts
  sessions of its own — work fans out one level, never two.
- **Work sessions follow the `work-session` skill** (in `.claude/skills/`):
  start-up check, house rules, verification bar, tracker and note, git.
  `test-worktree` and `drive-plan` cover how to verify. A coordinator's brief
  only needs the task, the files, and what done looks like.
- Pieces that touch disjoint files run in parallel. A pass that touches most
  of the plan files runs alone.
- **Each work session deletes or edits its own item file in
  [docs/work/next/](docs/work/next/), adds a file per new item or owner
  question, and writes its hand-over note to
  `docs/work/notes/<worktree-folder>.md` on its own branch**, so all of it
  merges with its code and can't drift from it. Open questions for the owner
  go in the note *and* a file in `docs/work/questions/`.
- **No file that every PR edits.** Parallel PRs editing one shared file (a
  numbered list, an index, a table) conflict on every merge. Give each entry
  its own file, and generate any overview instead of committing it.

## Testing from a worktree

The docker dev (`:8080`) and e2e (`:8090`) containers mount the **main**
checkout, so they never run a worktree's code. Test a worktree with its own
server and a private database — the `test-worktree` skill has the recipe.
Never point an integration run at `uneasy_test` while e2e is using it — the
integration suite TRUNCATEs it.

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
  DSN) stays out of the allowlist so it prompts. The commit-to-PR flow is
  the exception — `git commit`, `git fetch`, `git rebase`, `git push` and
  `gh pr create/ready/edit/comment` — because the guard hook blocks what
  in it could reach `main` (a merge, a push to `main`, a commit made on
  `main`, or a merge smuggled in through `rebase --exec` or
  `--upload-pack`), branch protection backs it up, and the owner still
  merges.
- Per-machine entries go in `.claude/settings.local.json`, which is
  gitignored.
