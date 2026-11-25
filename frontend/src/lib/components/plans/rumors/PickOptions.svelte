<!-- Rumors/PickOptions.svelte
  Resolve: the counts picker — options repeatable, total = the dice result on a make,
  (difficulty − result) on a mar.

  Option-picking. The actor picks; everyone else sees the same option list greyed out
  and inert. The counts shown to a watcher are their own (zeroed) local state —
  mirroring the actor's uncommitted picks would need the Tier-2 resolve-draft channel
  ADR-006 leaves open.

  The counts are the panel's (`picks`): they have to survive the take-asset screen and
  the consent wait, when this picker is not on screen.
-->
<script lang="ts">
	import AssetName from '$lib/components/shared/AssetName.svelte';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import { makeChoice, type Plan } from '$lib/api';
	import EndsNote from '../shared/EndsNote.svelte';
	import Stepper from '../shared/Stepper.svelte';
	import { playerName } from '../shared';
	import type { PlanContext } from '../types';
	import { flatChoices, LAST_STEP_TEXT, type RumorScene } from './shared';
	import type { RumorPicks } from './picks.svelte';

	let { ctx, plan, scene, picks, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		scene: RumorScene;
		picks: RumorPicks;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const players = $derived(ctx.players);
	const rollOutcome = $derived(ctx.rollOutcome);
	const activeRoll = $derived(ctx.activeRoll);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	const isActor = $derived(scene.isActor);
	const takeAssetDenied = $derived(scene.srData.take_asset_denied ?? false);

	// The picker speaks to its actor ("Hide yourself…"); everyone else sees the
	// same greyed list, so they get the viewer-neutral wording where there is one.
	const optionText = (o: { label: string; summary?: string }) =>
		isActor ? o.label : (o.summary ?? o.label);

	// Players must pick options equal to the dice result (make) or
	// (difficulty − result) (mar). Compute that exact count so the picker can
	// cap increments and require the full quota before applying.
	const requiredPicks = $derived.by(() => {
		const result = activeRoll?.result;
		if (result == null) return null;
		const difficulty = activeRoll?.adjusted_difficulty ?? activeRoll?.difficulty ?? null;
		if (rollOutcome === 'make') return Math.max(0, result);
		if (rollOutcome === 'mar' && difficulty != null) return Math.max(0, difficulty - result);
		return null;
	});

	function bump(key: string, delta: number) {
		// "Take asset" is locked off once the owner has declined it (no looping).
		if (key === 'take_asset' && takeAssetDenied) return;
		// Don't pick break_target more times than there are marginalia to tear.
		if (delta > 0 && key === 'break_target' && (picks.counts.break_target ?? 0) >= scene.btMarginaliaCap) return;
		// Don't let the running total exceed the dice quota.
		if (delta > 0 && requiredPicks != null && picks.total >= requiredPicks) return;
		const next = Math.max(0, (picks.counts[key] ?? 0) + delta);
		picks.counts = { ...picks.counts, [key]: next };
	}

	// Two-step apply: when "take asset" is picked, the aggressor first names the
	// specific assets to take (consent screen), since those can be ANY of the
	// victim's assets. Without a take, choices commit straight away.
	async function onApplyChoices(p: Plan, outcome: 'make' | 'mar') {
		if (picks.busy || picks.total === 0) return;
		if (requiredPicks != null && picks.total !== requiredPicks) return;
		if ((picks.counts.take_asset ?? 0) > 0) {
			// Advance to the asset-selection screen rather than committing.
			picks.takeSelected = [];
			picks.takeStep = 'assets';
			return;
		}
		picks.busy = true; error = '';
		try {
			await makeChoice(p.id, outcome, flatChoices(scene.options, picks.counts));
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not apply choices.';
		} finally { picks.busy = false; }
	}
</script>

<fieldset class="resolve-mirror-wrap choices-section"
	class:resolve-mirror={!isActor} disabled={!isActor}>
	<p class="choices-header">
		Result: <span class="outcome-{rollOutcome}">
			{rollOutcome === 'make' ? 'Make' : 'Mar'}
		</span>
	</p>
	<p class="choices-note">
		{#if rollOutcome === 'mar'}
			Pick options equal to (difficulty − result) (repeatable){#if requiredPicks != null}: choose <strong>{requiredPicks}</strong>{/if}.
			They land on <AssetName name={scene.mcName} />, <PlayerName {players} id={plan.preparer_id} />'s main character —
			the subject of {#if isActor}your{:else}<PlayerName {players} id={scene.actorID} />'s{/if} counter-rumor.
		{:else}
			Pick options equal to {isActor ? 'your' : 'the'} dice result (repeatable){#if requiredPicks != null}: choose <strong>{requiredPicks}</strong>{/if}.
		{/if}
	</p>
	{#if takeAssetDenied}
		<p class="choices-note muted">
			The owner declined the asset transfer — choose other options.
		</p>
	{/if}
	{#each scene.options as opt}
		{@const optDisabled = opt.key === 'take_asset' && takeAssetDenied}
		{@const atBreakCap = opt.key === 'break_target' && (picks.counts.break_target ?? 0) >= scene.btMarginaliaCap}
		<Stepper label={optionText(opt)} count={picks.counts[opt.key] ?? 0} onstep={(d) => bump(opt.key, d)} dimmed={optDisabled}
			incDisabled={optDisabled || atBreakCap || (requiredPicks != null && picks.total >= requiredPicks)}>
			{optionText(opt)}{#if optDisabled}{' '}(declined){:else if atBreakCap}<span class="choices-note muted"> — no marginalia left</span>{/if}
		</Stepper>
	{/each}
	<p class="choices-note">
		Total picks: <strong>{picks.total}</strong>{#if requiredPicks != null}{' '}/ {requiredPicks}{/if}
	</p>
	{#if isActor}
		<button class="action-btn primary"
			onclick={() => onApplyChoices(plan, rollOutcome!)}
			disabled={picks.busy || picks.total === 0 || (requiredPicks != null && picks.total !== requiredPicks)}>
			{picks.busy ? '…' : ((picks.counts.take_asset ?? 0) > 0 ? 'Next: choose assets' : 'Apply choices')}
		</button>
		<EndsNote show={picks.total > 0 && (picks.counts.take_asset ?? 0) === 0 && (picks.counts.break_target ?? 0) === 0 && (picks.counts.hide_source ?? 0) === 0} text={LAST_STEP_TEXT} />
	{/if}
</fieldset>
