# Work tracker

What's next and what's waiting on the owner, kept in the repo so progress
lands in the same PR as the code it describes. `make tracker` prints the
whole picture.

Older history — the 12-plan UX audit's findings, rulings and the hand-over
notes up to 2026-10-30 — is private and lives outside the repo (see the
private CLAUDE.md).

## One file per entry

| Folder | One file per | Front matter |
|---|---|---|
| [next/](next/) | piece of work still to do | `title`, `order`; optional `after:` or `blocked:` naming another file |
| [questions/](questions/) | question waiting on the owner | `title` |
| [notes/](notes/) | work session's hand-over note | none |

This used to be a single numbered `TRACKER.md`. Every PR edited it — each
deleted its own item and renumbered the rest — so any two parallel PRs
conflicted, and every merge sent the others back to rebase. With one file
per entry, a PR only adds or deletes its own files, and two PRs never touch
the same lines.

**The rules that keep it conflict-free:**

- **Finishing an item:** delete its file in your PR. Partly done: edit only
  that file.
- **New item or question:** add a new file with a short, specific slug.
  Never append to someone else's file.
- **Never renumber.** `order` only sorts the list. Leave gaps (10, 20, 30…)
  and pick an unused number when you add an item. Equal numbers are fine.
- **Answered question:** delete its file, in the PR that acts on the answer
  (or the coordinator's next setup PR).
- **No "in progress" entries.** Open PRs are the list of work in flight
  (`gh pr list`).
- **No shared index file.** Anything every PR would edit belongs in this
  script's output, not in a committed file. `make check-fast` fails if
  `TRACKER.md` comes back or a file's front matter is malformed.

The repo is public: paraphrase the rulebook, never quote it, and copy
nothing in from the private folder.

## Never audited

Chronicle Histories through the UI; the Make Demands overlay interacting
with other plans; Propose Decree's monarch case; tables of 4+ players.
