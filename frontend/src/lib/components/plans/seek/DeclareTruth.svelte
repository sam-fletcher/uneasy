<!-- Seek/DeclareTruth.svelte
  Sub-step: declare something true about the world. Only the choice actor gets the
  button; everyone else sees the box greyed out.
-->
<script lang="ts">
	import { declareTruth, type Plan } from '$lib/api';
	import EndsNote from '../shared/EndsNote.svelte';
	import type { PlanContext } from '../types';
	import { LAST_STEP_TEXT as lastText, type SeekScene } from './shared';
	import type { SeekDrafts } from './drafts.svelte';
	import { TEXT_LIMITS } from '$lib/textLimits';

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

	let dtBusy = $state(false);
	async function submitDeclareTruth(p: Plan) {
		if (dtBusy || !drafts.dtText.trim()) return;
		dtBusy = true; error = '';
		try {
			await declareTruth(p.id, drafts.dtText.trim());
			drafts.dtText = '';
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not declare truth.';
		} finally { dtBusy = false; }
	}
</script>

<div class="plan-form">
	<p class="choices-header">
		Declare something true ({scene.dtRemaining} remaining)
	</p>
	<p class="choices-note">
		It can't contradict any truth already known to the table.
	</p>
	<label class="form-label">
		The truth you declare:
		<textarea rows={2} bind:value={drafts.dtText} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
			placeholder="Declare something true about the world…"></textarea>
	</label>
	{#if canSubflow}
		<button class="action-btn primary"
			onclick={() => submitDeclareTruth(plan)}
			disabled={dtBusy || !drafts.dtText.trim()}>
			{dtBusy ? '…' : 'Declare truth'}
		</button>
		<EndsNote show={scene.totalOwed === 1} text={lastText} />
	{/if}
</div>
