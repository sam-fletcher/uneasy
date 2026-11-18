# A Spread Rumors rumor's text is never streamed to the table as it is typed

## What changed and why
Ticking "Keep it secret for now" on Spread Rumors promises that others won't
see the text until the rumor is spread. The submitted rumor kept it; the
**preparation draft did not**. While the preparer typed, `SpreadRumorsPanel`
sent `notes` in every draft, ticked or not. `PlanPanel` turned that into the
`uneasy:prepare_plan_draft` window event, `ws.ts` sent it as
`prepare_plan.draft`, and `hub.go` broadcast it to every client at the table, so
each watcher's read-only mirror filled in the secret text keystroke by
keystroke. Nothing was stored; it was a live leak, on `main`.

### The ruling (owner, 2026-11-16)
My first version stopped the text once the box was ticked. The review pointed
out that the form puts the text box above the checkbox, so the natural order
(type the rumor, then tick) shows every watcher the text before the tick, and
wiping it afterwards can't un-show it. The owner ruled **never stream the
rumor's text at all, ticked or not**: showing words as they are typed is the
least important thing a watcher's read-only screen does, a nicety that must
never cost anything else. Watchers keep the target asset, whether it is secret
and which asset holds it (a secret's existence is public); the text reaches the
table when the plan is prepared, as every plan's notes do. The general rule, now
written at the relay and in `prepDraft.ts`: **a live draft must never carry
text that could become a secret.** Rejected along the way: reordering the form
(narrows the gap, can't close it) and a warning in the placeholder.

- **Client.** `lib/plans/prepDraft.ts` (`spreadRumorsPrepDraft`) builds the
  draft: filter owner, target asset, `keep_secret`, holder asset, and no
  `notes` key. The panel no longer re-emits on each keystroke of the text,
  since typing sends nothing.
- **Watcher view.** Where a watcher's text box was: ticked, "Kept secret for
  now. Only alice can read it."; unticked (or the form not started),
  "alice is writing the rumor." Both are `.form-hint` plain secondary text; they
  read straight from the draft, not from effect-copied state, so they are right
  on first paint and testable without a browser. "Hidden under:" is unchanged.
  The preparer's own form is unchanged.
- **Server.** `hub.redactPreparePlanDraft` deletes `notes` from a draft that is
  labelled `spread_rumors` **or** carries a `keep_secret` key of any value
  (`true`, `false`, `"true"`, `1`, `null`). The previous client always sent
  `keep_secret` with a Spread Rumors draft and stamped `plan_type` when its
  150ms send timer fired (`PlanPanel.emitPrepDraft`), while `LazyPlanPanel` keeps
  the old panel mounted as the next card loads, so a preparer who types and
  taps another card sends the text under the *next* plan's label; the key
  covers that in both states. Only Spread Rumors' panel sends `keep_secret`.
  Comments at the relay, on `PreparePlanDraftPayload` and on
  `PlanContext.emitPrepDraft` state the rule.
- **Tracker:** deleted `next/spread-rumors-secret-draft-mirror.md` and the
  answered `questions/spread-rumors-stream-rumor-text.md`; added
  `questions/kept-secret-line.md` (the two lines' wording, my call).
- **Outside the repo:** the ruling is recorded in the private rulings file and
  the hidden-information memory, per the project's habit.

### The audit the owner asked for
"If any other plans include the writing of secrets, they might have the same
bug." Three things confirmed:

1. **`scene_setup.draft` carries nothing hidden.** Its payload is the holding,
   a custom place, the time elapsed, a time note and the present peers
   (`SceneSetupDraftPayload`). All of it is the scene's public header and post
   the moment the scene opens; neither `SceneSetupForm.svelte` nor
   `handler/scenes.go` mentions a secret. (A Liaise scene is public like any
   other; secrecy there works at the choice level.)
2. **No other secret text passes through either relay.** The hub accepts
   exactly four WebSocket commands from a client: `typing.start`,
   `typing.stop` (a boolean, no text), `scene_setup.draft` and
   `prepare_plan.draft`; `ws.ts` has three `send` sites and no others. Secret
   text is typed in three places: `handler/secrets.go`'s "write a secret" box in
   `RetinueView` (no relay reference at all), `plan_clandestinely_liaise_phases.go`'s
   kept-secret text (typed in `ClandestinelyLiaisePanel`'s resolve mode, sent on
   submit; its one `emitPrepDraft` sits behind `mode !== 'prep'`), and
   `plan_spread_rumors_routes.go`'s hide-source (resolve mode, sent on submit).
   Every one of the 13 files that call `emitPrepDraft` was read: each draft
   holds public picks and a prep note that `plan.prepared` posts to the whole
   table the moment the plan is prepared. Clandestinely Liaise's prep note is
   public on purpose (its `PreparedDescriptor` names the peers and the note,
   "a secret meeting between X and Y"); the meeting's secret is written at
   resolve, in a different input. Spread Rumors was the only draft holding text
   that can become a secret.
3. **No other live channel streams typed text before it is submitted.** No
   `EventSource`/SSE, no polling (`setInterval` appears nowhere in the
   frontend), no server-side draft-saving route (the one `draft_id` is Host
   Festivity's peer drafts, which are submitted state). Chat sends text only on
   send; its typing indicator is a boolean.

## How it was verified
Everything against this worktree's own code: a server built with `make build`
on `:8183` with a private database (dropped afterwards). "Before" is `main`'s
dev stack on `:8080` at the same commit.

**The flipped tests, failing on the previous head** (the version that stripped
only a ticked rumor) and passing now:

| Test | On the previous head | Now |
|---|---|---|
| `prepDraft.test.ts`, "an open rumor goes out without its text" and "never carries the text… ticked = false" | FAIL: the open draft still has `notes` | pass |
| `prepMirror.test.ts`, a watcher of an open rumor / of an unstarted form | FAIL: a `<textarea>` and no "alice is writing the rumor." | pass |
| `hub` `…StripsWhateverKeepSecretSays` (`keep_secret:false` among the values) | FAIL: `keep_secret=false let the text through` | pass |
| `hub` `…StripsASpreadRumorsDraftWhateverPlanItIsLabelled` (seek_answers, chronicle_histories, `""`, each with the flag true and false) | FAIL: the text got through under another label | pass |
| `hub` `…StripsASpreadRumorsDraftWithNoFlagAtAll` | FAIL: a Spread Rumors draft's text reached another client | pass |
| e2e `spread_rumors_secret_draft.spec.ts`, against `main` on `:8080` | FAIL: no "alice is writing the rumor." (and frames carry the text) | pass on `:8183` |

Unchanged cases that pass before and after, so they pin what must not change:
the ticked-box cases, other plans' notes still going out
(`…LeavesOtherPlansAlone`), a bare card highlight (`prep: null`) passing through,
the preparer's own form keeping its text box.

**An old client against the new server, live.** Two raw sockets on a table;
alice's sends the old client's message (text included) in five shapes. On this
branch carol's frames carry the rumor in **none** of: `spread_rumors` with the
flag true and false, `seek_answers` with it true and false, and an empty plan
type. The same five to `main`'s server all arrive with the text.

**Existing spec for this view:** `checkbox.spec.ts` (both tests, including the
keep-it-secret one that checks a watcher's mirror of a ticked box) passes on
this branch's server, with the new spec: 3 passed.

**Gates:** `make check-fast` passed (Go unit, vet, deadcode, svelte-check, 93
Vitest files / 1789 tests, hook tests, tracker check). `make test-integration` passed on a private
database (`handler` 205s), also on this code.

**Screenshots at 375px** (dev-seeded game; alice prepares, carol watches): a
watcher with the box unticked ("alice is writing the rumor.") and ticked ("Kept
secret…"), and, from the earlier round, the leak on `main`. They are in the PR.

**Not run against this code:** the full Playwright suite (only `checkbox.spec.ts`
and the new spec, and the new spec against `main`).

## Behaviour changes to look at before merging
- **Merge danger: WebSocket payload.** `prepare_plan.draft` for Spread Rumors no
  longer carries `notes`, ever. A watcher's tab still on the old client, left
  open across the deploy, draws the old empty text box until it reloads (the old
  client has no "writing" line); it never shows the text. A revert restores the
  leak; nothing stored changes.
- Watchers no longer watch a rumor being typed. For a public rumor they see
  "alice is writing the rumor." and the full text only when the plan is prepared,
  like every plan's notes. That is the ruling.
- Other plans' prep text is still streamed as typed (it is public at prepare
  time). If one ever gains a field that can become a secret, it builds its draft
  in `prepDraft.ts`.

## Open questions for the owner
- The wording of the two watcher lines, and whether they should be plain hints
  or wear a house mark: `questions/kept-secret-line.md`. My call, not ruled on;
  not a blocker.

## Noticed, not fixed
- **Any client can broadcast a `prepare_plan.draft`, not just the focus
  player.** The hub stamps `player_id` but never checks it is the focus player,
  and `PlanPanel` doesn't compare `player_id` with the focus player before
  mirroring. A bystander's client could make everyone's mirror show a draft.
  Not a leak (it's their own text, and Spread Rumors' carries none now), and
  `scene_setup.draft` works the same way, so it wants one decision for both.
