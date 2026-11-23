<!-- Seek/MarPenalty.svelte
  Sub-step: the mar penalty — describe a flaw in some of the preparer's OWN resource
  assets. Listed last so the make-list tasks are settled before this consequence. It
  blocks completion until satisfied; progress is tracked by the server
  (mar_self_flaws_applied), so a refetch after each break is all it needs.
-->
<script lang="ts">
	import { breakResource, type Plan } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import EndsNote from '../shared/EndsNote.svelte';
	import Notice from '$lib/components/shared/Notice.svelte';
	import { destructionWarning } from '$lib/assetRisk';
	import { playerName } from '../shared';
	import type { PlanContext } from '../types';
	import { forfeitStep, LAST_STEP_TEXT as lastText, type SeekScene } from './shared';
	import type { SeekDrafts } from './drafts.svelte';

	let { ctx, plan, scene, drafts, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		scene: SeekScene;
		drafts: SeekDrafts;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	const canSubflow = $derived(scene.amChoiceActor);

	let maBusy = $state(false);
	// Destruction warning for the currently-selected break target.
	const maWarn = $derived(destructionWarning(assets.find(a => a.id === drafts.maAssetID)));

	// Penalty self-flaw: like breaking a make-list resource but progress is tracked by the
	// server (mar_self_flaws_applied), so we just refetch after each break.
	async function submitPenaltyFlaw(p: Plan) {
		if (maBusy || drafts.maAssetID == null || drafts.maMargID == null) return;
		maBusy = true; error = '';
		try {
			await breakResource(p.id, drafts.maAssetID, drafts.maMargID);
			drafts.maAssetID = null; drafts.maMargID = null;
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not flaw your resource.';
		} finally { maBusy = false; }
	}
</script>

<!-- The mar penalty (flaw your own resources) is listed last so the
     make-list tasks are settled before this consequence. -->
<div class="plan-form">
	<p class="choices-header">
		Describe a flaw in your own resource assets
	</p>
	<p class="choices-note">
		The plan marred by {scene.marRequired} — {canSubflow ? 'you' : 'the preparer'} must break {scene.marRequired} of {canSubflow ? 'your' : 'their'} own resources.
	</p>
	<CardPicker
		label="Choose a marginalia to tear"
		items={scene.penaltyResources}
		{players}
		readOnly={!canSubflow}
		emptyMessage={scene.isPreparer
				? 'None of your resources can be broken.'
				: `None of ${playerName(players, plan.preparer_id)}'s resources can be broken.`}
		marginaliaMode
		selectedMarginaliaID={drafts.maMargID}
		selectedAssetID={drafts.maAssetID}
		onSelectMarginalia={(mID, parent) => {
			drafts.maMargID = mID;
			drafts.maAssetID = parent?.id ?? null;
		}}
	/>
	{#if maWarn}<Notice>{maWarn}</Notice>{/if}
	{#if canSubflow}
		{#if scene.penaltyResources.length === 0}
			<!-- The empty picker above says whose resources are out; this says what it means for the penalty. -->
			<p class="choices-note muted">This penalty can't be applied.</p>
			<button class="action-btn primary"
				onclick={() => forfeitStep(drafts, plan.id, 'mar_penalty', onPlansChanged, (m) => (error = m))} disabled={drafts.forfeitBusy}>
				{drafts.forfeitBusy ? '…' : 'Skip — no valid targets'}
			</button>
			<EndsNote show={scene.totalOwed === scene.marRemaining} text={lastText} />
		{:else}
			<button class="action-btn primary"
				onclick={() => submitPenaltyFlaw(plan)}
				disabled={maBusy || drafts.maAssetID == null || drafts.maMargID == null}>
				{maBusy ? '…' : 'Break your resource'}
			</button>
			<EndsNote show={scene.totalOwed === 1} text={lastText} />
		{/if}
	{/if}
</div>
