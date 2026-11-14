# redesign-scenes-018be6

The two scene screens, rebuilt: the form the focus player fills in to set a scene (and the read-only mirror
everyone else watches), and the panel that sits on the Play pane while the scene is on. Wave 5 of the
visual redesign. The screens were never mocked up, so this applies the rulings as written; what they
don't settle is in the three question files below.

## What changed and why

**1. The prompt loses its box and its gold wash.** It was a bordered, gold-tinted panel with a gold stripe
down its left edge. It is now a small heading ("Prompt") and the prompt's words at step-text size, in
both the form and the scene's panel. Only controls and game pieces are boxed, and a prompt is neither.

**2. Everything else in them follows the same rule.** Where, When, Who else is here, Location and Present
are small secondary capitals with their content under them. The groups are kept apart by the step gap
(16px) and a label sits 8px above what it heads. The old `.custom-panel` (a bordered, tinted box around the
"Another location" field) and the rule above the End Scene button are gone. The only boxes left are the
shared pieces: the asset cards, the chips, the fields and the buttons.

**3. The controls come from the shared recipes** (`SceneSetupForm.svelte`):
- the six durations are `<Chip compact>` (they were hand-drawn pills). They wrap three to a line by
  themselves, so the `@container column (max-width: 300px)` query is gone (a Play column is never below
  344px now);
- the two text fields are 44px, 16px type, the raised surface and the neutral control border, with the
  bright gold focus ring and a faint placeholder. They also gained an `aria-label` (the place field had only
  a placeholder);
- "Begin Scene" and "End Scene" are the main button; "Take over" is the secondary button at 44px (it was a
  36px gold-washed button);
- the "nothing to pick" lines are plain secondary text, not italic.

**4. The scene's header** (`SceneDetailsPanel.svelte`) names the place beside the house scene mark in place of
the 📍 emoji, the time as plain words (a note is no longer italic), and the focus player as their shape and
their name in their colour (it was a bare coloured name).

**5. A watcher's form is no longer dimmed.** It used to grey its prompt, headings and chips with a
`.readonly` rule. It now has no rule of its own: every control is disabled and draws its own disabled look,
as the Play piece did for the plan panels. The help row stays operable. This makes the "Local help" section
of the style guide untrue ("where the screen greys out for watchers"), so I corrected that sentence.

**6. The page scrolls them, not the panels.** Both components used to carry `overflow-y: auto; min-height: 0`,
which in a flex column lets a panel shrink and scroll inside the scroller it already sits in. They now have
`flex-shrink: 0` like the plan panel, and keep only the space that holds their last control off the tab bar.

**7. Tests.** `SceneSetupForm.test.ts` (new, 22 tests) and `SceneDetailsPanel.test.ts` (grown from 1 to 19)
render the components through `svelte/server` and read their stylesheets; `tests/e2e/scenes.spec.ts` (new,
4 specs) drives the form and the panel as three players at 375px.

**Style guide:** a "Scenes" subsection before "Pages outside a game", and the corrected sentence in
"Local help". No tokens were added to `app.css`, and no shared component was created.

**Markers removed** (`tokens: not migrated`): `SceneSetupForm.svelte`, `SceneDetailsPanel.svelte`.

## How it was verified

Against this branch's own code (a private server on `:8163`, a private database, rebased on `main` after
#85):

- `make check-fast`: passes. 79 test files, 1334 tests, `svelte-check` clean, `go vet`, the linter, the Go unit
  tests, the git-guard tests and the tracker check. (`make check-fast` runs the linter with `--fix`; it
  changed nothing.)
