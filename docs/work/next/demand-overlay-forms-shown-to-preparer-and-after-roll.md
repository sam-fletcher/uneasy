---
title: "Make Demands overlay: forms and banner promise things that can't happen"
order: 84
---

`TargetPlanDemandOverlay.svelte` shows the demand's forms and banner without
asking whether the server would act on them:

- **Preparer who won the option herself.** There is no hand-off and the server
  never holds the roll for her, yet she reads "…so the roll waits on you" with
  Re-aim / Keep buttons (keep-or-change-target) or a leverage picker on her own
  plan.
- **After the dice land.** The leverage form is gated on the plan being
  `resolving`, not on the roll being open, so the winner still sees "Leverage none —
  let the roll fail" afterwards (the server answers 409). The retarget form also
  stays, and its copy says "You may still adjust it until the plan resolves", but
  `/demand-retarget` answers 409 once the roll has resolved. This is a copy and
  gating bug: hide both forms (and fix the copy) when the roll is closed.
- **keep_assets banner on plans that award nothing.** The banner says "Any assets
  this plan would have given alice go to bob instead" for Seek Answers and Chronicle
  Histories, which give their preparer no assets (the draft card says so). Skip the
  line for those plans, as `AWARDS_NO_ASSETS` already does for the draft.

Pinned as four `test.fixme`s in `frontend/tests/e2e/make_demands.spec.ts`; un-fix
them when this lands.
