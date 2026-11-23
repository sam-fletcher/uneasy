# Hand-over: retro tooling (two checks)

Branch `claude/retro-tooling-a02d30`. No tracker item: the owner approved both
checks in a retro. The worktree folder is `redesign-plan-panels-c-e37a80`, but
the branch is the one this note names.

## What changed and why

- **Question files must say what was checked.** `docs/work/tracker.py --check`
  (and `make tracker-check`, already in `make check-fast`) now fails a file in
  `docs/work/questions/` with no `checked:` field, or an empty one. The field
  holds the rulings, principles, decisions, ADR or style-guide sections read,
  and why none settles the question. About a third of the question files
  filed were already answered by a ruling, which cost the owner time. The 77
  question files on `origin/main` are frozen in `LEGACY_QUESTIONS` and exempt.
  `docs/work/README.md` has the table row, the one-line exemption and a
  two-line example.
- **A hook blocks unquoted zsh globs in `--include=`/`--exclude=`.**
  `.claude/hooks/guard_zsh_globs.py` is a PreToolUse hook on Bash, modelled on
  `guard_git.py`, registered beside it in `.claude/settings.json`. zsh expands
  an unquoted `--include=*.ts` itself, so grep searches nothing and prints
  nothing, which reads as "not found". It happened about 18 times in one
  session. The hook blocks (exit 2) an unquoted, unescaped `*`, `?` or `[` in
  such a value, with a one-line message showing the quoted form. Quoted and
  escaped values pass, and so does a glob elsewhere in the command (`ls *.md`).
- `make test-hooks` now also runs `.claude/hooks/test_guard_zsh_globs.py`, so
  `make check-fast` covers the new hook.

## How it was verified

- **Question check, failing first.** Before the change, a question file with
  no `checked:` passed the check (exit 0). After it, the same file failed
  (exit 1, `missing 'checked'`), and so did an empty `checked:`. A file with a
  `checked:` value passed, and the existing tree still passed. The probe file
  was removed afterwards.
- **Glob hook, failing first.** `test_guard_zsh_globs.py` (43 cases) was run
  against a stub that blocks nothing: 14 block cases failed. Against the real
  hook: 43 cases, 0 failed. `make test-hooks` passes both suites
  (`guard_git` 61 cases, 0 failed).
- **Hook by hand.** Sample JSON piped into `guard_zsh_globs.py`:
  `--include=*.ts` → exit 2; `--exclude=*.test.ts` → exit 2; `--include='*.ts'`,
  `--include="*.ts"`, `--include=\*.ts`, `ls *.md`, and
  `git grep -n PATTERN -- '*.ts'` → exit 0.
- **`make check-fast`:** exit 0 on this branch, after both commits. Go packages
  all `ok`; `guard_git` 61 cases and `guard_zsh_globs` 43 cases, 0 failed; the
  tracker check passed; svelte-check reported 0 errors.
- **Not run:** the integration suite and e2e. No Go, server or frontend code
  changed. The `settings.json` entry was checked for valid JSON and for the same
  shape as the `guard_git.py` entry, but the hook has not been exercised through
  a live Claude session. It takes effect for sessions that load settings after
  this merges.

## Behaviour changes to look at before merging

- Every Bash call in a session that loads these settings now also runs
  `guard_zsh_globs.py`. A command with an unquoted glob in `--include=` or
  `--exclude=` is blocked, with the quoted form in the message.
- A new question file needs `checked:`, or `make check-fast` fails. The 77 legacy
  files are exempt. Deleted legacy names left in `LEGACY_QUESTIONS` are harmless.

## Open questions for the owner

None. Both checks were approved in the retro.

## Noticed, not fixed

- The glob hook covers only the `--include=VALUE` and `--exclude=VALUE` forms.
  It does not cover the space-separated `--include *.ts`, a glob inside
  `$(...)` (`--include=$(echo *.ts)`), or a glob inside `bash -c "..."`, which it
  reads as quoted. Each is rare. Extend the hook only if one recurs.
