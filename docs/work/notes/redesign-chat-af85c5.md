# redesign-chat-af85c5

The Chat pane and the Tones sheet, rebuilt to the signed-off mockups. Wave 4 of the visual redesign.

## What changed and why

**The feed** (`ChatPanel.svelte`, off its `tokens: not migrated` marker) now follows the rulings:

- **Dividers are centred, in three weights** (new `components/chat/FeedDivider.svelte`): the day is
  faint, where the new messages start is main parchment and carries the count ("4 new", read out as
  "4 new messages"), a row boundary is deep gold. The old filled row-number pill is gone.
- **A plan's resolution is one header row** (new `components/chat/SpanHeader.svelte`): mark, name, the
  preparer's shape, a status word, a caret, then its lines indented behind a thin rule. No card. The
  status is "Resolving" while it runs, then Make (green) / Mar (red) / "Fell through" / "No effect"
  in plain text; **Make War and Clandestinely Liaise show no Make or Mar**, since the book gives them
  neither (the app stores "make" for both when they end). The plan's closing sentence stays under the
  rule when the span is folded. A folded span with something unread carries the pale dot, replacing
  the old count pill. The words live in `$lib/chatView.ts` so they are tested without a DOM.
- **A scene uses the same row** (its banner, "Live" while it runs, a caret). A plan that stages a
  scene folds into the scene's container as before; its header is the plan's.
- **War's lines carry the barbute in war red**: `markForCode` sends `plan.make_war` (what every
  war line is posted under) to the `war` family, and a Make War plan's header wears it too.
- **Messages** have the player's shape beside the byline (12px, the shape's floor). The sunken ground
  behind in-character speech is gone; small capitals in the mask colour tell it now.
- **Header and composer.** The Tones chip is as the shell left it; "Hide bookkeeping" is now a switch
  (a native checkbox with `role="switch"`, so a screen reader says "switch"). The composer has the
  box, the house main button (greys out until there is text) and "Speaking as" above them, a control
  with the caret and the pick's tick. The keyboard still opens only when the box is tapped.
- **Marks and boxes that went:** the ranking card's border and fill, the outcome box, the scene card,
  the typed `↓ ⬇ ● ▴ ▾` characters, the message counts, the unread count pill, the shadow on the
  floating "N new below" / "Return to now" chips (now house chips).

**The Tones sheet** (`table/TonesSheet.svelte`): before the Main Event it is the tile grid as it was,
with the lede's box and gold rule gone and the "+ Add" button on the house main button. Once tones
lock it lists only the topics someone set, grouped Never, Avoid detail, Include (`$lib/toneGroups.ts`),
with a line counting the rest.

**Chat's dot.** Review found a way to clear it for messages nobody saw, and chasing it found a second:

1. **Leaving the pane flushed the newest post.** One tap on a tab now, and a reader parked on the
   "6 new" divider who tapped Play marked all six read. Fixed: a report now covers only the newest
   post that was on screen when it was *scheduled* (the one place the visible / window-visible /
   near-the-bottom guards are checked); `reportReadMarker` takes that as `upTo`; leaving the pane
   flushes only a report that is already pending.
2. **The arrival scroll never landed on the divider.** The one-shot "land on the unread divider"
   scroll was overwritten: writing `didInitialScroll` (it was `$state`) re-ran its own effect before
   the landing ran, and that second run queued a scroll to the bottom. So a phone reader with a long
   unread run always arrived at the bottom, where the read marker moved two seconds later: the new
   messages cleared without being on screen. **This is on `main` too** (I checked: it lands at the
   bottom and reports the marker). It is one word (`didInitialScroll` is a plain variable now) and in
   my file, but it changes where every table opens its Chat when there is unread, so it is listed under
   behaviour changes: **you can veto it** by reverting that one line, though the first fix then has little
   to protect on a phone.

A new e2e parks a reader at the divider with a long unread run, checks they arrived *on* the divider,
waits out the debounce, taps Play, and checks the marker did not move and the dot stayed; scrolling to
the bottom then clears it. I added an e2e for the column name at three columns too (the shell's covered
the tab).

