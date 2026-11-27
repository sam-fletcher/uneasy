# Coordinator: the holistic question pass (2026-11-26)

The coordinating session that took over on 2026-11-26 and went through every open owner question at
once. This note hands the rest of the plan to the next coordinator. Rulings are recorded in the
private rulings and decisions files, never here.

## What was done

- **79 open questions down to 45.** Three dropped as already ruled or stale, about 30 settled
  through a consent list where "yes" kept what's built, five settled from rendered rulebook pages, and
  the bug-shaped ones moved to `next/` (PR #133).
- **Built and merged:** #134 (Make War: state 404, accept votes logged), #135 (Host Festivity), #136
  (Make Demands), #137 (small UI), #138 (small ops), #139 (book-rule fixes).
- **Session models:** Sonnet for open work, Haiku at high effort for settled work, where the test is
  "would two careful developers given this brief make essentially the same PR?" Reviews record which
  model built each PR. So far: #135 (Sonnet) had one must-fix, a host name in plain text; #136 (Sonnet)
  went back once for stale screenshots; #137 and #139 (Sonnet) and #138 (Haiku) were clean.

## The plan from here, in order

The questions are grouped by the design decision underneath them, not by screen. Grouping by screen
had the same decision ruled three times (a watcher's view of someone else's pick comes up on four
screens). Visual decisions go on the private design canvas as boards, because text settles them
badly.

1. **Board 1: the screens outside a plan, with the Prologue.** Lobby, Prologue, Shake-Up, ending vote
   and ended screen. The decision underneath: once the Waiting On bar goes, how these screens say whose
   move it is, given that the "who-line above the step heading" grammar was ruled only for plan steps.
   This board unblocks `redesign-prologue` and part of `redesign-cleanup`. Questions:
   `step-ask-heading-lobby-shakeup-vote`, `table-title-wording`, `shake-up-repeated-words`,
   `shake-up-act-track`, `record-shakeup-pips`, `ended-standings-layout`, `player-row-details`,
   `lobby-primer-gold-title`, `standing-strip-closing-line`, `closing-stage-length`, plus the track
   codes `redesign-prologue` leaves to mockups.
2. **Board 2: someone else's pick.** The system has "selected" (yours) and "locked", but no "theirs".
   `plan-grid-redesign`, `scene-watcher-picks`, `scene-labels-and-fields` (the typed-place half),
   `decree-join-mirror-length`, `duel-call-and-tray`.
3. **Board 3: notices and app chrome.** `notice-marks`, `shake-up-notice-tones`, `update-banner-tint`,
   `connection-notice-look`, `push-blocked-rule`.
4. **Board 4: the marks sheet.** `star-mark`, `chat-picker-chevron-direction`, and the at-risk triangle
   from `player-row-details`. Small enough to ride with board 3.
5. **Board 5: wording, "new", and controls on a phone.**
   - Heading and line wording: `step-label-liaise-phases`, `step-label-several-picks`,
     `step-label-two-asks`, `scene-panel-focus-line`, `kept-secret-line`.
   - What "new" means: `own-writes-not-new`, `seen-across-devices-choices`, `profile-finished-unread`.
   - Controls on a phone: `rename-an-asset-affordance`, `sheet-grab-bar`, `plan-panels-b-main-button`.
     For the last, the war's join pattern (choice chips, then one main button) already suggests the answer.
6. **The log's grammar.** The log guard checks that board changes are logged, never what the line
   says, so these are one missing grammar: `chronicle-mar-announcement`, `mi-mar-post-lists-fates`,
   `break-importance-owner-only`, `festivity-abandoned-peer-break-severity`. `log-guard-followups`
   waits on this round and on the owner's approval.
7. **The remaining rules calls,** with rulebook pages rendered as images: `spread-propaganda-interferer-tie`
   (and whether banked dice count toward "the most assets"), `spread-propaganda-short-mar`,
   `riposte-last-marginalia`, `giving-away-a-main-character`, `make-demands-pre-roll`,
   `make-war-peace-proposal-loose-ends`. Also proposed: one standing rule for a recurring bug shape,
   "the server refuses anything the UI wouldn't offer".
8. **Triage when a question is filed** (a change to the `work-session` skill and the tracker README,
   if the owner agrees). If nothing is blocked and the default is easy to reverse, the point goes in
   the PR's "For the owner" section with a screenshot, and the merge accepts it. A point that waits on
   playtest evidence goes on a watch list, not into `questions/`.

**Answered later on 2026-11-26:** the war's words (the cost of battle is paid, never owed; a
surrender's opponents take spoils of war) and peace terms (kept on the Laws tab as treaties, and the
war's closing line quotes them). Both are built by the items below. Whether terms can do anything
mechanical joins `make-war-peace-proposal-loose-ends` for the rules round.

## Ready to build, no question open

- `festivity-duel-terms`, `render-crash-report`, `make-demands-overlay-stand-in-line`,
  `check-fast-fresh-worktree` and `war-cost-and-spoils-words`: each brief is settled, so Haiku at high
  effort. Each touches its own files, so they can run in parallel.
- `war-peace-treaties` after `war-cost-and-spoils-words`, since both touch the peace handler. Sonnet:
  it crosses the server, the laws payload and the "new" dot.
- Then the queue: `redesign-prologue` (after board 1), `inline-text-names-in-strings`,
  `set-piece-candidates`, `plan-gain-recipient-helper` (alone: it touches most plan panels),
  `redesign-cleanup` last.

## Noticed, not fixed

- Three question files deleted on 2026-11-03 had left comments in e2e specs pointing at them. This
  branch rewrites those comments to say what was settled, and found the answer for each in the code.
- If the next design pass finds a general design rule (as "a box marks a thing you can touch" is), it
  should be written down as a rule with a test for applying it. Filed as a one-off case, it can't stop
  the next screen from asking again.
