---
title: "Make War: the cost of battle is paid, not owed, and a surrender's opponents take spoils of war"
order: 82
---

Ruled by the owner on 2026-11-26. The cost of battle is paid from the payer's own assets, once for
each opponent, and nobody is owed it. On a surrender, the book says each opponent "takes" one asset
of their choice. The app says "owed" and "claim" today.

Change only the words a player reads. Identifiers, CSS classes and API names (`open_claims`,
`.owed-line`, `takeSurrenderAsset`) may keep their names.

- `plans/war/CostOfBattle.svelte`: the label "Owed this row, in order" becomes **"To pay this row,
  in order"**. Each line keeps "against" ("▲ Wren against ◆ otto"): it names the opponent a payment
  is for, without making them a creditor. Its `EndsNote` says "surrender claim"; reword it as below.
- `handler/plan_make_war_peace.go`: the rejection line "…the cost of battle is still owed." becomes
  **"…the cost of battle still has to be paid."**
- `plans/war/SurrenderClaims.svelte`: the section heading "Surrender claims" becomes **"Spoils of
  war"**. The button "Claim" becomes **"Take"**: the picker's label above it already names whose
  assets they are. The empty message becomes "No assets left to take." The error becomes "Could not
  take the asset." The button stays the house main button: a choice wears no meaning colour, and the
  take's log line already carries the take mark.
- `plans/war/BattleCostForm.svelte`: the surrender toggle's description becomes **"After this
  payment you surrender: each opponent who hasn't surrendered takes one asset of their choice from
  you."**
- Both `EndsNote`s ("…every battle cost and surrender claim…") become **"Play goes on once every
  battle cost is paid and every surrender's spoils are taken."**
- `phases/MainEventView.svelte`, around line 486: "…blocked on a cost of battle or a surrender
  claim." becomes **"…blocked on a cost of battle, or on spoils of war still to be taken."**
- `make_war.spec.ts` reads "Surrender claims" in four places; update it, and any unit test that reads
  these strings.

**Done when:** `git grep -n -i "owed this row\|surrender claims\|still owed"` finds nothing a player
reads; the Make War e2e passes; a 375px screenshot of the cost list and of the spoils section.