**Files outside my list** (kept small): `lib/chatFeed.ts` (the war override and its comment, and
`reportReadMarker`'s `upTo`; `chatFeed.test.ts` has the tests) and `routes/table/[id]/+page.svelte` (**one line**: `{plans}` is
passed to `ChatPanel`, which the preparer's shape and the war mark need). `LogMark.svelte` is comments
only: the war family was already drawn; I removed mentions of the mobile chat bar. `docs/STYLE_GUIDE.md`
has a new "The chat feed" subsection and one corrected line in "Log marks".

## How it was verified

Against this worktree's code (a private server on `:8176`, private database):

- `make check-fast`: passes (exit 0). 1067 unit tests, `svelte-check` clean apart from the
  `.type-note` warning in `HelpContent.svelte` that was already there.
- **e2e.** The whole suite passed twice (187) on the first version. After the review fixes I ran it a
  third time: **186 of 189 passed**. The run took 31 minutes (it is normally under 3) because the
  machine was loaded by other sessions, and the three failures were timing: my "a failed send restores
  the text" spec and two plan specs (`make_introductions` and `seek_answers`, "perform_steps") that hit
  `advance-row` "game not found". **All three pass when rerun alone, and `chat.spec.ts` passes 16 of 16
  on a second run.** I did not rerun the whole suite a fourth time. Updated specs: `chat.spec.ts` (the
  divider is "N new", the switch is a `switch`, scene headers no longer count messages). The plan
  helpers in `helpers/*.ts` still find `.plan-group` / `.plan-header[data-plan-id]` / `.scene-header`:
  I kept those class names and `data-` attributes so other pieces' specs don't break.
- **New unit tests:** `chatView.test.ts` and `toneGroups.test.ts` (pure rules), a `markForCode` test,
  `chatFeed.render.test.ts` (the whole feed server-rendered: the three divider kinds and their counts,
  a plan header's parts, status words, folded vs open, the dot, the barbute, Send disabled, no
  autofocus, the header; plus a stylesheet check that a long scene banner can't squeeze "Live"), and
  `table/tonesSheet.render.test.ts` (both shapes). **Each shown to bite**: I broke the war routing,
  the war-header mark, the war/liaise Make suppression, the Never-first order, Send's `disabled`, the
  divider's count and the folded outcome line in turn and watched the named tests fail; the squeeze
  test failed against the old flex rule.
- **New e2e** (`chat.spec.ts`): a resolved Seek Answers plan at 375 (name, alice's shape, "Make", a
  44px row with no border or fill, folded keeps the closing sentence, tap unfolds it behind a 1px rule
  with no fill); the dot clearing on the Chat column name at 1280, including a message that arrives
  while it is on screen; Send greyed out until there is text, whitespace isn't text, the box is 16px
  and at least 44px tall, nothing has focus on arrival.
- **Review follow-up:** the tab-switch e2e was shown to bite twice, with the old unconditional flush
  put back (the marker jumped to the newest post: "a tab switch marked unread messages as read") and
  with `didInitialScroll` as `$state` again (the divider was not in the viewport on arrival).
- **Looked at** at 375 and 1112 (the Chat column): the feed with a day, "5 new" and a row boundary; a
  folded and an unfolded plan; two war lines; the composer empty and with text; the Tones sheet
  locked and as the grid; a live scene and the open "Speaking as" menu. A before-and-after pair for
  each is in the PR. Looking found one bug (a long scene banner squeezed "Live" to "Liv"), now fixed
  and guarded.

**Not run against this code:** `make test-integration` (no Go changed); a real phone; Safari or
Firefox; hover and focus states (written in CSS, not photographed); a war's *own* plan header in a
browser (no dev route posts a real war: the barbute in a Make War header and the missing Make word
are covered by the unit and render tests, and the war *lines* in the shots were inserted as rows);
the `Speaking as` control for a borrowed peer (a peer controlled by another player).

## Behaviour changes to look at before merging

- **Chat opens on the unread divider** (a quarter of the way down) when there is unread, as the code
  always meant it to, instead of at the bottom. See "Chat's dot" above.
- **The unread divider says "N new", not "New messages".** The count lives there now, since the tab
  has none.
- **No message counts on a scene or plan** ("2 messages", "5 entries"), and no count pill: a folded
  span with something unread shows the pale dot.
- **Important lines are still deep gold text, but lost the left rule.** The colour rulings give deep
  gold to important log lines; "important chat gets only the pale dot" is about the tab and the old
  strip's gold border, not the feed's text. (I had filed this as a question; review answered it, so
  the file is deleted.) The rule went because it would run into the thin rule a span hangs its lines
  from, and colour already says "important".
- **"Speaking as" closes on Escape and on a tap outside**, as well as on choosing (it used to close
  only by choosing). Escape returns focus to the control, never to the message box.
- **A Tones tile's edge is the control border** (`--color-border-control`). It was a faint white
  wash (`rgba(255,255,255,0.08)`); no token is a translucent edge, so I used the nearest one. On a
  green, gold or red tile it reads as a thin grey outline; say if you would rather it were none.
- **In-character speech has no ground**; table-talk in a scene is the same size as any message.
- **The floating chips read "N new below" and "Return to now"**, with no arrows.
- **"Hide bookkeeping" is a switch**, and its on state is the bright-gold pick look.
- **A resolved plan's closing sentence no longer has a "Make"/"Mar" label beside it**: the header's
  status word says it once.

## Open questions for the owner

- [chat-speaking-as-outside-scenes](../questions/chat-speaking-as-outside-scenes.md): the control only
  shows when there is a choice of voice.
- [chat-scenes-and-speech](../questions/chat-scenes-and-speech.md): scenes and in-character speech
  were never mocked up; the choices I made are listed.
- Still open and left alone, as asked: [tones-chip-wording](../questions/tones-chip-wording.md).

## Noticed, not fixed

- **A plan's fold can collapse under a reader when its closing post arrives.** A span is open while
  it runs and folds once it is resolved and read, so a reader watching it resolve can see it shut.
  Old behaviour (the same rule as before the redesign), low harm because the closing sentence and
  status word stay visible; the fix would be to remember that the reader saw it open. Not done.
- **You see your own "alice is writing…" line** while you type. It is on `main` too (the "before"
  shots show it). The page's typing label doesn't leave the viewer out.
- **`tones-chip-wording.md` still says ChatPanel draws the chip "as a stopgap".** It is no longer a
  stopgap; I left that file's text alone because it isn't mine to edit.
- **`StyleGuide` page** has no entry for `FeedDivider` or `SpanHeader`: they live in `components/chat/`
  and are used only by the feed, so I didn't add them to the shared pieces. If the Public Record piece
  wants the same fold row for its plan lines, `SpanHeader` is the one to lift into `shared/`.
