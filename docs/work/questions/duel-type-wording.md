---
title: "Should the duel's type read 'wits' or 'trial of wits' throughout?"
checked: "RULINGS (Propose Duel: arms or wits is narrative only; Host Festivity: a challenger names the type, 2026-11-26) and the 12 plans rules (Propose Duel: the book's question about putting an opponent on trial), both read. Nothing in them says which phrase the game shows. The style guide has no wording rule for duel types, and a search of the repo found three phrasings already in use."
---

The duel's type is one of two values, stored as `arms` or `wits`. The app says it three ways:

- The chip that picks it reads "Wits / Trial" (`duel/DuelTermsFields.svelte`).
- The prepared log line and the festivity banner say "a duel of wits" (`handler/plan_propose_duel.go` `PreparedDescriptor`; `ChallengeBanner.svelte`).
- The duel's heading says "a trial of wits" (`ProposeDuelPanel.svelte`, the `against` snippet).

The book's rules text uses "wits" for the type and frames the duel as putting an opponent "on trial". "Wits" is the value the code stores and the word the rules use for the type; "trial" is the book's framing.

**Recommendation:** "a duel of wits" everywhere. Change the chip to "Wits", and the heading to match the log line. That keeps the type word the same in the stored value, the chip, the log and the heading.

**The owner's call:** one phrase for the type, or keep "trial" as flavour in the heading only?
