---
name: commit
description: Commit work to git in this repo — group it into logical commits and write messages that keep the reason for each change, including what was rejected. Use whenever committing, tidying a branch's history before or after a push, or writing a commit message.
---

# Committing

A commit message is where the **reason** for code lives. The diff shows
what changed; the message keeps why — the goal, the constraint that shaped
it, and the alternative that was turned down. `main` keeps every branch
commit (PRs merge with a merge commit), so `git blame` leads any future
reader from a line straight to its message.

**Write for someone who has only this public repo, months from now.** They
can't see this conversation, the private rules folder, the design
principles or your memory. Every line must make sense to them.

## 1. Checkpoint freely; push only verified work

Commit whenever it helps while you work; checkpoints get tidied in step 4.
The history you push for review holds only work whose checks pass
(`work-session` §4).

## 2. Group into logical commits

Each commit is one logical change: one fix, one feature, one refactor. Files
that belong to the same change go together; stage them by name
(`git add <files>`). Interactive staging (`git add -p`) doesn't work here.

The tracker files (`docs/work/next/`, `docs/work/questions/`) and hand-over
note go in their own last commit
(`Tracker and hand-over note: <piece>`, or `Tracker: <piece>` when there's
no note), so the code commits read cleanly.

## 3. Write the message

**Subject:** imperative, under about 70 characters, in the game's terms,
saying what changes for players or developers. When the change belongs to
one plan or area, lead with it:

```text
Host Festivity: a duel pauses the whole table
Fix the duel leaking the declarer's die; announce the call
```

**Body**, sized to the decision:

- **Mechanical change** (typo, rename, formatting, dead code): no body; the
  subject says it all.
- **A real decision:** a body with the goal and the constraint that shaped
  the change, then — the most valuable line — the **alternative that was
  rejected, and why**. Include one only if it was genuinely considered;
  never invent one to fill the shape. Don't restate the subject.
- **No automated check covers the change:** end the body with a line
  saying so — `No tests: skill file, nothing covers it.` A mechanical,
  body-less commit doesn't need one.

Where the reason comes from:

- **Authoritative:** the owner's brief, the owner's rulings (design
  principles, memory), the rulebook, an existing test or invariant.
- **Your own call** is not yet the owner's intent. Say so honestly in the
  body ("Not ruled on by the owner: chose X over Y because …"), and list the
  same call under *For the owner* in the PR and in a `docs/work/questions/` file, so the owner
  examines the assumption before merging. Never dress your reasoning up as
  theirs.

Reference only what a reader of this repo can open: a file path, a test
name, another commit, a PR, a rulebook page number. For everything else,
state the substance in a sentence:

| Instead of | Write |
|---|---|
| "per the owner's ruling" | the ruling itself, paraphrased |
| "see design principle 3" | the principle's point, in a sentence |
| "Option B", "as discussed", "step 4 of the plan" | the actual design chosen or rejected |
| a quoted rulebook passage | a paraphrase, with the page number |

Be specific: no "various improvements", no "to clean up the code". A commit
holding several small fixes that couldn't be separated lists them in the
subject or body.

Write multi-line messages through a heredoc, and end every message with the
attribution line your instructions give:

```bash
git commit -F - <<'EOF'
<Plan or area>: <what changes>

<The goal, and the constraint that shaped this change.>

Rejected: <the alternative> — <why it lost>.

No tests: <only if nothing covers it>

<attribution line>
EOF
```

## 4. Tidy before review

Your branch's history is yours to shape until it merges: squash checkpoint
commits, reorder, reword, split. After rewriting a pushed branch, push with
`git push --force-with-lease` (it refuses if someone else pushed to the
branch since you last fetched). Force pushes are fine on your own branch;
the guard hook blocks only pushes to `main`.

**Done when** `git log --format='%h %s%n%b' main..HEAD` reads as a short
story a stranger could follow: one logical change per commit, and every
real decision with its reason.
