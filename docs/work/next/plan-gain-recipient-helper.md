---
title: "Plans: one helper for who receives a plan's gains, and a stand-in test fixture"
order: 86
---

Five PRs in November (#107, #111, #114, #115, #118) wrote copy that forgot a Make Demands winner:
a perform-steps stand-in addressed as "you" where the preparer was meant, or a peer "going to the
preparer" when a keep-assets winner receives it. #118 repeated a flaw #114 had just fixed a few
lines away, because each panel works the recipient out for itself
(`ExchangeCourtiersPanel.svelte`'s `peerGoesTo`, `demand/TargetPlanDemandOverlay.svelte`'s
`winners.keep_assets`, and others).

**Build:**
- **One client helper** (in `plans/shared.ts` or beside `demandWinnersFromPlan`) that answers who
  receives what a plan would give its preparer, and who makes the preparer's choices, given the
  plan and any demand against it. Mirror the server's `AssetRecipientForPlan` and the
  perform-steps routing, and say so in a comment.
- **Use it everywhere a panel names a recipient or a decider.** Grep for `keep_assets`,
  `demandWinnersFromPlan` and `preparer_id` in copy. Change no wording beyond what the helper
  corrects.
- **A component-test fixture** that builds a plan with a perform-steps stand-in and a keep-assets
  winner (two different players), so a panel test can render both cases in one line. Add a test
  per plan that names a recipient or a decider.
- **Then tighten the work-session house rule** "Who acts and who receives" to name the helper and
  the fixture.

**Model:** Sonnet. It's code, with some judgement about which lines name a recipient.
