---
title: "Make War's words for paying a cost, and for what a surrender gives up"
checked: "Owner, 2026-11-26: the cost of battle shouldn't be framed as owing (it is paid from the payer's own assets, once per opponent; nobody is owed it), and surrender might take a better word (ceding, reparations, spoils of war). DECISIONS 'Play tab' names the war and its sides but not these words."
---

While a cost of battle is due, the war's panel lists every payer and the opponent they pay it
against, in the order they pay. Today the list reads "Owed this row, in order" over lines like
"▲ Wren against ◆ otto", a rejected peace proposal logs "the cost of battle is still owed", and a
surrender's takers see "Surrender claims" and "claim one of your assets".

The book's verbs are "pay the cost of battle" and, on a surrender, the opponents each "take one
asset of their choice". Proposed:

- The list's label: "To pay this row, in order". The lines keep "against": the cost is paid once
  for each opponent, and "against" names which one without making them a creditor.
- The rejection log line: "the cost of battle still has to be paid".
- Surrender: the takers' section is "Spoils of war", and its verb is the book's "take" (a take is
  blue, something gained). The surrendering player's line says each opponent takes one asset of
  their choice.

Strings: `plans/war/CostOfBattle.svelte`, `plans/war/BattleCostForm.svelte`,
`plans/war/SurrenderClaims.svelte`, and the rejection line in `handler/plan_make_war_peace.go`;
`make_war.spec.ts` reads some of them.
