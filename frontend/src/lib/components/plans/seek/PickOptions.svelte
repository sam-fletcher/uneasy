<!-- Seek/PickOptions.svelte
  Resolve: option counts picker (repeatable, total = dice result) → makeChoice.

  The option list is the same for make and mar (per spec). The choice actor (preparer,
  or a perform_steps demand winner acting in their place) picks; everyone else sees the
  same option list greyed out. On a mar the preparer must additionally describe a flaw
  in (difficulty − result) of their OWN resources; that penalty sub-flow blocks completion
  until satisfied, and the note here says what it will be.
-->
<script lang="ts">
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import { makeChoice, type Plan } from '$lib/api';
	import Stepper from '../shared/Stepper.svelte';
	import { playerName } from '../shared';
	import type { PlanContext } from '../types';
	import { OPTIONS, type SeekScene } from './shared';
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
	const rollOutcome = $derived(ctx.rollOutcome);
	const activeRoll = $derived(ctx.activeRoll);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	let choicesBusy = $state(false);

	// "Choose a number of these options equal to your result" — exactly result,
	// on both a make and a mar (MaxChoices = result).
	const requiredPicks = $derived(activeRoll?.result ?? null);

	const atCap = (key: string): boolean => {
		const cap = scene.optionCap[key];
		return cap != null && (drafts.counts[key] ?? 0) >= cap;
	};

	function bump(key: string, delta: number) {
		// Can't pick an option with no valid target, or past its live target count.
		if (delta > 0 && (!scene.optionAvailable[key] || atCap(key))) return;
		// Don't let the running total exceed the dice quota.
		if (delta > 0 && requiredPicks != null && drafts.total >= requiredPicks) return;
		const next = Math.max(0, (drafts.counts[key] ?? 0) + delta);
		drafts.counts = { ...drafts.counts, [key]: next };
	}

	async function onApplyChoices(p: Plan, outcome: 'make' | 'mar') {
		if (choicesBusy || drafts.total === 0) return;
		if (requiredPicks != null && drafts.total !== requiredPicks) return;
		choicesBusy = true; error = '';
		try {
			const flat: string[] = [];
			for (const opt of OPTIONS) {
				for (let i = 0; i < (drafts.counts[opt.key] ?? 0); i++) flat.push(opt.key);
			}
			await makeChoice(p.id, outcome, flat);
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not apply choices.';
		} finally { choicesBusy = false; }
	}

	// The choice actor (preparer, or a perform_steps demand winner acting in their
	// place) picks; everyone else sees the same option list greyed out.
	const canPick = $derived(scene.amChoiceActor);
	const marPenalty = $derived(scene.marPenalty);
</script>

<fieldset class="resolve-mirror-wrap" class:resolve-mirror={!canPick} disabled={!canPick}>
	<div class="choices-section">
		<p class="choices-header">
			<span class="outcome-{rollOutcome}">
				{rollOutcome === 'make' ? 'Make' : 'Mar'}
			</span> options
		</p>
		<p class="choices-note">
			Pick options equal to {canPick ? 'your' : 'the'} dice result (repeatable){#if requiredPicks != null}: choose exactly {requiredPicks}{/if}.
			{#if marPenalty && marPenalty.owed > 0}
				{#if canPick && scene.isPreparer}You{:else}<PlayerName {players} id={plan.preparer_id} />{/if} must also describe a flaw in (break) {marPenalty.owed} of {canPick && scene.isPreparer ? 'your' : 'their'} own resource assets, after this step.
				{#if marPenalty.owed < marPenalty.shortfall}
					{' '}The plan marred by {marPenalty.shortfall}, but only {marPenalty.owed} of {canPick && scene.isPreparer ? 'your' : 'their'} resources can still be flawed.
				{/if}
			{:else if marPenalty && marPenalty.shortfall > 0}
				The plan marred by {marPenalty.shortfall}, but {#if canPick && scene.isPreparer}you have{:else}<PlayerName {players} id={plan.preparer_id} /> has{/if} no resource left to flaw, so {canPick && scene.isPreparer ? 'you owe' : 'owes'} none.
			{/if}
		</p>
		{#each OPTIONS as opt}
			{@const available = scene.optionAvailable[opt.key]}
			<Stepper label={opt.label} count={drafts.counts[opt.key] ?? 0} onstep={(d) => bump(opt.key, d)} dimmed={!available}
				incDisabled={!available || atCap(opt.key) || (requiredPicks != null && drafts.total >= requiredPicks)}>
				{opt.label}{#if !available}{' '}<span class="choices-note muted">— no valid targets</span>{:else if atCap(opt.key)}{' '}<span class="choices-note muted">— all targets picked</span>{/if}
			</Stepper>
		{/each}
		<p class="choices-note">
			Total picks: <strong>{drafts.total}</strong>{#if requiredPicks != null}{' '}/ {requiredPicks}{/if}
		</p>
		{#if canPick}
			<button class="action-btn primary"
				onclick={() => onApplyChoices(plan, rollOutcome!)}
				disabled={choicesBusy || drafts.total === 0 || (requiredPicks != null && drafts.total !== requiredPicks)}>
				{choicesBusy ? '…' : 'Apply choices'}
			</button>
		{/if}
	</div>
</fieldset>
