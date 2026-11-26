# Swipe a bottom sheet down to dismiss it, and stop pull-to-refresh

Branch `claude/awesome-colden-98adb4`.

## What changed and why

The owner kept swiping sheets down to close them, and the browser's
pull-to-refresh took the gesture and reloaded the whole page. No
`overscroll-behavior` rule existed anywhere. Two parts, both in the one
component every sheet goes through (`RetinueSheet.svelte`):

1. **Pull-to-refresh is shut while a sheet is open.** `.sheet-body` has
   `overscroll-behavior: contain`, and while `open` is true
   `document.documentElement` gets `overscroll-behavior-y: none` (removed on
   close or destroy). The second half is needed because Chrome doesn't trap the
   overscroll of a body too short to scroll, so the gesture would still reach
   the page. It is a module-level *count* of open sheets, not a flag, because a
   hand-off (one sheet closing as another opens) can run the new sheet's setup
   before the old one's teardown.
2. **Swipe down to dismiss**, in the new `src/lib/sheetSwipe.ts` (a Svelte
   action on `.sheet`; separate from the component only so the thresholds can be
   unit tested).
   - Touch events, `touchmove` registered `{ passive: false }`.
   - Decided once per gesture, at the first move past 5px: claimed only if the
     finger goes mostly downward **and** `.sheet-body.scrollTop <= 0`. Otherwise
     the gesture is left to the browser for its whole length. So a body scrolled
     partway scrolls back up, and a scroll-up flick that reaches the top doesn't
     carry on into closing: the user lifts and swipes again.
   - "At the top" means every vertical scroller from the touch target up to the
     sheet is at 0 (`scrolledAmount`), not just `.sheet-body`: on a phone Help
     fixes its tab panel's height and scrolls *that* (`.help.panel .body`), so
     the sheet body stays at 0 there. Found in review; pinned by the Help case
     in the e2e spec, which fails with a body-only check.
   - Never starts on `input`, `textarea`, `select` or `[contenteditable]`.
   - Once claimed, `preventDefault` on each move, `translateY(dy)` on `.sheet`,
     and the scrim fades in step.
   - Release: past 25% of the sheet's height or faster than 0.5px/ms (measured
     over the last 100ms) closes. The sheet slides the rest of the way off, and
     `onClose` is called only afterwards, so `dismissOnBack` behaves as it does
     for the × button. Otherwise it springs back. If the parent doesn't close
     the sheet after `onClose`, the inline styles are cleared so it isn't
     stranded off-screen.
   - Enabled only when the sheet is a bottom sheet: phone layout, or hosted over
     a column. The unhosted centred dialog (≥744px) isn't draggable.
   - Reduced motion: the sheet still follows the finger; spring-back and exit
     jump straight to the end state.
   - A second finger mid-drag, or `touchcancel`, springs back.
   - No grab bar (see the question file `sheet-grab-bar.md`).

## How it was verified

Against this worktree's own build, a private server and database (both removed):

- `make check-fast`: passes (2360 Vitest tests, svelte-check clean).
- New `tests/e2e/sheet_swipe.spec.ts`, real touch through CDP
  `Input.dispatchTouchEvent`, 375px, `hasTouch` + `isMobile`, 4 tests passing:
  Help closes on a swipe down (and the page overscroll is set while open and
  cleared after, history entry spent); a short slow drag follows the finger then
  springs back; a drag starting in the feedback form's textarea doesn't move the
  sheet; Tones (a lobby table, so it overflows a 520px-high window) scrolled
  halfway scrolls back up on a downward swipe and doesn't close, then closes from
  the top; reduced motion; hosted (900px, table page) closes while the centred
  dialog (900px, profile page) doesn't move.
- New `src/lib/sheetSwipe.test.ts` pins the thresholds, the claim rule, and the
  source rules (`overscroll-behavior: contain` on `.sheet-body`, the root
  property set and removed, `{ passive: false }` touchmove, no pointer events,
  the text-field exemption).
- Proved to bite: removing the text-field exemption makes the feedback-form
  test fail (the sheet closes under the finger); making the claim ignore
  `scrollTop` fails the Tones test.
- `back_dismiss` and `retinue_pieces` e2e specs still pass.

**Not verified:** real iOS Safari and Android Chrome. CDP touch emulation drives
Chrome's own gesture pipeline but isn't a finger, and the actual pull-to-refresh
reload (Android Chrome) and iOS rubber-banding can't be seen from here. The owner
should try it on their phone: open Help, Tones and the feedback form; swipe each
down; check the page no longer reloads; scroll Tones and swipe up/down inside it.
Go integration tests weren't run: no Go code changed.

## Behaviour changes to look at before merging

- Every sheet now drags down on a phone. A downward drag that starts while the
  body is at its top moves the sheet (it used to bounce or reload the page).
- While any sheet is open, the page behind it can't be overscrolled or
  pulled to refresh (all widths, all browsers that support the property).
- The × button, scrim tap, ESC and Back are unchanged.

## Open questions for the owner

- Should a sheet show a grab bar? Filed as `docs/work/questions/sheet-grab-bar.md`.

## Noticed, not fixed

- A sheet still has no exit animation for ×, scrim tap, ESC or Back: it vanishes,
  while a swiped sheet slides out. Harmless, slightly inconsistent.
