---
title: "Plans: a set-piece line for the moments a plan stages"
order: 66
after: next/plan-type-tiers.md
---

**Starts with a design round** the owner signs off: mockups, no code. It can share a round with
`plan-type-tiers`, and builds after it, since both edit the same panels and may add to the type rules. The name
"set-piece line" is a placeholder.

**What was lost.** Before the plan-panels-B PR, Clandestinely Liaise's delay reveal opened with its own small
scene. The owner counts it among the most beautiful parts of the app:

- the scene the preparer wrote, centred, in quotation marks;
- under it, the two meeting peers, stacked and centred, with space below before the die-face picker:

  ```
            alice's Lady Vey             player name, then the peer in italic deep gold
                 meets                   muted italic, on its own line
             bob's Ser Ott
  ```

The redesign flattened it into one left-aligned line at body size, with the peers in plain italic, and the
scene into a small secondary line (`ClandestinelyLiaisePanel.svelte`, the `delayReveal` branch; the old markup
is in that file's history as `.cl-meeting-peers`).

**The opportunity.** Consistency matters, but this screen shows a kind of line the visual language doesn't have
yet: a moment the game stages, set apart from the instructions around it. The design round decides:

- what it looks like: centring, quotation marks, italic, deep gold, its size, the space around it;
- when a line earns it, so it stays rare enough to mean something;
- where else it applies. Candidates to check, not yet looked at: Make War's declaration (who against whom),
  Propose Duel's pairing of the duellists, the decree being debated and the law once enacted, Make
  Introductions' newcomer arriving;
- which current rules it bends, and how far. The style guide keeps italic for asset names and leaves other
  quoted text plain, keeps deep gold for the game's own voice (headings, the main button), and never centres
  anything a player wrote in the chat. The old reveal centred the preparer's own words and set the peers in
  gold.

Once the look is signed off, it becomes one shared component with a `/dev/styleguide` entry and a style guide
section, and Liaise's delay reveal is its first user.

**Files:** a new shared component, `ClandestinelyLiaisePanel.svelte`, the other panels the round picks,
`docs/STYLE_GUIDE.md`, `/dev/styleguide`.
