---
title: "What should the scene panel's \"Focus: <name>\" line say, or should it go?"
---

Every scene's details end with a line "Focus: <shape> <name>". It has been there since the first scenes
screen (May 2026) and survived the redesign unchanged, so it may be a leftover rather than a choice. "Focus"
is the rulebook's word for the player whose turn it is, but no other screen uses it: the Retinue says "Sets
next scene", the open step says "set the scene", and Help doesn't mention it. A player who hasn't read the
book sees a label they can't place.

The line shows `scene.focus_player_id`, and that means two different things. For a scene a player set on
their turn, it is the player who set it, and "Take over" (offered to everyone else) refers to them. For a
plan's scene, the server stamps the plan's preparer (`plan_scenes.go`): nobody set that scene by hand; it
opened by itself when the plan began resolving. A label that says "Set by" would be wrong there.

Options:

- **Keep "Focus:"** as the book's word. Cheapest; stays jargon.
- **A label per kind of scene:** "Set by <name>" for a turn scene, "<name>'s plan" for a plan's scene. Says
  what each is, and costs a branch in `SceneDetailsPanel.svelte` (the panel already knows which kind it has).
- **Drop the line** where the header already names the preparer. A turn scene read later would no longer say
  who ran it.

**Recommendation: the label per kind of scene.** It keeps what a late reader needs and uses the app's own
verbs. The cost is that it has to be right for both kinds, so it should wait for the owner's reading of what
"focus" ought to mean on a plan's scene.

Which does the owner want?
