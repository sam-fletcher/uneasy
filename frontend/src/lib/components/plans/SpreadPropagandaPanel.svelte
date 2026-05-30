<!-- SpreadPropagandaPanel.svelte
  Prep + resolve UI for the Spread Propaganda plan.
  Resolve flow: dice roll → make/mar choices → complete.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		type Plan, type Asset, type Player, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import MakeMarPicker from './MakeMarPicker.svelte';
	import CardPicker from './CardPicker.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import {
		MAKE_OPTIONS, MAR_OPTIONS, parseResolutionData, playerName,
		assetsWithIntactMarginalia, playersExcept,
	} from './shared';
	import { spGivePeer, spBreakSelf } from '$lib/api';

	import type { PlanPanelProps } from './types';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const rollActive = $derived(ctx.rollActive);
	const rollOutcome = $derived(ctx.rollOutcome);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	const readOnly = $derived(ctx.readOnly);
	const prepDraft = $derived(ctx.prepDraft as { notes?: string } | null);

	let performStepsWinnerID = $state<number | null>(null);
	// The preparer resolves their own plan; the perform_steps demand winner may
	// drive the choice in their place.
	const isPreparer = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const amChoiceActor = $derived(
		isPreparer || (currentPlayerID != null && currentPlayerID === performStepsWinnerID),
	);

	// Prep state
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	async function submitPrep() {
		if (prepBusy) return;
		if (!prepNotes.trim()) { prepError = 'Preparation notes are required.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'spread_propaganda',
				preparation_notes: prepNotes.trim(),
			});
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	$effect(() => { if (readOnly) prepNotes = prepDraft?.notes ?? ''; });
	let emitTimer: ReturnType<typeof setTimeout> | null = null;
	$effect(() => {
		if (readOnly || mode !== 'prep') return;
		void prepNotes;
		if (emitTimer) clearTimeout(emitTimer);
		emitTimer = setTimeout(() => {
			emitTimer = null;
			ctx.emitPrepDraft({ notes: prepNotes });
		}, 150);
	});
	onDestroy(() => { if (emitTimer) clearTimeout(emitTimer); });

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

	// ── Mar asset-picker effects (driven by the preparer) ─────────────────────
	// give_peer (a) and break_self (c) need the preparer to pick an asset, so
	// they resolve through dedicated routes after the make-choice is recorded.
	const spData = $derived(plan ? (parseResolutionData(plan).spread_propaganda ?? {}) : {});
	const needsGivePeer = $derived(!!spData.give_peer_required && !spData.give_peer_done);
	const needsBreakSelf = $derived(!!spData.break_self_required && !spData.break_self_done);
	const marEffectsPending = $derived(needsGivePeer || needsBreakSelf);

	// give_peer: pick one of the preparer's peers and a recipient player.
	const preparerPeers = $derived(
		plan ? assets.filter(a => a.owner_id === plan.preparer_id && a.asset_type === 'peer' && !a.is_destroyed) : []
	);
	let givePeerID = $state<number | null>(null);
	let giveToPlayerID = $state<number | null>(null);
	let giveBusy = $state(false);
	async function submitGivePeer(p: Plan) {
		if (giveBusy || givePeerID == null || giveToPlayerID == null) return;
		giveBusy = true; resError = '';
		try {
			await spGivePeer(p.id, givePeerID, giveToPlayerID);
			givePeerID = null; giveToPlayerID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not give up peer.';
		} finally { giveBusy = false; }
	}

	// break_self: tear one marginalium from one of the preparer's own assets.
	const breakSelfAssets = $derived(
		plan ? assetsWithIntactMarginalia(assets, plan.preparer_id) : []
	);
	let breakMargID = $state<number | null>(null);
	let breakBusy = $state(false);
	async function submitBreakSelf(p: Plan) {
		if (breakBusy || breakMargID == null) return;
		breakBusy = true; resError = '';
		try {
			await spBreakSelf(p.id, breakMargID);
			breakMargID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not break asset.';
		} finally { breakBusy = false; }
	}
</script>

{#if mode === 'prep'}
	<fieldset class="plan-form-fieldset" disabled={readOnly}>
		<div class="plan-form">
			{#if prepError}<p class="res-error">{prepError}</p>{/if}
			<label class="form-label">
				Message and Methods:
				<textarea rows={2} bind:value={prepNotes} class="form-textarea"
					placeholder="What are you spreading through the realm, and how? Distributing pamphlets? Giving sermons? Feeding talking points to town criers?" required></textarea>
			</label>
			{#if !readOnly}
				<div class="form-actions">
					<button class="action-btn primary" onclick={submitPrep} disabled={prepBusy || !prepNotes.trim()}>
						{prepBusy ? '…' : 'Prepare Plan'}
					</button>
				</div>
			{/if}
		</div>
	</fieldset>

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

		{:else if choicesDone}
			<div class="complete-section">
				{#if existingChoices.length > 0}
					<p class="choices-applied">Choices applied: {existingChoices.join(', ')}</p>
				{/if}

				{#if isPreparer && needsGivePeer}
					<div class="plan-form">
						<p class="choices-header">A peer leaves your retinue — hand it to another player</p>
						<CardPicker
							label="Peer to give up"
							items={preparerPeers}
							{players}
							emptyMessage="You have no peers to give."
							selected={givePeerID}
							onSelect={(id) => (givePeerID = id)}
						/>
						<PlayerChips
							players={playersExcept(players, plan.preparer_id)}
							isActive={(pl) => giveToPlayerID === pl.id}
							onSelect={(pl) => (giveToPlayerID = pl.id)}
						/>
						<button class="action-btn primary" onclick={() => submitGivePeer(plan)}
							disabled={giveBusy || givePeerID == null || giveToPlayerID == null}>
							{giveBusy ? '…' : 'Give up peer'}
						</button>
					</div>
				{/if}

				{#if isPreparer && needsBreakSelf}
					<div class="plan-form">
						<p class="choices-header">Break yourself — tear a marginalium from one of your assets</p>
						<CardPicker
							label="Marginalium to tear"
							items={breakSelfAssets}
							{players}
							emptyMessage="You have no intact marginalia."
							marginaliaMode
							selectedMarginaliaID={breakMargID}
							onSelectMarginalia={(mID) => (breakMargID = mID)}
						/>
						<button class="action-btn primary" onclick={() => submitBreakSelf(plan)}
							disabled={breakBusy || breakMargID == null}>
							{breakBusy ? '…' : 'Tear marginalia'}
						</button>
					</div>
				{/if}

				{#if isPreparer && !marEffectsPending}
					<p class="complete-note">
						Write any follow-scene narration in the scene thread, then complete the plan.
					</p>
					<button class="action-btn primary" onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				{:else if marEffectsPending && !isPreparer}
					<p class="ft-prompt muted">
						Waiting for {playerName(players, plan.preparer_id)} to face the fallout…
					</p>
				{/if}
			</div>

		{:else if !amChoiceActor && !choicesDone}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is resolving Spread Propaganda…
			</p>
		{/if}
	</ResolvingCard>
{/if}
