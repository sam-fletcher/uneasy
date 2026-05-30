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
		introductionsMar, introductionsMarginalia,
		type Plan,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import MakeMarPicker from './MakeMarPicker.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import {
		MAKE_OPTIONS, parseResolutionData, playerName, assetName, playersExcept,
	} from './shared';
	import { parseMakeIntroductionsData } from '$lib/plans/resolutionData/make_introductions';
	import type { PlanPanelProps } from './types';
	import FormField from './FormField.svelte';

	const MI_MAR_OPTIONS = [
		{ key: 'other_retinue',  label: 'Another retinue' },
		{ key: 'broken_arrival', label: 'Arrives broken' },
		{ key: 'delayed',        label: 'Delayed' },
		{ key: 'broken_journey', label: 'Broken journey' },
	];

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	// Flat-name shim so the existing body keeps reading bare names. Each
	// $derived accessor stays reactive to ctx field changes.
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
	const prepDraft = $derived(ctx.prepDraft as { peer_count?: number; notes?: string } | null);

	let performStepsWinnerID = $state<number | null>(null);
	// The preparer resolves their own plan; the perform_steps demand winner may
	// drive the choice in their place.
	const isPreparer = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const amChoiceActor = $derived(
		isPreparer || (currentPlayerID != null && currentPlayerID === performStepsWinnerID),
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

	// ── Per-peer mar resolution ───────────────────────────────────────────────
	const marPending = $derived(!!miData.mar_pending);
	const marOutcomes = $derived(miData.mar_outcomes ?? []);
	function outcomeFor(peerID: number) {
		return marOutcomes.find(o => o.peer_asset_id === peerID) ?? null;
	}
	const unresolvedPeerIDs = $derived(createdPeerIDs.filter(id => !outcomeFor(id)));
	const allPeersDone = $derived(
		createdPeerIDs.length > 0 && createdPeerIDs.every(id => outcomeFor(id)?.done),
	);
	// broken_arrival peers this player has been assigned to author.
	const myAuthorPeerIDs = $derived(
		marOutcomes
			.filter(o => o.outcome === 'broken_arrival' && !o.done && o.author_player_id === currentPlayerID)
			.map(o => o.peer_asset_id),
	);

	let marOutcome = $state('other_retinue');
	let marTargetPlayer = $state<number | null>(null);
	let marText = $state('');
	let marBusy = $state(false);
	async function submitMar(peerID: number) {
		if (marBusy || !plan) return;
		marBusy = true; resError = '';
		try {
			const params: { peer_asset_id: number; outcome: string; target_player_id?: number; text?: string } = {
				peer_asset_id: peerID, outcome: marOutcome,
			};
			if (marOutcome === 'other_retinue' || marOutcome === 'broken_arrival') {
				params.target_player_id = marTargetPlayer ?? undefined;
			}
			if (marOutcome === 'broken_journey') params.text = marText.trim();
			await introductionsMar(plan.id, params);
			marOutcome = 'other_retinue'; marTargetPlayer = null; marText = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not resolve peer.';
		} finally { marBusy = false; }
	}

	let authorText = $state('');
	let authorBusy = $state(false);
	async function submitAuthor(peerID: number) {
		if (authorBusy || !plan || !authorText.trim()) return;
		authorBusy = true; resError = '';
		try {
			await introductionsMarginalia(plan.id, peerID, authorText.trim());
			authorText = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not write marginalia.';
		} finally { authorBusy = false; }
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
		{#if needsPeerNaming && isPreparer}
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

		{:else if needsPeerNaming && !isPreparer}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is naming the new peers…
			</p>

		{:else if rollActive && !choicesDone && !marPending}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome === 'make'}
			{#if !choicesDone && amChoiceActor}
				<MakeMarPicker
					outcome="make"
					options={MAKE_OPTIONS.make_introductions ?? []}
					selected={selectedChoices}
					busy={choicesBusy}
					onToggle={toggleChoice}
					onSubmit={() => onApplyChoices(plan, 'make')}
				/>
			{:else if choicesDone && isPreparer}
				<div class="complete-section">
					<p class="complete-note">
						Write any follow-scene narration in the scene thread, then complete the plan.
					</p>
					<button class="action-btn primary" onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				</div>
			{:else if !choicesDone}
				<p class="ft-prompt muted">
					{playerName(players, plan.preparer_id)} is resolving Make Introductions…
				</p>
			{/if}

		{:else if rollOutcome === 'mar'}
			{#if !marPending}
				{#if isPreparer}
					<div class="complete-section">
						<p class="choices-note">The introductions were marred — resolve each newcomer's fate.</p>
						<button class="action-btn primary" onclick={() => onApplyChoices(plan, 'mar')} disabled={choicesBusy}>
							{choicesBusy ? '…' : 'Begin resolving'}
						</button>
					</div>
				{:else}
					<p class="ft-prompt muted">
						{playerName(players, plan.preparer_id)} is resolving the marred introductions…
					</p>
				{/if}
			{:else}
				<div class="mi-mar">
					{#each createdPeerIDs as pid (pid)}
						{@const o = outcomeFor(pid)}
						{#if o}
							<p class="choices-applied">
								{assetName(assets, pid)} — {o.outcome.replace('_', ' ')}{o.outcome === 'broken_arrival' && !o.done ? ' (awaiting marginalia)' : ' ✓'}
							</p>
						{/if}
					{/each}

					{#each myAuthorPeerIDs as pid (pid)}
						<div class="plan-form">
							<p class="ft-prompt">
								Write the marginalia that defines <strong>{assetName(assets, pid)}</strong>.
							</p>
							<textarea rows={2} class="form-textarea" bind:value={authorText}
								placeholder="Who has arrived at court?"></textarea>
							<button class="action-btn primary" onclick={() => submitAuthor(pid)}
								disabled={authorBusy || !authorText.trim()}>
								{authorBusy ? '…' : 'Write marginalia'}
							</button>
						</div>
					{/each}

					{#if isPreparer && unresolvedPeerIDs.length > 0}
						{@const pid = unresolvedPeerIDs[0]}
						<div class="plan-form">
							<p class="ft-prompt">
								Resolve <strong>{assetName(assets, pid)}</strong>
								({unresolvedPeerIDs.length} remaining):
							</p>
							<div class="chip-row">
								{#each MI_MAR_OPTIONS as opt (opt.key)}
									<button type="button" class="chip-btn" class:active={marOutcome === opt.key}
										onclick={() => (marOutcome = opt.key)}>{opt.label}</button>
								{/each}
							</div>
							{#if marOutcome === 'other_retinue' || marOutcome === 'broken_arrival'}
								<FormField label={marOutcome === 'other_retinue' ? 'Joins which retinue' : 'Who defines them'}>
									<PlayerChips
										players={playersExcept(players, plan.preparer_id)}
										isActive={(p) => marTargetPlayer === p.id}
										onSelect={(p) => (marTargetPlayer = p.id)}
									/>
								</FormField>
							{:else if marOutcome === 'broken_journey'}
								<label class="form-label">
									Marginalia (then broken):
									<textarea rows={2} class="form-textarea" bind:value={marText}
										placeholder="The mark of an arduous journey…"></textarea>
								</label>
							{/if}
							<button class="action-btn primary" onclick={() => submitMar(pid)}
								disabled={marBusy
									|| ((marOutcome === 'other_retinue' || marOutcome === 'broken_arrival') && marTargetPlayer == null)
									|| (marOutcome === 'broken_journey' && !marText.trim())}>
								{marBusy ? '…' : 'Resolve peer'}
							</button>
						</div>
					{/if}

					{#if allPeersDone && isPreparer}
						<div class="complete-section">
							<p class="complete-note">All newcomers resolved. Complete the plan.</p>
							<button class="action-btn primary" onclick={() => onComplete(plan)} disabled={resBusy}>
								{resBusy ? '…' : 'Complete plan'}
							</button>
						</div>
					{:else if !isPreparer && myAuthorPeerIDs.length === 0}
						<p class="ft-prompt muted">
							{playerName(players, plan.preparer_id)} is resolving the marred introductions…
						</p>
					{/if}
				</div>
			{/if}
		{/if}
	</ResolvingCard>
{/if}