- **e2e: the full suite, 208 specs, all pass.** One new spec file, `scenes.spec.ts`, has four tests:
  - *setup, as the focus player and a watcher* (alice and bob): the heading as each, the small headings and
    the prompt with no fill or border (read from the browser's computed styles), "Begin Scene" grey until a place and
    a time, every chip and field 44px tall, the six chips wrapping to two lines, no sideways scroll, the draft
    reaching the watcher live (a chip, a typed place, a picked holding, a typed time), every watcher control
    disabled, the form not faded, the help row still opening for the watcher, and Begin Scene landing both on
    the scene;
  - *the opening scene* has a note and no chips, and needs the note;
  - *details, as the focus player (alice), a player in the scene (bob) and a bystander (carol)*: the heading,
    the place, time and focus line, the mark and shape, no 📍, no box on the panel, one main button for
    alice only, "Take over" for the one unclaimed peer (44px) to bob and carol and none for alice, bob taking
    it over and everyone seeing "Played by bob", then End Scene clearing the panel;
  - *the follow-up scene after a plan*: Seek Answers played out over the API, then the plan's prompt on the form
    (as alice and as a watcher) and on the scene, in the same small heading.
- **Every new test bites.** 36 unit mutations, each run against its own test file and each turned red, then restored:
  the box and the wash back on the prompt (both files), a watcher's chip or field live, the readonly fade back (class
  and rule), chips full size, a watcher given the submit button, the submit button not the main one, a watcher no longer
  mirroring the draft (chip, peers, typed place), a field 36px or on the raised-twice fill, either panel scrolling itself again,
  chips on the opening scene, the help row hidden from a watcher, gold labels, an italic hint, a container query back, the old
  label span back, the pin back or the mark gone, an italic note, a chip and a note both printing, a 36px "Take over",
  "Take over" shown to the focus player or not the secondary button, "End Scene" not the main button, no shape or no colour on
  the focus line, a rule above End Scene, the prompt shown when empty, a destroyed peer listed. Two were not caught
  at first (one mutation was malformed, and one test's scene never held both a chip and a note), so I fixed the
  mutation and added the test. Three e2e batches (each rebuilds the server and runs `scenes.spec.ts`) turned it red as designed:
  a boxed prompt in the form, chips on the opening scene and the pin back (all four tests red); a live chip for a watcher and "Take
  over" offered to the focus player (setup and details red); full-size chips and a hidden prompt on the scene (setup and follow-up red).
- **Looked at, at 375px, before and after** (the pairs are in the PR): the setup form as the focus player and as a watcher, the opening scene's form,
  the scene as the focus player and as a player in it (a peer to take over), a scene with a typed place and note, and the follow-up
  scene after a plan (its form and its scene). Built from this branch and from the `main` it started on, each on its own
  server and the same database.
- **Not run against this code:** `make test-integration` (no Go changed); a real phone; Safari or Firefox; hover and focus
  states (described in CSS, not photographed); 344px and the two- and three-column widths; the scene's panel as a bystander in a
  screenshot (it is in the e2e spec, which reads the page as carol, but I did not photograph it); a plan-scene (Host Festivity,
  Propose Decree, Chronicle Histories, Clandestinely Liaise), which these two components never show; a table of four or five
  players.

## Behaviour changes to look at before merging

- **The prompt and the groups are no longer boxed or tinted.** At 375px the form is a column of small
  headings, words, cards and fields.
- **The time chips are `<Chip compact>`, left-aligned, three to a line** (they were centred hand-drawn pills).
  `chip.css` keeps compact for chips that navigate or filter, and the signed-off mockups' choice chips (Aid /
  Interfere) are full size. I chose compact so six fit in two lines at 343px (full size wraps to three raggedly);
  they keep the 44px height. If you would rather they were full size, drop `compact` in `SceneSetupForm.svelte`.
- **The help body's sub-head ("Need ideas?") changed** from muted capitals at about 11px to 14px plain text in the main
  colour, and the help paragraphs and list are 14px (the lede is 16px).
- **A typed place no longer lights up.** The old form gave the "Another location" panel a gold border and
  wash once it held text. Now the field shows its words and the cards deselect (question file below).
- **"Take over" is bigger** (44px, was 36px) and plain, not gold.
- **A watcher's form is at full brightness**, with each control in its disabled look. A picked chip or
  card they see is still bright gold with faint text, which is the shared recipe (question file below).
- **The scene's place line has the scene mark, not the pin.** The time note isn't italic.
- **The 📍 is gone.** Anything reading the pin in a screenshot or a test must read `.place-mark`.

## Open questions for the owner

- [scene-labels-and-fields](../questions/scene-labels-and-fields.md): which small heading the groups get
  (page-level secondary capitals, which I built, or the Play mockups' faint sentence case); and a typed
  place or time has no "chosen" marker.
- [scene-watcher-picks](../questions/scene-watcher-picks.md): a watcher sees the focus player's picks in
  bright gold (the shared chip and card's locked look), where the plan grid shows them neutral. The same
  question as the third point of [plan-grid-redesign](../questions/plan-grid-redesign.md); one answer settles
  both, and it needs a shared mirrored look on the chip and card.
- [scene-place-mark](../questions/scene-place-mark.md): the scene mark stands in for the place's pin.

## Noticed, not fixed

- **A "section label" recipe now exists four times** (the profile's `.group-heading`, `PaneName`,
  and the two `.label` rules here): 12px, capitals, 0.06em tracking, secondary. The cleanup piece could make it
  one shared class; I was told to create no shared component.
- **`layoutTokens.test.ts` still allows `@container (max-width: 300px)`** "for SceneSetupForm's squeezed
  column", and the style guide's "Layout widths" rules still list it. Nothing uses it now. (Both are
  outside this piece's files.)
- **`SceneDetailsPanel` repeats the location twice** when a holding is the place (the header line, then the
  holding's card under Location). That is the old behaviour and I left it.
- **The commented-out scene-roll TODO** at the foot of `SceneDetailsPanel.svelte` is untouched.
