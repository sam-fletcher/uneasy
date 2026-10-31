---
name: review-pr
description: The coordinator's review of a work session's draft PR before it goes to the owner — fast pre-checks, then separate reviews against the brief, the game rules and design principles, and the house rules, then send it back or mark it ready. Use when a work session reports its draft PR, or the owner asks for a PR to be reviewed.
---

# Reviewing a draft PR

The coordinator reviews every draft PR before the owner sees it. The owner
should receive PRs that do what was asked, follow the rules as written,
and break none of the house rules — so their review can focus on design
and feel.

## 1. Gather

- `gh pr view <n> --json title,body,headRefName,commits,files` and
  `gh pr diff <n>`.
- **The brief** you gave the session (in this conversation, or its tracker
  row). It's the spec this PR is judged against. If there is none — the
  owner asked you to review a PR from elsewhere — skip the Brief axis and
  say so in the review.
- The hand-over note in the diff (`docs/work/notes/<session-name>.md`).

## 2. Pre-checks

Cheap, mechanical, and enough to send it straight back:

- **CI** passes (`gh pr checks <n>`).
- **Up to date:** after `git fetch origin`,
  `git rev-list --count origin/<head branch>..origin/main` is 0. If not, the
  session rebases and re-runs its checks first.
- **The PR body** has every `pr` skill section, with Evidence before and
  after, and says what wasn't verified.
- **Commits** follow the `commit` skill: logical, and real decisions carry
  their reason.
- **Tracker row and hand-over note** are in the diff. (A coordinator's own
  setup PR has no work session, so a tracker row is enough.)
- **Public repo:** search the diff, PR body and commit messages for
  `_RULES`, `DESIGN_PRINCIPLES`, `adr/` and `/Users/`, and read each hit. A
  passing mention of a private file is the existing style; copied private
  text, a quoted rulebook passage, or a personal path is a must-fix.

## 3. Review on separate axes

Start one subagent per axis, in parallel, each with a fresh context. Give
each the diff command, the commit list and its brief below, and ask for
under 400 words, sorting findings into **must fix** and **judgement call**.
Separate reviewers keep one kind of problem from hiding another: a PR can
do exactly what the brief asked and still get a rule wrong.

**Brief.** Pass it the brief. "What the brief asked for that is missing or
partial; what the diff does that wasn't asked for (scope creep — the
session should have noted it, not fixed it); what looks done but done
wrong. Quote the brief for each."

**Rules and design.** Pass it the private paths: the rules files for each
plan or phase the diff touches, the rulebook (read pages as images with
the render script; the book is the final word where files disagree), the
design principles, and the memory index, whose entries hold the owner's
past rulings — without them the reviewer will flag a ruled deviation as a
mistake. "Where does the change
depart from the rules as written, or simplify them? Where did the session
interpret a rule the owner hasn't ruled on? Which design principles apply,
and does the change follow them? Cite the page or principle." Its findings
quote private material, so they stay in this conversation.

**House rules.** Pass it the `work-session` §3 list, `docs/STYLE_GUIDE.md`
for UI changes, and this smell list. "Check every house rule against the
diff. Is each fix pinned by a test that would fail without it? Flag these
smells as judgement calls, never must-fixes: a name that doesn't say what
it holds; the same logic in two places; a function reaching into another
type's data more than its own; the same few parameters always travelling
together; the same `switch` on the same type in several places; one change
forcing edits across many files; flexibility nobody asked for."

**Correctness.** When the diff changes Go or Svelte logic, also run the
built-in `code-review` skill on the PR for bugs.

## 4. Check the evidence

Compare the Evidence section with the verification bar in `work-session`
§4. A UI change needs 375px screenshots from each viewpoint that changed; a
fix needs its test shown failing without it. If a claim looks doubtful,
re-run that one cheap check yourself against the branch (`test-worktree`).

## 5. Decide

Report the axes **side by side**, not merged into one ranking, then:

- **Any must-fix** → send the findings to the work session that wrote it
  (it still has the context), paraphrasing anything private. It fixes,
  tidies its history, force-pushes and reports back; review what changed.
- **Questions only the owner can answer** → the PR's *For the owner*
  section and the tracker's "Waiting on the owner".
- **Clean, or judgement calls only** → post a short review on the PR and
  mark it ready:

  ```bash
  gh pr comment <n> --body-file <scratchpad>/review.md
  gh pr ready <n>
  ```

  The comment is public: per axis, one line on what was checked and the
  verdict, plus the judgement calls worth the owner's eye. Paraphrase
  rules findings; never cite the private files. (GitHub won't let the
  account that opened a PR approve it, and every session uses the owner's
  account, so a comment stands in for an approval.)

Tell the owner the PR is ready, with its URL.
