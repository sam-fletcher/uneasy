---
title: "Two small UI changes the owner approved: one checkbox size, and the scene's sub-headings"
order: 76
---

Approved on 2026-11-26.

- **Every checkbox is 24px.** The marginalia boxes inside an open asset card (the "Marginalium to
  tear" picker on Make War's cost of battle, a break in Exchange Courtiers) are still 20px with a
  14px tick, set in `AssetCardSelectable.svelte` (`.m-tap`). Use the shared 24px box from
  `shared/checkbox.css`; the rows are already 44px.
- **A scene's groups take the step sub-heading.** "Where", "When" and "Who else is here" in
  `SceneSetupForm.svelte`, and the group labels in `SceneDetailsPanel.svelte`, are 12px secondary
  capitals today. They take the step sub-heading from the "Play steps" section of
  `docs/STYLE_GUIDE.md` (deep gold, 12px capitals, letter-spaced). A field's own prompt, where there
  is one, is the 14px secondary label.

- **Comments that cite deleted question files.** Three questions were answered on 2026-11-26 and
  their files deleted; reword each comment to state the answer: `lib/recordLines.ts` (and its
  test) keep "Resolving" on a war or liaison while it is played; `ChatPanel.svelte`'s Tones chip
  keeps "Tones · 0 set", the ruled wording.

**Done when:** 375px before/after screenshots of an asset card's marginalia picker, the scene setup
form and the scene details panel.
