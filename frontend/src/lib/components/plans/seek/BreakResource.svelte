<!-- Seek/BreakResource.svelte
  Sub-step: break a resource (describe a flaw). Each resource may be flawed at most once
  ("overlooked until now"), so flawed resources drop out of the picker. Only the choice
  actor works it; everyone else sees the same picker greyed out and read-only.
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
	const brRemaining = $derived(scene.brRemaining);

	let brBusy = $state(false);
	// Destruction warning for the currently-selected break target.
	const brWarn = $derived(destructionWarning(assets.find(a => a.id === drafts.brAssetID)));

	async function submitBreakResource(p: Plan) {
		if (brBusy || drafts.brAssetID == null || drafts.brMargID == null) return;
		brBusy = true; error = '';
		try {
			await breakResource(p.id, drafts.brAssetID, drafts.brMargID);
			drafts.brAssetID = null; drafts.brMargID = null;
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not break resource.';
		} finally { brBusy = false; }
	}
</script>

<div class="plan-form">
	<p class="choices-header">
		Break a resource ({brRemaining} remaining)
	</p>
	<CardPicker
		label="Marginalium to tear"
		items={scene.brResourcesWithMarginalia}
		{players}
		readOnly={!canSubflow}
		emptyMessage="No resource can be broken."
		ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
		marginaliaMode
		selectedMarginaliaID={drafts.brMargID}
		selectedAssetID={drafts.brAssetID}
		onSelectMarginalia={(mID, parent) => {
			drafts.brMargID = mID;
			drafts.brAssetID = parent?.id ?? null;
		}}
	/>
	{#if brWarn}<Notice>{brWarn}</Notice>{/if}
	{#if canSubflow}
		{#if scene.brResourcesWithMarginalia.length === 0}
			<!-- The empty picker above already says nothing can be broken. -->
			<p class="choices-note muted">This pick has no valid target.</p>
			<button class="action-btn primary"
				onclick={() => forfeitStep(drafts, plan.id, 'break_resource', onPlansChanged, (m) => (error = m))} disabled={drafts.forfeitBusy}>
				{drafts.forfeitBusy ? '…' : 'Skip — no valid targets'}
			</button>
			<EndsNote show={scene.totalOwed === brRemaining} text={lastText} />
		{:else}
			<button class="action-btn primary"
				onclick={() => submitBreakResource(plan)}
				disabled={brBusy || drafts.brAssetID == null || drafts.brMargID == null}>
				{brBusy ? '…' : 'Break resource'}
			</button>
			<EndsNote show={scene.totalOwed === 1} text={lastText} />
		{/if}
	{/if}
</div>
