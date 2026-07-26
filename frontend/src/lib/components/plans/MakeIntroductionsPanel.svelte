<!-- MakeIntroductionsPanel.svelte
  Prep + resolve UI for the Make Introductions plan.
  Resolve flow: name the peers → dice roll → make/mar → arrivals → complete.

  The peers named pre-roll are DRAFTS, not assets: nobody joins a retinue until
  they arrive, and arriving is what creates them (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md
  D4). So this panel addresses peers by draft id throughout, reads their names
  out of resolution_data rather than the asset list, and owns two arrival forms:

    - the preparer's, for the make step's "add marginalia to each" and for a
      delayed peer finally turning up on their rescheduled row;
    - another player's, for the two mar outcomes whose marginalia isn't the
      preparer's to write.
-->
<script lang="ts">
	import { onDestroy, untrack } from 'svelte';
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		createIntroductionsPeer, finalizeIntroductionsPeers, introductionsArrival,
		introductionsMar, introductionsMarginalia, getAssetSuggestions,
		type Plan,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import MakeMarPicker from './MakeMarPicker.svelte';
	import SuggestionPicker from '../SuggestionPicker.svelte';
	import AssetCreationForm from '../AssetCreationForm.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import {
		MAKE_OPTIONS, parseResolutionData, playerName, playersExcept,
	} from './shared';
	import {
		parseMakeIntroductionsData, miDrafts, miHasArrived, miPendingArrivals,
	} from '$lib/plans/resolutionData/make_introductions';
	import type { PlanPanelProps } from './types';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import FormField from './FormField.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

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
	// MI defers its dice roll until the preparer has named each of peer_count
	// peers. peerNames is sized to peer_count; entries that correspond to
	// already-recorded drafts are filled with the draft's name so the preparer
	// can resume after a refresh.
	const miData = $derived(plan ? parseMakeIntroductionsData(plan) : {});
	const miPeerCountTarget = $derived(miData.peer_count ?? 0);
	const drafts = $derived(miData.drafts ?? []);
	// A synthetic delayed-arrival plan carries one traveller and never names
	// anyone; it goes straight to that peer's arrival form.
	const isDelayedArrival = $derived(!!miData.delayed_arrival);
	const peersNamingDone = $derived(
		miPeerCountTarget > 0 && drafts.length >= miPeerCountTarget
	);
	// True only while we're in the pre-roll naming window: plan resolving,
	// no dice roll yet, preparer hasn't finalized.
	const needsPeerNaming = $derived(
		plan != null
			&& !isDelayedArrival
			&& !rollActive
			&& rollOutcome == null
			&& miPeerCountTarget > 0
			&& !peersNamingDone
	);

	let peerNames = $state<string[]>([]);
	let peersBusy = $state(false);
	let peersError = $state('');

	// Peer-name suggestions (shared across all peer slots), fetched once the
	// naming step appears for the preparer.
	let peerNameSuggestions = $state<string[]>([]);
	let peerNameSuggLoading = $state(false);
	let peerNameSuggFetched = false;
	$effect(() => {
		if (!needsPeerNaming || !isPreparer || peerNameSuggFetched) return;
		peerNameSuggFetched = true;
		peerNameSuggLoading = true;
		getAssetSuggestions(gameID, 'peer', 'name')
			.then(res => { peerNameSuggestions = res.suggestions; })
			.catch(() => { peerNameSuggestions = []; })
			.finally(() => { peerNameSuggLoading = false; });
	});

	// Resize peerNames whenever peer_count or the draft list changes.
	// Already-named slots are filled with the draft's name (so the user can see
	// them after a refresh); empty slots keep whatever's been typed so far.
	//
	// The carry-over read of peerNames is untracked: an effect that both reads
	// and writes the same state re-triggers itself, and Svelte aborts the flush
	// — which strands every other pending update in the component (it was the
	// suggestion pickers stuck on "Loading suggestions…").
	$effect(() => {
		if (!plan) return;
		const total = miPeerCountTarget;
		const named = drafts.map(d => d.name);
		untrack(() => {
			const next: string[] = [];
			for (let i = 0; i < total; i++) {
				next.push(named[i] ?? peerNames[i] ?? '');
			}
			peerNames = next;
		});
	});

	async function submitPeers() {
		if (peersBusy || !plan) return;
		const startIdx = drafts.length;
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

	// ── Arrival: the crossing from draft to asset ─────────────────────────────
	//
	// The preparer's form. Serves the make step ("add marginalia to each") and
	// the delayed peer who finally turns up on their rescheduled row. One peer
	// at a time, in naming order.
	const pendingArrivals = $derived(miPendingArrivals(miData));
	const arrivingDraft = $derived(pendingArrivals[0] ?? null);
	const arrivalsOwed = $derived(pendingArrivals.length);
	const allArrived = $derived(
		(miData.make_pending || isDelayedArrival) && arrivalsOwed === 0,
	);

	let arrivalName = $state('');
	let arrivalMarginalia = $state('');
	let arrivalBusy = $state(false);
	// Re-seed the form each time a different draft comes up for arrival: the
	// name arrives pre-filled from the pre-roll, the marginalia blank. The guard
	// is a plain let, not $state — an effect that reads its own writes
	// re-triggers itself and Svelte aborts the flush.
	let arrivalSeededFor: string | null = null;
	$effect(() => {
		const d = arrivingDraft;
		if (!d || arrivalSeededFor === d.id) return;
		arrivalSeededFor = d.id;
		arrivalName = d.name;
		arrivalMarginalia = '';
	});

	async function submitArrival() {
		if (arrivalBusy || !plan || !arrivingDraft) return;
		if (!arrivalName.trim() || !arrivalMarginalia.trim()) return;
		arrivalBusy = true; resError = '';
		try {
			await introductionsArrival(plan.id, {
				draft_id: arrivingDraft.id,
				name: arrivalName.trim(),
				marginalia: arrivalMarginalia.trim(),
			});
			arrivalSeededFor = null;
			arrivalName = ''; arrivalMarginalia = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not bring them to court.';
		} finally { arrivalBusy = false; }
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
	function outcomeFor(draftID: string) {
		return marOutcomes.find(o => o.draft_id === draftID) ?? null;
	}
	const unresolvedDrafts = $derived(drafts.filter(d => !outcomeFor(d.id)));
	const allPeersDone = $derived(
		drafts.length > 0 && drafts.every(d => outcomeFor(d.id)?.done),
	);
	// Peers whose marginalia this player owes — both the broken_arrival author
	// and the other_retinue recipient, since D6 hands the note to the new owner.
	const myAuthorDrafts = $derived(
		drafts.filter(d => {
			const o = outcomeFor(d.id);
			return o != null && !o.done && o.author_player_id === currentPlayerID;
		}),
	);

	let marOutcome = $state('other_retinue');
	let marTargetPlayer = $state<number | null>(null);
	let marText = $state('');
	let marJourneyText = $state('');
	let marBusy = $state(false);
	const marReady = $derived(
		((marOutcome === 'other_retinue' || marOutcome === 'broken_arrival') && marTargetPlayer != null)
		|| marOutcome === 'delayed'
		|| (marOutcome === 'broken_journey' && !!marText.trim() && !!marJourneyText.trim()),
	);
	async function submitMar(draftID: string) {
		if (marBusy || !plan) return;
		marBusy = true; resError = '';
		try {
			const params: {
				draft_id: string; outcome: string;
				target_player_id?: number; text?: string; journey_text?: string;
			} = { draft_id: draftID, outcome: marOutcome };
			if (marOutcome === 'other_retinue' || marOutcome === 'broken_arrival') {
				params.target_player_id = marTargetPlayer ?? undefined;
			}
			if (marOutcome === 'broken_journey') {
				params.text = marText.trim();
				params.journey_text = marJourneyText.trim();
			}
			await introductionsMar(plan.id, params);
			marOutcome = 'other_retinue'; marTargetPlayer = null;
			marText = ''; marJourneyText = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not resolve peer.';
		} finally { marBusy = false; }
	}

	let authorText = $state('');
	let authorBusy = $state(false);
	async function submitAuthor(draftID: string) {
		if (authorBusy || !plan || !authorText.trim()) return;
		authorBusy = true; resError = '';
		try {
			await introductionsMarginalia(plan.id, draftID, authorText.trim());
			authorText = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not write marginalia.';
		} finally { authorBusy = false; }
	}

	/** How a resolved peer's fate reads back in the recap list. */
	function marOutcomeLabel(o: { outcome: string; done: boolean }): string {
		const label = MI_MAR_OPTIONS.find(x => x.key === o.outcome)?.label ?? o.outcome;
		return o.done ? `${label} ✓` : `${label} (awaiting marginalia)`;
	}
</script>

{#if mode === 'prep'}
	<fieldset class="plan-form-fieldset" disabled={readOnly}>
		<div class="plan-form">
			{#if prepError}<ErrorText message={prepError} variant="panel" />{/if}
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
				<textarea rows={2} bind:value={prepNotes} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
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
		{#if isDelayedArrival}
			{#if arrivingDraft && isPreparer}
				<div class="plan-form">
					<p class="ft-prompt">
						<em>{arrivingDraft.name}</em> has finally reached court. Describe them
						as they arrive.
					</p>
					<AssetCreationForm
						{gameID}
						assetType="peer"
						bind:name={arrivalName}
						bind:marginalia={arrivalMarginalia}
						disabled={arrivalBusy}
						nameLabel="1 · Name"
						marginaliaLabel="2 · First marginalia"
					/>
					<div class="form-actions">
						<button class="action-btn primary" onclick={submitArrival}
							disabled={arrivalBusy || !arrivalName.trim() || !arrivalMarginalia.trim()}>
							{arrivalBusy ? '…' : 'They arrive'}
						</button>
					</div>
				</div>
			{:else if allArrived && isPreparer}
				<div class="complete-section">
					<p class="complete-note">The newcomer has arrived. Complete the plan.</p>
					<button class="action-btn primary" onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				</div>
			{:else}
				<p class="ft-prompt muted">
					{playerName(players, plan.preparer_id)} is welcoming a delayed newcomer…
				</p>
			{/if}

		{:else if needsPeerNaming && isPreparer}
			<div class="mi-naming">
				<p class="form-hint">
					Name each of the {miPeerCountTarget}
					{miPeerCountTarget === 1 ? 'peer' : 'peers'} you're introducing.
					Once you finalize, the dice will roll — you'll describe whoever
					actually turns up.
				</p>
				{#if peersError}<ErrorText message={peersError} variant="panel" />{/if}
				{#each peerNames as _, i (i)}
					{@const locked = drafts[i] != null}
					<div class="form-label">
						<span>Peer {i + 1}:</span>
						{#if locked}
							<input
								type="text"
								class="form-input"
								bind:value={peerNames[i]}
								disabled
							/>
						{:else}
							<SuggestionPicker
								suggestions={peerNameSuggestions}
								bind:value={peerNames[i]}
								loading={peerNameSuggLoading}
								customPlaceholder="Name, title, role…"
								maxlength={TEXT_LIMITS.NAME}
								disabled={peersBusy}
							/>
						{/if}
					</div>
				{/each}
				<div class="form-actions">
					<button class="action-btn primary" onclick={submitPeers} disabled={peersBusy}>
						{peersBusy ? '…' : (drafts.length > 0 ? 'Resume & roll' : 'Name peers & roll')}
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
			{:else if choicesDone && arrivingDraft && isPreparer}
				<div class="plan-form">
					<p class="ft-prompt">
						<em>{arrivingDraft.name}</em> arrives at court. Describe them —
						every newcomer needs a marginalia.
						{#if arrivalsOwed > 1}<span class="muted">({arrivalsOwed} left)</span>{/if}
					</p>
					<AssetCreationForm
						{gameID}
						assetType="peer"
						bind:name={arrivalName}
						bind:marginalia={arrivalMarginalia}
						disabled={arrivalBusy}
						nameLabel="1 · Name"
						marginaliaLabel="2 · First marginalia"
					/>
					<div class="form-actions">
						<button class="action-btn primary" onclick={submitArrival}
							disabled={arrivalBusy || !arrivalName.trim() || !arrivalMarginalia.trim()}>
							{arrivalBusy ? '…' : 'They arrive'}
						</button>
					</div>
				</div>
			{:else if choicesDone && isPreparer}
				<div class="complete-section">
					<p class="complete-note">
						Write any follow-scene narration in the chat, then complete the plan.
					</p>
					<button class="action-btn primary" onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				</div>
			{:else}
				<p class="ft-prompt muted">
					{playerName(players, plan.preparer_id)} is welcoming the new peers…
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
					{#each drafts as d (d.id)}
						{@const o = outcomeFor(d.id)}
						{#if o}
							<p class="choices-applied">{d.name} — {marOutcomeLabel(o)}</p>
						{/if}
					{/each}

					{#each myAuthorDrafts as d (d.id)}
						{@const o = outcomeFor(d.id)}
						<div class="plan-form">
							<p class="ft-prompt">
								{#if o?.outcome === 'other_retinue'}
									<em>{d.name}</em> is joining your retinue. Write the marginalia
									that defines them.
								{:else}
									Write the marginalia that defines <em>{d.name}</em>.
								{/if}
							</p>
							<textarea rows={2} class="form-textarea" bind:value={authorText} maxlength={TEXT_LIMITS.MARGINALIA}
								placeholder="Who has arrived at court?"></textarea>
							<button class="action-btn primary" onclick={() => submitAuthor(d.id)}
								disabled={authorBusy || !authorText.trim()}>
								{authorBusy ? '…' : 'Write marginalia'}
							</button>
						</div>
					{/each}

					{#if isPreparer && unresolvedDrafts.length > 0}
						{@const d = unresolvedDrafts[0]}
						<div class="plan-form">
							<p class="ft-prompt">
								Resolve <em>{d.name}</em>
								({unresolvedDrafts.length} remaining):
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
								<p class="form-hint">
									{marOutcome === 'other_retinue'
										? 'They write the marginalia, and the newcomer joins them once they have.'
										: 'They write the marginalia; the newcomer arrives once it is written.'}
								</p>
							{:else if marOutcome === 'delayed'}
								<p class="form-hint">
									A d6 decides how many rows they spend on the road. Past row 13
									and they never arrive at all.
								</p>
							{:else if marOutcome === 'broken_journey'}
								<label class="form-label">
									Who arrived:
									<textarea rows={2} class="form-textarea" bind:value={marText} maxlength={TEXT_LIMITS.MARGINALIA}
										placeholder="A description, a function, a fun fact…"></textarea>
								</label>
								<label class="form-label">
									What the journey cost them (arrives torn):
									<textarea rows={2} class="form-textarea" bind:value={marJourneyText} maxlength={TEXT_LIMITS.MARGINALIA}
										placeholder="The mark of an arduous journey…"></textarea>
								</label>
							{/if}
							<button class="action-btn primary" onclick={() => submitMar(d.id)}
								disabled={marBusy || !marReady}>
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
					{:else if !isPreparer && myAuthorDrafts.length === 0}
						<p class="ft-prompt muted">
							{playerName(players, plan.preparer_id)} is resolving the marred introductions…
						</p>
					{/if}
				</div>
			{/if}
		{/if}
	</ResolvingCard>
{/if}
