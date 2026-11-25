<!-- Seek/RevealSecret.svelte
  Sub-step: reveal an asset's secrets. The target is another player's asset ("ask a
  player to show you the underside of a specific one of their assets"), never the
  preparer's own. Only the choice actor works it; everyone else sees the same picker
  greyed out and read-only.
-->
<script lang="ts">
	import { revealSecret, type Plan } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import EndsNote from '../shared/EndsNote.svelte';
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

	const players = $derived(ctx.players);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	const canSubflow = $derived(scene.amChoiceActor);
	const rsRemaining = $derived(scene.rsRemaining);

	let rsBusy = $state(false);

	async function submitRevealSecret(p: Plan) {
		if (rsBusy || drafts.rsAssetID == null) return;
		rsBusy = true; error = '';
		try {
			await revealSecret(p.id, drafts.rsAssetID);
			drafts.rsAssetID = null;
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not reveal secret.';
		} finally { rsBusy = false; }
	}
</script>

<div class="plan-form">
	<p class="choices-header">Reveal an asset's secrets</p>
	<p class="choices-note">{rsRemaining} remaining.</p>
	<CardPicker
		label="Asset"
		items={scene.revealableAssets}
		{players}
		readOnly={!canSubflow}
		emptyMessage="No asset's secrets remain to learn."
		ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
		selected={drafts.rsAssetID}
		onSelect={(id) => (drafts.rsAssetID = id)}
	/>
	{#if canSubflow}
		{#if scene.revealableAssets.length === 0}
			<!-- The empty picker above says why; this says what it means for the pick. -->
			<p class="choices-note muted">This pick has no valid target.</p>
			<button class="action-btn primary"
				onclick={() => forfeitStep(drafts, plan.id, 'reveal_secret', onPlansChanged, (m) => (error = m))} disabled={drafts.forfeitBusy}>
				{drafts.forfeitBusy ? '…' : 'Skip — no valid targets'}
			</button>
			<EndsNote show={scene.totalOwed === rsRemaining} text={lastText} />
		{:else}
			<button class="action-btn primary"
				onclick={() => submitRevealSecret(plan)}
				disabled={rsBusy || drafts.rsAssetID == null}>
				{rsBusy ? '…' : 'Reveal secrets'}
			</button>
			<EndsNote show={scene.totalOwed === 1} text={lastText} />
		{/if}
	{/if}
</div>
