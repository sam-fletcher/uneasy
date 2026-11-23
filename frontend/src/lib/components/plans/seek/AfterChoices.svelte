<!-- Seek/AfterChoices.svelte
  Resolve, once the choices are committed: what was picked and what each step did, then
  a sub-step for each pick still owed (break a resource, reveal secrets, declare a truth,
  ask a question), the mar penalty, and the preparer's Complete.

  Only the choice actor (the preparer, or a perform_steps winner in their place) works
  the committed make/mar list; everyone else sees the same area greyed out and read-only.
  The pickers and buttons are blocked by the disabled fieldset, and the cards take
  readOnly (they're role=checkbox, not form controls). The committed state sits OUTSIDE
  the fieldset, at full opacity for every viewer.
-->
<script lang="ts">
	import { completePlan, type Plan } from '$lib/api';
	import ChoicesApplied from '../ChoicesApplied.svelte';
	import type { PlanContext } from '../types';
	import { OPTIONS, type SeekScene } from './shared';
	import type { SeekDrafts } from './drafts.svelte';
	import ResolvedSoFar from './ResolvedSoFar.svelte';
	import BreakResource from './BreakResource.svelte';
	import RevealSecret from './RevealSecret.svelte';
	import DeclareTruth from './DeclareTruth.svelte';
	import AskQuestion from './AskQuestion.svelte';
	import MarPenalty from './MarPenalty.svelte';

	let { ctx, plan, scene, drafts, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		scene: SeekScene;
		drafts: SeekDrafts;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const onPlansChanged = $derived(ctx.onPlansChanged);

	const canSubflow = $derived(scene.amChoiceActor);

	// ── Complete ────────────────────────────────────────────────────────────
	let resBusy = $state(false);
	async function onComplete(p: Plan) {
		if (resBusy) return;
		resBusy = true; error = '';
		try {
			await completePlan(p.id);
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not complete plan.';
		} finally { resBusy = false; }
	}
</script>

<div class="complete-section">
	<!-- Committed state (Tier-1, ADR-006): the picked plan and the
	     specifics of every step done so far, shown at full opacity to
	     EVERY viewer so the made choices persist after a picker commits.
	     Sits outside the greyed picker fieldset, like Propose Decree's
	     live law text. -->
	<ChoicesApplied choices={scene.existingChoices} options={OPTIONS} />
	<ResolvedSoFar {ctx} {scene} />

	<fieldset class="resolve-mirror-wrap" class:resolve-mirror={!canSubflow} disabled={!canSubflow}>

		{#if scene.brRemaining > 0}
			<BreakResource {ctx} {plan} {scene} {drafts} bind:error />
		{/if}

		{#if scene.rsRemaining > 0}
			<RevealSecret {ctx} {plan} {scene} {drafts} bind:error />
		{/if}

		{#if scene.dtRemaining > 0}
			<DeclareTruth {ctx} {plan} {scene} {drafts} bind:error />
		{/if}

		{#if scene.aqRemaining > 0}
			<AskQuestion {ctx} {plan} {scene} {drafts} bind:error />
		{/if}

		{#if scene.marRemaining > 0}
			<MarPenalty {ctx} {plan} {scene} {drafts} bind:error />
		{/if}

		<!-- Completion is always the preparer's (the server's CompletePlan
		     is preparer-only), even when a perform_steps winner drove the steps. -->
		{#if scene.subflowsDone && scene.isPreparer}
			<p class="complete-note">
				Bring the scene to a close, then complete the plan.
			</p>
			<button class="action-btn primary"
				onclick={() => onComplete(plan)} disabled={resBusy}>
				{resBusy ? '…' : 'Complete plan'}
			</button>
		{/if}
	</fieldset>
</div>
