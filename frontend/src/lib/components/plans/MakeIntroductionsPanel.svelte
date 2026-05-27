<!-- MakeIntroductionsPanel.svelte
  Prep + resolve UI for the Make Introductions plan.
  Resolve flow: dice roll → make/mar choices → complete.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		createIntroductionsPeer, finalizeIntroductionsPeers,
		type Plan,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import MakeMarPicker from './MakeMarPicker.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import { MAKE_OPTIONS, MAR_OPTIONS, parseResolutionData, playerName } from './shared';
	import { parseMakeIntroductionsData } from '$lib/plans/resolutionData/make_introductions';
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

	const readOnly = $derived(ctx.readOnly);
	const prepDraft = $derived(ctx.prepDraft as { peer_count?: number; notes?: string } | null);

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
		if (!prepNotes.trim()) { prepError = 'Preparation notes are required.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'make_introductions',
				peer_count: miPeerCount,
				preparation_notes: prepNotes.trim(),
			});
			miPeerCount = 1;
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	$effect(() => {
		if (!readOnly) return;
		miPeerCount = prepDraft?.peer_count ?? 1;
		prepNotes = prepDraft?.notes ?? '';
	});
	let emitTimer: ReturnType<typeof setTimeout> | null = null;
	$effect(() => {
		if (readOnly || mode !== 'prep') return;
		void miPeerCount; void prepNotes;
		if (emitTimer) clearTimeout(emitTimer);
		emitTimer = setTimeout(() => {
			emitTimer = null;
			ctx.emitPrepDraft({ peer_count: miPeerCount, notes: prepNotes });
		}, 150);
	});
	onDestroy(() => { if (emitTimer) clearTimeout(emitTimer); });

	// Resolve state
	let resError = $state('');
	let resBusy = $state(false);
	let selectedChoices = $state<string[]>([]);
	let choicesBusy = $state(false);

	// Pre-roll peer-naming state.
	//
	// MI defers its dice roll until the focus player has named each of
	// peer_count peers. peerNames is sized to peer_count; entries that
	// correspond to already-created peers are filled with their asset
	// names so the focus player can resume after a refresh.
	const miData = $derived(plan ? parseMakeIntroductionsData(plan) : {});
	const miPeerCountTarget = $derived(miData.peer_count ?? 0);
	const createdPeerIDs = $derived(miData.created_peer_ids ?? []);
	const peersNamingDone = $derived(
		miPeerCountTarget > 0 && createdPeerIDs.length >= miPeerCountTarget
	);
	// True only while we're in the pre-roll naming window: plan resolving,
	// no dice roll yet, focus player hasn't finalized.
	const needsPeerNaming = $derived(
		plan != null
			&& !rollActive
			&& rollOutcome == null
			&& miPeerCountTarget > 0
			&& !peersNamingDone
	);

	let peerNames = $state<string[]>([]);
	let peersBusy = $state(false);
	let peersError = $state('');

	// Resize peerNames whenever peer_count or created list changes.
	// Already-created slots are filled with the asset's current name (so
	// the user can see them after a refresh); empty slots are editable.
	$effect(() => {
		if (!plan) return;
		const total = miPeerCountTarget;
		const next: string[] = [];
		for (let i = 0; i < total; i++) {
			const createdID = createdPeerIDs[i];
			if (createdID != null) {
				const a = assets.find(x => x.id === createdID);
				next.push(a?.name ?? `Peer ${i + 1}`);
			} else {
				next.push(peerNames[i] ?? '');
			}
		}
		peerNames = next;
	});

	async function submitPeers() {
		if (peersBusy || !plan) return;
		const startIdx = createdPeerIDs.length;
		for (let i = startIdx; i < miPeerCountTarget; i++) {
			const name = (peerNames[i] ?? '').trim();
			if (!name) {
				peersError = `Name peer ${i + 1} before continuing.`;
				return;
			}
		}
		peersBusy = true; peersError = '';
		try {
			for (let i = startIdx; i < miPeerCountTarget; i++) {
				await createIntroductionsPeer(plan.id, { name: peerNames[i].trim() });
			}
			await finalizeIntroductionsPeers(plan.id);
			onPlansChanged();
		} catch (e) {
			peersError = e instanceof Error ? e.message : 'Could not finalize peers.';
		} finally { peersBusy = false; }
	}

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
	<fieldset class="plan-form-fieldset" disabled={readOnly}>
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
					placeholder="What role will they fill, in court or otherwise?" required></textarea>
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
		{#if needsPeerNaming && isFocusPlayer}
			<div class="mi-naming">
				<p class="form-hint">
					Name each of the {miPeerCountTarget}
					{miPeerCountTarget === 1 ? 'peer' : 'peers'} you're introducing.
					Once you finalize, the dice will roll.
				</p>
				{#if peersError}<p class="res-error">{peersError}</p>{/if}
				{#each peerNames as _, i (i)}
					{@const locked = createdPeerIDs[i] != null}
					<label class="form-label">
						Peer {i + 1}:
						<input
							type="text"
							class="form-input"
							bind:value={peerNames[i]}
							disabled={locked || peersBusy}
							placeholder="Name, title, role…"
						/>
					</label>
				{/each}
				<div class="form-actions">
					<button class="action-btn primary" onclick={submitPeers} disabled={peersBusy}>
						{peersBusy ? '…' : (createdPeerIDs.length > 0 ? 'Resume & roll' : 'Create peers & roll')}
					</button>
				</div>
			</div>

		{:else if needsPeerNaming && !isFocusPlayer}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is naming the new peers…
			</p>

		{:else if rollActive && !choicesDone}
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
