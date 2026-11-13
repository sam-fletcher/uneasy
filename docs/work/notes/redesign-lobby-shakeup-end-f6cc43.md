# redesign-lobby-shakeup-end-f6cc43

The lobby, the Shake-Up, the ending vote and the ended screen, moved onto the new look. Wave 4 of the
visual redesign. None of these four screens was mocked up, so each was built from the shared pieces and the
written rulings, and every call the rulings don't make is a file in `docs/work/questions/`.

Behaviour is unchanged: the same routes, the same events, the same rules. Only how the screens look, and a
few words of markup, moved.

## What changed and why

**Four files left `tokens: not migrated`** and now sit on the type, gap and radius scales:
`phases/LobbyView`, `phases/ShakeUpView`, `phases/EndedView` and `EndgameVotePanel`.

| screen | before | after |
|---|---|---|
| **Lobby** | the verdict in a bordered card (a warm border for the host), a pill-shaped invite chair with a filled code badge and a hollow dot, the notifications button in main gold | the verdict is a heading, its words and the one main button, standing on the page; the invite chair is a dashed 44px control with the code as plain mono text and "copy" turning to "Copied!" in the success colour; "Enable notifications" is a secondary button, so the screen keeps one main button; the bell is `BellGlyph` (the profile's), not a second drawing |
| **Shake-Up: header** | three bordered boxes joined by arrows, the current one on the gold wash; "About the Shake-Up" a hand-built accordion in a box | the acts are a track (the act in play deep gold with a gold line and its step in words; finished acts faint); "About the Shake-Up" is a `ChecklistRow`, which keeps the "open only in the first act" seed |
| **Shake-Up: token pools** | one pill per player, in the player's colour, the active one on the wash | `TableRoster`: shape, name, a clock on whoever the game waits on (the same set as the player row), the count at the right |
| **Shake-Up: announce** | a boxed form; options, marginalia and titles as pills, picked = the gold wash | no box; options and titles are `Chip`s, the pick the bright gold border with no fill; the typed ♛ is `CrownGlyph`; a marginalia to tear is a small piece with the picked state (border and tick); labels are `FormField`; the "no marginalia" line is a `Notice`; the input takes the control recipe |
| **Shake-Up: open spend** | a sunken boxed panel; gold paragraphs for a raised cost, a forced abandon and the cost floor; the floor button faded with `opacity` | no box; the raised cost is a warning `Notice`, a forced abandon a danger one, the floor's reason a live warning one; the −1 button greys through `aria-disabled` alone; the spender's name is no longer bold |
| **Ending vote** | a violet-framed card holding two card-buttons, each with a bold count and a violet "Your vote" | no box; each ending is a `Chip` with its count beside it ("1 vote"), the picked one bright gold, and "Your vote" a plain small-caps word; what each ending does sits under its chip as text, tied by `aria-describedby`; the tally, stragglers and tie-break are their own block |
| **Ended** | "Game Over", a line, then three columns of "2. alice" in muted text | stacked lists, one per track: rank, the player's shape, their name in their colour, a rule between rows |

Files outside my four that I touched: `docs/STYLE_GUIDE.md` (one new subsection, "The lobby, the Shake-Up and
the end of the game"), and tests (below). Nothing else.

**Why not the `Chip` for a marginalia to tear?** A note can run to 160 characters. In a fully rounded chip that
is a four-line stadium whose ends clip the words, so the note is a game piece picked with the piece's own
selected state, the way the retinue picks one.

**The Shake-Up view had no side padding on live `main`.** After the shell piece took the page's gutter away, the
view sat flush against the screen edge (the "before" pictures show it). It now carries its own 12px, like the
lobby and the ended screen. The vote panel does too.

## How it was verified

All against this worktree's own build: a private server on `:8148` and private databases (never `uneasy_test`).

- `make check-fast` passes: 1039 unit tests, `svelte-check` with 0 errors (one warning, the `.type-note` in
  `HelpContent.svelte` that was there before), `go vet`, the Go unit tests, the git-guard hook tests and the
  tracker check. The token guard in that suite is what proves the four files hold no leftover literals or markers.
- `make test-integration` passes on a second private database. No Go changed, so it is a sanity run.
- **The full e2e suite passes: 189 tests**, the 184 that were there plus 5 new ones.
- **New unit tests** (`lobbyShakeupEnd.test.ts`, 25 cases): the standings' order and the dummy slot left out; a
  shape per row; the two vote chips and their `aria-describedby`; the act track's `aria-current`; the intro
  opening only in the first act; the invite chair on a seat-free table and not on a full one; the Start button
  disabled with one player; and, read from the stylesheets, that none of the four files uses a retired token or
  `opacity` or draws a group with an outline or fill.
- **They bite.** I broke the source nine ways (a dummy listed, no sort, no shape, a chip not tied to its words,
  the act unmarked, the intro always open, a chair on a full table, a boxed verdict, a filled picked note) and
  each failed its test. The tenth, a vote chip never selected, passes by design: server rendering has no votes,
  so the picked state is only reachable in a browser, and the e2e spec covers it.
- **New e2e** (`ending_vote`, `lobby_invite`, `shake_up`; helpers in `helpers/endgame.ts`): they play the screens
  as the players would, at 375px. The vote is opened through the real gate. `shake_up` plays a whole Shake-Up out to
  reach the ended screen, which the seed endpoint can't. They read the painted result: the pick's border is
  `--color-you` and its fill is its neighbour's; the step has no border or fill; the −1 button is greyed and not
  faded; the raised-cost notice is the warning tone and has no tint; `Pay` is the one main button; the chair is
  dashed with the control corner. All five **fail against the unmodified build** (the old vote panel had a
  border, no `aria-pressed` chips, and so on).
- **Looked at** at 375px (34 states, before and after, in the PR): the lobby as host, host alone, guest, a full
  table and with the invite copied; the Shake-Up rolling as the roller and a watcher, announcing (an option, a
  target, a break with a marginalia picked, a claim-a-title in the Power act), waiting, no tokens, and an open
  spend as the spender (waiting, raised, pay or abandon, forced abandon, commit), a reactor (before and after
  reacting, at the cost floor) and a bystander; the vote before and after voting, as the facilitator and a
  guest; and the ended screen. Also at 344px (nothing of mine overflows; the player row scrolls sideways by
  design) and at 1280px.

**Not run against this code:** a real phone, Safari or Firefox (Chromium only); hover and focus states
(described in the recipes, only the chip's are covered by existing specs); a screen reader; five-player
Shake-Up pictures beyond the lobby; a Shake-Up claim-a-title run all the way through to its marginalia.

## Behaviour changes to look at before merging

- **Wording that moved.** "Your vote" is now a small-capitals word beside the chip; a count reads "1 vote" or "0
  votes" (it was a bare numeral). The token pools read "You 2", not "carol: 2". Nothing else in the copy changed.
- **The token pools are taller.** One 44px line per player, in place of a wrapped row of pills: about 130px at
  three players and 220px at five, against about 50px. They are `TableRoster`, which keeps every list of seats
  one height.
- **The Shake-Up's own words are still on the page twice** ("Shake-Up" under the title bar's "Shake-Up ·
  Esteem", "Game Over" under "Game over", "Your tokens" above the pools). Left alone; see the question.
- **"Enable notifications" is secondary** (it was main gold), so the lobby's one main button is Start Prologue.
- **Announce options and titles wrap as full-size chips**, mostly one to a line at 375px, where the old pills
  were smaller and sat two to a line.
- **The act track has no arrows.** Typed arrows fell back to a different font on each platform, and the house
  chevron means "opens elsewhere".

## Open questions for the owner

- [step-ask-heading-lobby-shakeup-vote](../questions/step-ask-heading-lobby-shakeup-vote.md): should these
  screens take the "Your move — …" heading the Play piece is adding?
- [shake-up-repeated-words](../questions/shake-up-repeated-words.md): drop the headings and the token line that
  repeat the title bar and the pools?
- [shake-up-notice-tones](../questions/shake-up-notice-tones.md): warning for a raised cost, danger for a forced
  abandon.
- [ended-standings-layout](../questions/ended-standings-layout.md): stacked lists, three columns or one table; and
  whether the ranks should still read "2.", "3.", "4." when a dummy holds first.
- [shake-up-act-track](../questions/shake-up-act-track.md): how the three acts look without boxes or arrows.

## Noticed, not fixed

- **Pieces of these screens are other sessions' files and still look as they did**: `Buffet` ("What can
  happen?"), `DiceRollPanel` (the roll step, with its 🎲 emoji and boxes), `CardPicker`'s and `FormField`'s
  labels (deep gold at a literal size, from `planPanel.css`), and the Waiting On bar above every one of these
  screens. The "after" pictures show them unrestyled.
- **`MainCharacterChoicePanel`**, the sibling takeover panel to the ending vote, still has the framed-card look.
  It isn't in any `redesign-*` item that I could find (Play lists `MainEventView` and the plan frame, not this
  panel). The coordinator may want to assign it.
- **`dev/advance-row advance_to` never opens the ending vote** (it skips the gates), and **`dev/force-roll`
  refuses Shake-Up rolls**. The routes that work are the focus player's `POST …/pass-focus` at row 7, and
  `POST /api/rolls/{id}/ready` as the current roller. `docs/DEVELOPMENT.md`'s dev-tools table could say so.
- **A raise clears the raiser's own pass**, so the raiser must pass again before the spender can commit. That is
  how it was built, and the rule is deliberate (an adjustment reopens the reaction window for everyone), but it
  surprised me in a script, and a player who raises may not expect to be asked again. The spend panel says
  "waiting on <raiser> to react".
- `svelte-check` still warns about `.type-note` in `HelpContent.svelte` (there before this PR).
