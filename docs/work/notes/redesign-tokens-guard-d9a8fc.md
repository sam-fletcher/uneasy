# Redesign: tokens and the guard test

## What changed and why
The first piece of the visual redesign: the tokens, and a guard that holds each migrated
file to them. Nothing is restyled except through the tokens.

- **`app.css` tokens.** Text sizes (`--text-label/small/body/heading/title`, plus
  `--text-display` for the landing hero, the one exception), gaps (`--space-1`…`-6`),
  radii by kind (`--radius-piece/control/chip/sheet/dot`), the new text, border and
  gold semantics (`--color-border-piece/-control/-quiet`, `--color-you`), and the new
  primitives behind them. Names say the job, not the pixels.
- **The whole app moves palette at once**, with no component edits: `--color-surface`
  and `--color-surface-2` take the new raised and raised-twice greys; the text tokens
  collapse onto three parchment shades; the five player colours are replaced (in
  `playerColor.ts` and the `--player-*` reference block, which `designTokens.test.ts`
  keeps in agreement).
- **Base rules**: `text-wrap: balance` on headings, labels, legends and buttons,
  `text-wrap: pretty` on paragraphs, once, in `app.css`.
- **The guard**, `frontend/src/lib/scaleTokens.test.ts`: fails any `font-size`, `font`,
  `padding*`, `margin*`, `gap`, `row-gap`, `column-gap` or `border-*radius*` that isn't a
  token or 0, across `<style>` blocks and style attributes (`style="…"`, `style={…}`,
  `style:prop`), including string and template literals inside their expressions
  (`style:padding={'7px'}`, `` style={`padding:7px`} ``), and `!important` doesn't hide a
  literal.
  80 files that still have literals (69 `.svelte`, 11 `.css`, 1,354 declarations) carry a
  first-line `tokens: not migrated` marker and are skipped. The guard fails a marker on a
  file with nothing left to migrate, and a marker anywhere but line 1.
- **`docs/STYLE_GUIDE.md`**: the colour and token sections are rewritten for the new
  sets, with a retiring list, what the guard accepts, and how the marker works. The old
  player-row "state channels" section is kept, with a note that its rebuild piece
  rewrites it.

### Changes after the coordinator's review
- Dice carry no shape yet and keep their owner's colour on the edge, so the guide no
  longer lists dice among the places a shape is drawn, and `--color-border-piece`
  (comment and guide) no longer names dice, with a line saying they are the exception.
  `--radius-piece` still names dice: the ruling gives them the piece corner.
- The guard no longer lets a literal through inside a style expression, and its
  `!important` strip is global; both have rejecting cases, and I checked that removing the
  fix fails the new ones.
- The guide, `app.css`, `playerColor.ts` and the commit messages were reworded where they
  sat close to the private rulings.
- The values of the scales are deliberately not pinned by the test (names, order, units
  and the 12px floor are), so they can be tuned in one place; a comment says so.
  `playerColor.ts` now notes Sapphire is 4.49:1 on a raised surface.

### Faint text lightened (owner's ruling)
The first draft's faint shade (`#8d8a7f`) measured 5.03 / 4.49 / 3.93 on the page, raised
and raised twice, so it fell under WCAG's 4.5:1 on both raised surfaces. I raised that as
a question; the owner chose to **lighten it**. The step is now `#999689`, not the owner's
rounder `#9c998e`: that one is only ΔE 5.5 from `--neutral-300` (`#999999`), under the
palette's ΔE 6 near-duplicate bar, and `#999689` clears the bar against every primitive
(nearest are `--neutral-300` at 6.5, `--parchment-300` at 7.1, `--neutral-400` at 7.6).
It measures 5.87 / 5.23 / 4.58, so faint text may now sit on any ground, and the guide's
"put faint on the page" limit is gone. The step keeps its name (`--parchment-400` still
sits between 300 and 500 by lightness). `--color-text-faint-warm`, a retiring alias,
follows on its own. The question file is deleted; this is the record.

