# Redesign: the landing and sign-in pages, Help, and the app frame

## What changed and why

The screens outside a game move onto the redesign's tokens and shared pieces. None of
them was mocked up, so each follows the rulings as written, and anything they did not
settle is in `docs/work/questions/`.

- **Sections get no box.** The sign-in, reset-password and locked-out forms lost their
  card (fill, border, rounded corner). Only the controls inside them are boxed.
- **Log in / Sign up** are two chips; the one you're on wears the bright gold border.
- **Errors** (`ErrorText`) draw the danger mark beside the words. It keeps its classes,
  its `variant` and `extra`, and `role="alert"`, so the roughly fifty call sites keep
  their placement and only gain the mark.
- **Successes** the user has just caused (a request sent, a password changed, feedback
  recorded) are `Notice` success notices with `live` set.
- **Help** (`HelpContent`): topic tabs are `Chip`s; the footer's Send feedback is a
  secondary button, since a sheet with no step has no main button; the figures, the
  pace cards, the plan cells and the asset grid lose their boxes and keep their
  spacing, with thin rules between neighbours; the asset type reads as a status word,
  as it does in the Retinue; the MAR stamp is a red word; the example asset card is a
  game piece (piece border and corner).
- **Public Record diagram** is drawn at its real size (150px wide, the same as before).
  An SVG's font size counts in its drawing's units, so a scaled-down drawing could not
  use the text tokens. Drawing it 1:1 lets its labels use `--text-label` and
  `--text-small`.
- **HelpDisclosure** no longer wraps `ChecklistRow`. It draws the shared row itself
  (`tapRow.css` plus `RowMark`) with the help mark at its start, and its body below.
  `ChecklistRow` is untouched; the lobby's checklist still uses it.
- **Update banner** keeps its tinted strip, now a divider line instead of a warm rule.
  Its tint is still the sunken surface token (see the question file).
- **App frame** (`+layout.svelte`): buttons and text fields take the tokens; focus is the
  bright gold on inputs and links; the header is the table title bar's height with no
  fill. The 44px text-field floor is not in the frame. It is set on the sign-in, reset,
  locked-out and feedback text fields only, so game inputs keep their own height.
- **Landing** (`+page.svelte`): the title is the one `--text-display` user, and its small
  "the" is `calc(var(--text-display) * 0.5)`, which the guard accepts.
- **Shake-Up mark:** Help's record diagram draws the Shake-Up row with the house burst
  mark that `LogMark` draws for the Shake-Up family, in place of the ✶ character. The
  rulings give the Shake-Up that mark, because a mark has to come from something the
  app ships. The mark is placed with a `foreignObject`, so the drawing stays in one
  place. The question file that asked this is deleted.
- **Errors:** the Errors section of the style guide now describes the danger-notice
  form. `ErrorText` has a render test that pins its `role="alert"` and danger mark.
- **Style guide**: `HelpDisclosure` and `ErrorText` rows and the Local help paragraph
  describe the new frame; a "Pages outside a game" subsection is added.

## How it was verified

Against this worktree's own code (server on `:8192`, private database
`uneasy_claude_redesign_pages_and_help_1893c6`):

- `make check-fast`: exit 0. svelte-check reports 0 errors and 0 warnings; 1015 unit
  tests pass, including the scale guard (no marker left on any file I migrated, and no
  literal font size, gap or radius in them) and the tracker check.
- **Prove it bites:** I added a background to `HelpDisclosure`'s body. The new no-box
  test failed on it. I reverted the change and the test passed again.
- **e2e, full suite** against my own server: 181 of 184 passed in one run. The three
  that failed (`chronicle_histories`, `make_war` surrender, `spread_rumors` make
  pre-roll) hit 30-second timeouts while a screenshot run was using the same server.
  Run alone, all three pass. Covered by name: `back_dismiss` (Help and its feedback
  hand-off), `profile_tables`, `table_shell` (Help's button and the sheets),
  `prologue_claim` (a `HelpDisclosure` in the prologue), `styleguide` (the chips),
  `smoke` and `player_join` (the lobby, which mounts Help).
- **Dense game screens at 375px**, after the change: a seek-answers plan in its
  pre-roll step, and alice's retinue with a secrets editor open (a text area and two
  buttons). Both lay out as before. The retinue's marginalia field was not opened.
- **ErrorText test:** the new render test fails when `role="alert"` is changed to
  `status`, and passes once it is reverted.
- **Screens at 375px**, before and after, by Playwright at a phone viewport: landing
  (login and signup modes), the login error, reset-password (form and missing token),
  locked-out, Help (Goal and Assets, plus Record, Rankings, Plans and Dice in a tall
  frame so the whole body shows), the feedback form and the update banner.
- **Integration:** `make test-integration` against the private database. Go code is
  untouched. (Result is recorded in the PR.)

**Not run:** a clean full e2e run with nothing else on the server; a real phone (checked
in Playwright's mobile emulation only); the push-blocked steps (no headless browser can
block notifications, so that screen was not seen on screen); a real deploy of the update
banner (I simulated a newer build in the page).

## Behaviour changes to look at before merging

- The sign-in forms have no card any more. The landing page, reset-password and locked-out
  show their form directly on the page.
- Every error in the app gains the danger mark, including plan panels and the table page.
- Log in / Sign up is a pair of chips, not a filled tab.
- Help's tab selection is the gold border, not the old filled tab. The prose is 16px, up
  from about 15px.
- The landing kicker is 14px (it was about 13–16px). The subtitle is 16px.
- The sign-in, reset, locked-out and feedback text fields are at least 44px tall. Every
  text field gets the neutral control border and the bright gold focus ring, and
  the global button padding changed by a few pixels, so every button without its own
  padding moves slightly. The dense game screens were checked for that.
- The Shake-Up row in Help's record diagram has the house burst mark, not the ✶
  character.
- `HelpDisclosure` looks the same in its own row, but it is now drawn by this file, not
  `ChecklistRow`. It appears in the prologue and in scene setup.

## Open questions for the owner

Each has a file in `docs/work/questions/`:

- `push-blocked-rule.md`: should the notifications-blocked steps keep their warning rule?
- `update-banner-tint.md`: what tint should the update banner take once the sunken surface goes?
- `landing-kicker-size.md`: should the landing kicker be the label size or the small size?
- `notice-marks.md` (already open, not from this PR): whether an error gets a mark at all,
  and the octagon drawing. This PR gives every error the danger mark. The answer changes
  the errors here, and `ErrorText` is the one place to change them.

## Noticed, not fixed

- `make check-fast` prints `middleware/auth.go:94:6: unreachable func: AccountContext`
  from the deadcode step. It is not in this change, and the target still exits 0.
- Two retiring tokens are still in use. Help's record rails and dice outlines use
  `--color-border-strong`, and the update banner uses `--color-surface-sunken` (its
  question is open). The cleanup piece will need replacements for both.
- The cancelled die in Help's dice example still fades with opacity, as the live roll
  panel's cancelled die does. The rule against opacity is about text, so I left it.
