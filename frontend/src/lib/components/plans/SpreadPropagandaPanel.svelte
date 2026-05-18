<!-- SpreadPropagandaPanel.svelte
  Prep + resolve UI for the Spread Propaganda plan.
  Resolve flow: dice roll → make/mar choices → complete.
-->
<script lang="ts">
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		type Plan, type Asset, type Player, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import MakeMarPicker from './MakeMarPicker.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import { MAKE_OPTIONS, MAR_OPTIONS, parseResolutionData, playerName } from './shared';

	import type { PlanPanelProps } from './types';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

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
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	async function submitPrep() {
		if (prepBusy) return;
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'spread_propaganda',
				preparation_notes: prepNotes.trim() || null,
			});
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
		<label class="form-label">
			Message and Methods:
			<textarea rows={2} bind:value={prepNotes} class="form-textarea"
				placeholder="What are you spreading through the realm, and how? Distributing pamphlets? Giving sermons? Feeding talking points to town criers?"></textarea>
		</label>
		<div class="form-actions">
			<button class="action-btn primary" onclick={submitPrep} disabled={prepBusy}>
				{prepBusy ? '…' : 'Prepare Plan'}
			</button>
		</div>
	</div>

{:else if plan}
	{@const existingChoices = (parseResolutionData(plan).make_mar_choices ?? []).map(c => c.option)}
	{@const choicesDone = existingChoices.length > 0}

	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		{#if rollActive && !choicesDone}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome != null && !choicesDone && amChoiceActor}
			<MakeMarPicker
				outcome={rollOutcome}
				options={(rollOutcome === 'make' ? MAKE_OPTIONS.spread_propaganda : MAR_OPTIONS.spread_propaganda) ?? []}
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
				{playerName(players, plan.preparer_id)} is resolving Spread Propaganda…
			</p>
		{/if}
	</ResolvingCard>
{/if}
