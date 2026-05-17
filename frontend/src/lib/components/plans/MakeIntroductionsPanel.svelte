<!-- MakeIntroductionsPanel.svelte
  Prep + resolve UI for the Make Introductions plan.
  Resolve flow: dice roll → make/mar choices → complete.
-->
<script lang="ts">
	import './planPanel.css';
	import { preparePlan, makeChoice, completePlan, type Plan } from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import MakeMarPicker from './MakeMarPicker.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import { MAKE_OPTIONS, MAR_OPTIONS, parseResolutionData, playerName } from './shared';
	import type { PlanPanelProps } from './types';
	import FormField from './FormField.svelte';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	// Flat-name shim so the existing body keeps reading bare names. Each
	// $derived accessor stays reactive to ctx field changes.
	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const isFocusPlayer = $derived(ctx.isFocusPlayer);
	const rollActive = $derived(ctx.rollActive);
	const rollOutcome = $derived(ctx.rollOutcome);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	let performStepsWinnerID = $state<number | null>(null);
	const amChoiceActor = $derived(
		isFocusPlayer || (currentPlayerID != null && currentPlayerID === performStepsWinnerID),
	);

	// Prep state
	let miPeerCount = $state(1);
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	async function submitPrep() {
		if (prepBusy) return;
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'make_introductions',
				peer_count: miPeerCount,
				preparation_notes: prepNotes.trim() || null,
			});
			miPeerCount = 1;
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	// Resolve state
	let resError = $state('');
	let resBusy = $state(false);
	let selectedChoices = $state<string[]>([]);
	let choicesBusy = $state(false);

	function toggleChoice(key: string) {
		selectedChoices = selectedChoices.includes(key)
			? selectedChoices.filter(k => k !== key)
			: [...selectedChoices, key];
	}

	async function onApplyChoices(p: Plan, outcome: 'make' | 'mar') {
		if (choicesBusy) return;
		choicesBusy = true; resError = '';
		try {
			await makeChoice(p.id, outcome, selectedChoices);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not apply choices.';
		} finally { choicesBusy = false; }
	}

	async function onComplete(p: Plan) {
		if (resBusy) return;
		resBusy = true; resError = '';
		try {
			await completePlan(p.id);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not complete plan.';
		} finally { resBusy = false; }
	}
</script>

{#if mode === 'prep'}
	<div class="plan-form">
		{#if prepError}<p class="res-error">{prepError}</p>{/if}
		<FormField label="Number of peers">
			<div class="chip-row">
				{#each [1, 2, 3, 4] as n}
					<button
						type="button"
						class="chip-btn"
						class:active={miPeerCount === n}
						onclick={() => (miPeerCount = n)}
					>
						{n}
					</button>
				{/each}
			</div>
		</FormField>
		<p class="form-hint">Difficulty will be {2 + miPeerCount}.</p>
		<label class="form-label">
			Intent:
			<textarea rows={2} bind:value={prepNotes} class="form-textarea"
				placeholder="What role will they fill, in court or otherwise?"></textarea>
		</label>
		<div class="form-actions">
			<button class="action-btn primary" onclick={submitPrep} disabled={prepBusy}>
				{prepBusy ? '…' : 'Prepare Plan'}
			</button>
		</div>
	</div>

{:else if plan}
	{@const existingChoices = parseResolutionData(plan).choices ?? []}
	{@const choicesDone = existingChoices.length > 0}

	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		{#if rollActive && !choicesDone}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome != null && !choicesDone && amChoiceActor}
			<MakeMarPicker
				outcome={rollOutcome}
				options={(rollOutcome === 'make' ? MAKE_OPTIONS.make_introductions : MAR_OPTIONS.make_introductions) ?? []}
				selected={selectedChoices}
				busy={choicesBusy}
				onToggle={toggleChoice}
				onSubmit={() => onApplyChoices(plan, rollOutcome!)}
			/>

		{:else if choicesDone && isFocusPlayer}
			<div class="complete-section">
				{#if existingChoices.length > 0}
					<p class="choices-applied">Choices applied: {existingChoices.join(', ')}</p>
				{/if}
				<p class="complete-note">
					Write any follow-scene narration in the scene thread, then complete the plan.
				</p>
				<button class="action-btn primary" onclick={() => onComplete(plan)} disabled={resBusy}>
					{resBusy ? '…' : 'Complete plan'}
				</button>
			</div>

		{:else if !amChoiceActor && !choicesDone}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is resolving Make Introductions…
			</p>
		{/if}
	</ResolvingCard>
{/if}
