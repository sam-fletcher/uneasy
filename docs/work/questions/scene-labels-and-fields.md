---
title: "Scene screens: which small heading, and is a typed place or time marked?"
---

The scene screens were never mocked up, so two of their looks are my reading of the rules, not
something you have seen.

- **Which small heading do the groups in a scene get?** You ruled that the scene prompt becomes "a
  heading and text", and the step's own heading ("Your move — set the scene") already sits just above
  it in deep gold at 18px, so a second gold heading right under it would compete with it. That leaves
  two looks for the small headings over the prompt, Where, When, Who else is here, Location and Present:
  - **Page-level capitals (what I built):** 12px, capitals with a little tracking, secondary colour. It is
    the label the profile's table groups and the column names use, so the scene screens read like the
    pages around them.
  - **The Play mockups' label:** 12px, **sentence case, faint** ("For", "Against", "Leverage from your
    retinue" in the signed-off mockups; the roll's `.dice-label` follows them). It is what a group
    *inside a step* is called in the one place you have seen a step drawn, and it is quieter, so
    the step's own heading is the loudest thing on the screen.

  Both are one rule: `.label` in `SceneSetupForm.svelte` and in `SceneDetailsPanel.svelte` (drop the
  capitals and the tracking, take the faint colour). Deep gold is a third option, not one I would pick.
  Which do you want?
- **A typed place or time has no "this is the answer" marker.** A place can be a picked holding or
  typed words, and a time can be a chip or typed words, and choosing one clears the other. The picked
  card or chip wears the bright gold border. The old form gave the typed place a gold-washed panel for
  the same job; the wash is retired and a field inside a panel of its own doubles the border, so the field now just
  shows its words. The alternative is to give the field the selected border (2px bright gold) once it
  holds the answer; it would then sit beside the focus ring, which is the same bright gold. Which do
  you want?