### Decisions I made (flag any you'd rule differently)
- **`--radius-sm/md/lg`: kept at their old values, retired, and not a token to the guard.**
  Aliasing them to the new radii would have changed shapes in files that aren't being
  restyled (`sm` is 6px; no new radius is 6), and deleting them would have broken 11
  declarations silently (an undefined var is a 0 radius). Leaving them is the only
  option that changes nothing. The guard rejects them in migrated files, so each piece
  picks a radius by the kind of thing.
- **The four old text tokens became three, so `--color-text-muted` (154 uses) now means
  secondary, not faint.** The old muted grey was 6.1:1 and faint is 5.9:1, so by
  luminance faint is nearer; by name and role ("secondary labels"), secondary is.
  Secondary keeps the app's contrast up. One line in `app.css` flips it. I also mapped
  the two warm leftovers (`-tertiary-warm` → secondary, `-faint-warm` → faint, which
  fixes the 3.2:1 "+"); the brief named only four.
- **`--text-display`** holds the landing hero's current `clamp(2.8rem, 14vw, 4.25rem)`.
  Without a token the hero could never pass the guard. The hero's kicker and `h1 .the`
  are the landing piece's to fold into the scale.
- **`--color-border-quiet`** (`#2a2a2a`, the existing `neutral-800`) is for the record's
  row rules, which the mockups use; it needed a name.
- **Guard details**: `calc()`/`min()`/`max()`/`clamp()` of tokens, `0`, `auto`, `normal`,
  `inherit` and `env(safe-area-inset-*)` pass; `em`, `%`, `vw` and keyword sizes don't.
  A local custom property (`var(--chat-strip-height)`) passes, but its fallback is judged,
  and one *named* for a gap, gutter, padding, margin, radius or font size is held to the
  same rule (else `--x: 0.7rem` would be a way round). A token that doesn't exist
  (`--space-7`) fails, since it would silently render as nothing.
- **Seats are assigned in join order, creator first** (checked in `handler/tables.go`:
  create sets seat 1, join sets `count + 1`; no leave or kick route exists, so no gaps).

## How it was verified
Against this worktree's code:
- `make check-fast`: see the PR body for the result (run on the rebased branch).
- The guard bites both ways on real files (not just the unit cases): adding
  `padding: 7px; font-size: 13px; …` to the unmarked `BellGlyph.svelte` fails the main
  guard test, and putting a marker on the clean `ErrorText.svelte` fails the stale-marker
  test. Both restored; output in the PR.
- 81 tests in `scaleTokens.test.ts`, most of them cases of what the guard accepts and
  rejects, including calc, negative margins, inherit, auto, inline styles and comments.
- Seed, build and 375px screenshots on a private server (`:8192`) against `main`'s dev
  stack (`:8080`) as the baseline: a Make Demands table and the profile page.
- Contrast ratios computed for every text shade, gold, player colour and border on all
  three surfaces (in the PR body).

Not run: `make test-integration` and e2e. Nothing in Go or in behaviour changed; the
frontend build serves unchanged markup. The profile screenshots aren't a like-for-like
pair: the dev database had a second table and a different push-permission state than
my private one.

## Behaviour changes to look at before merging
- **The whole app changes palette on merge**, and `main` deploys itself. Seat colours
  change (every player's name, dots and dice edges), the surfaces step slightly, and
  secondary text brightens (`--color-text-muted` 6.1:1 → 9.1:1). The mixed look is the
  accepted cost.
- 1-pixel-ish differences in boxes (`--color-surface` 252525 → 242424, `-surface-2`
  2a2a2a → 2e2e2e) are visible only side by side.

## Open questions for the owner
- One remains, with the coordinator: Sapphire (seat 4) is 5.04:1 on the page but 4.49 on
  a raised surface and 3.93 on raised twice. It is a name colour and rarely sits on raised
  twice. `playerColor.ts` records the 4.49.

## Noticed, not fixed
- `svelte-check` warns about an unused `.type-note` selector in `HelpContent.svelte`;
  it was there before this piece.
- `app.html`'s `theme-color` and the manifest hold the deep gold as a literal. They
  can't use a CSS var; they only need touching if the brand gold moves.
- `layoutTokens.test.ts` and `docs/STYLE_GUIDE.md` still describe the 790/1070 docks
  and the rail; the layout piece updates them.
