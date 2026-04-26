<!-- ProposeDecreePanel.svelte
  Prep + resolve UI for Propose Decree (Tier 2, Power).

  Prep: notes (the decree text).

  Resolve has three phases:
    1. Council meeting — no dice roll yet. Eligible non-members (Monarch
       or power-ranked above the preparer) see a multi-asset picker to
       join. The current signatory (highest-power council member; the
       preparer by default) sees a "Call the roll" button.
    2. Rolling — dice play out.
    3. Post-roll — signatory writes the addendum; preparer completes.
       On mar, the other council members narrate amendments via scene posts
       (we surface a prompt; the actual posting happens in SceneView).

  The law row is created server-side when make-choice is submitted (with an
  empty choices array — PD has no option picks). On make a resource asset
  representing the law is also created.
-->
<script lang="ts">
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		joinCouncil, callRoll, setAddendum,
		type Plan, type Asset, type Player, type Ranking, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import { playerName, parseResolutionData } from './shared';

	import type { PlanPanelProps } from './types';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const rankings = $derived(ctx.rankings);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const isFocusPlayer = $derived(ctx.isFocusPlayer);
	const rollActive = $derived(ctx.rollActive);
	const rollOutcome = $derived(ctx.rollOutcome);
	const onRollCreated = $derived(ctx.onRollCreated);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	let performStepsWinnerID = $state<number | null>(null);
	const amChoiceActor = $derived(
		isFocusPlayer || (currentPlayerID != null && currentPlayerID === performStepsWinnerID),
	);

	// ── Prep ─────────────────────────────────────────────────────────────────
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	async function submitPrep() {
		if (prepBusy) return;
		if (!prepNotes.trim()) { prepError = 'Write the decree.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'propose_decree',
				preparation_notes: prepNotes.trim(),
			});
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	// ── Resolve: decoded state ───────────────────────────────────────────────
	type PDState = {
		signatoryID: number | null;
		council: number[];
		addendum: string;
		lawID: number | null;
	};
	const pdState = $derived.by<PDState>(() => {
		const rd = parseResolutionData(plan);
		return {
			signatoryID: rd.signatory_id ?? null,
			council: rd.signatory_player_ids ?? [],
			addendum: rd.addendum ?? '',
			lawID: rd.law_id ?? null,
		};
	});

	function powerRank(playerID: number | null): number | null {
		if (playerID == null) return null;
		const r = rankings.find(x => x.category === 'power' && x.player_id === playerID);
		return r?.rank ?? null;
	}

	const preparerRank = $derived(plan ? powerRank(plan.preparer_id) : null);
	const myRank = $derived(powerRank(currentPlayerID));
	const amMember = $derived(
		currentPlayerID != null && pdState.council.includes(currentPlayerID)
	);
	const amSignatory = $derived(
		currentPlayerID != null && pdState.signatoryID === currentPlayerID
	);
	// Eligible to join: Monarch (rank 1) or power-ranked above the preparer,
	// and not already in the council (the preparer is auto-seated).
	const canJoin = $derived.by(() => {
		if (!plan || currentPlayerID == null || amMember) return false;
		if (myRank == null || preparerRank == null) return false;
		return myRank === 1 || myRank < preparerRank;
	});

	const myUnleveragedAssets = $derived(
		currentPlayerID == null
			? []
			: assets.filter(a =>
				a.owner_id === currentPlayerID && !a.is_destroyed && !a.is_leveraged
			)
	);

	// Has the signatory called the roll yet? We treat the presence of the
	// roll (rollActive or rollOutcome set) as the council being closed.
	const councilClosed = $derived(rollActive || rollOutcome != null);
	// The law exists once make-choice has fired. ApplyChoice sets law_id.
	const lawEnacted = $derived(pdState.lawID != null);

	// ── Join council ─────────────────────────────────────────────────────────
	let selectedAssetIDs = $state<number[]>([]);
	let joinBusy = $state(false);
	let resError = $state('');

	function toggleAsset(id: number) {
		selectedAssetIDs = selectedAssetIDs.includes(id)
			? selectedAssetIDs.filter(x => x !== id)
			: [...selectedAssetIDs, id];
	}

	async function submitJoin(p: Plan) {
		if (joinBusy || selectedAssetIDs.length === 0) return;
		joinBusy = true; resError = '';
		try {
			await joinCouncil(p.id, selectedAssetIDs);
			selectedAssetIDs = [];
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not join council.';
		} finally { joinBusy = false; }
	}

	// ── Call the roll ────────────────────────────────────────────────────────
	let callBusy = $state(false);
	async function submitCallRoll(p: Plan) {
		if (callBusy) return;
		callBusy = true; resError = '';
		try {
			const res = await callRoll(p.id);
			if (res.roll) onRollCreated(res.roll);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not call the roll.';
		} finally { callBusy = false; }
	}

	// ── Apply make-choice (no option picks; just creates the law) ────────────
	let applyBusy = $state(false);
	async function applyResult(p: Plan, outcome: 'make' | 'mar') {
		if (applyBusy) return;
		applyBusy = true; resError = '';
		try {
			await makeChoice(p.id, outcome, []);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not enact the law.';
		} finally { applyBusy = false; }
	}

	// ── Addendum ─────────────────────────────────────────────────────────────
	let addendumDraft = $state('');
	let addendumBusy = $state(false);
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			addendumDraft = pdState.addendum;
			selectedAssetIDs = [];
		}
	});
	async function submitAddendum(p: Plan) {
		if (addendumBusy) return;
		addendumBusy = true; resError = '';
		try {
			await setAddendum(p.id, addendumDraft.trim());
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not save addendum.';
		} finally { addendumBusy = false; }
	}

	// ── Complete ─────────────────────────────────────────────────────────────
	let resBusy = $state(false);
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
			Decree:
			<textarea rows={3} bind:value={prepNotes} class="form-textarea"
				placeholder="What law do you propose?"></textarea>
		</label>
		<button class="action-btn primary" onclick={submitPrep} disabled={prepBusy}>
			{prepBusy ? '…' : 'Prepare Propose Decree'}
		</button>
	</div>

{:else if plan}
	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />

		<!-- Council roster (visible to all) ────────────────────────────── -->
		<div class="choices-section">
			<p class="choices-header">
				Council ({pdState.council.length})
				{#if pdState.signatoryID != null}
					· signatory: <strong>{playerName(players, pdState.signatoryID)}</strong>
				{/if}
			</p>
			{#if pdState.council.length === 0}
				<p class="choices-note muted">No one has joined yet.</p>
			{:else}
				<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
					{#each pdState.council as pid}
						<li>
							{playerName(players, pid)}
							{#if pid === pdState.signatoryID}<strong> (signatory)</strong>{/if}
							{#if pid === plan.preparer_id}<span class="muted"> (preparer)</span>{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</div>

		<!-- Council phase: join form + call-roll ──────────────────────── -->
		{#if !councilClosed}
			{#if canJoin}
				<div class="plan-form" style="margin-top:0.5rem;">
					<p class="choices-header">Join the council</p>
					<p class="choices-note">
						Leverage one or more of your assets to join the discussion.
					</p>
					{#if myUnleveragedAssets.length === 0}
						<p class="choices-note muted">You have no un-leveraged assets to offer.</p>
					{:else}
						<div class="choice-list">
							{#each myUnleveragedAssets as a}
								<label class="choice-item" style="display:flex;align-items:center;gap:0.5rem;">
									<input type="checkbox"
										checked={selectedAssetIDs.includes(a.id)}
										onchange={() => toggleAsset(a.id)} />
									<span>{a.name}</span>
								</label>
							{/each}
						</div>
						<button class="action-btn primary"
							onclick={() => submitJoin(plan)}
							disabled={joinBusy || selectedAssetIDs.length === 0}>
							{joinBusy ? '…' : `Join (${selectedAssetIDs.length} asset${selectedAssetIDs.length === 1 ? '' : 's'})`}
						</button>
					{/if}
				</div>
			{:else if !amMember && currentPlayerID != null}
				<p class="choices-note muted" style="margin-top:0.5rem;">
					Only the Monarch or players ranked above the preparer on power
					may join the council.
				</p>
			{/if}

			{#if amSignatory}
				<div class="plan-form" style="margin-top:0.5rem;">
					<p class="choices-note">
						When discussion is done, close the council and call the roll.
					</p>
					<button class="action-btn primary"
						onclick={() => submitCallRoll(plan)}
						disabled={callBusy}>
						{callBusy ? '…' : 'Call the roll'}
					</button>
				</div>
			{/if}

		<!-- Roll in progress ─────────────────────────────────────────── -->
		{:else if rollActive && !lawEnacted}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		<!-- Post-roll: enact the law (no option picks) ──────────────── -->
		{:else if rollOutcome != null && !lawEnacted && amChoiceActor}
			<div class="choices-section">
				<p class="choices-header">
					Result: <strong class="outcome-{rollOutcome}">
						{rollOutcome === 'make' ? '✓ Make' : '✗ Mar'}
					</strong>
				</p>
				<p class="choices-note">
					{#if rollOutcome === 'make'}
						The decree becomes law. A resource asset owned by the signatory
						will be created.
					{:else}
						The decree passes, amended by the council. No resource asset is
						created; other council members should narrate amendments in the scene.
					{/if}
				</p>
				<button class="action-btn primary"
					onclick={() => applyResult(plan, rollOutcome!)}
					disabled={applyBusy}>
					{applyBusy ? '…' : 'Enact'}
				</button>
			</div>

		<!-- After law enacted: addendum + (mar) amendment prompt + complete ─ -->
		{:else if lawEnacted}
			<div class="complete-section">
				<p class="choices-applied">
					Law enacted{#if rollOutcome === 'mar'} (amended){/if}.
				</p>

				{#if rollOutcome === 'mar' && amMember && !amSignatory}
					<p class="choices-note">
						As a council member, post your amendment in the scene thread below.
					</p>
				{/if}

				{#if amSignatory}
					<div class="plan-form">
						<p class="choices-header">Addendum</p>
						<label class="form-label">
							<textarea rows={2} bind:value={addendumDraft} class="form-textarea"
								placeholder="but/and …"></textarea>
						</label>
						<button class="action-btn"
							onclick={() => submitAddendum(plan)}
							disabled={addendumBusy}>
							{addendumBusy ? '…' : 'Save addendum'}
						</button>
					</div>
				{:else if pdState.addendum}
					<p class="plan-notes">
						Addendum: "<em>{pdState.addendum}</em>"
					</p>
				{/if}

				{#if isFocusPlayer}
					<button class="action-btn primary"
						onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				{/if}
			</div>

		{:else if !amChoiceActor && !lawEnacted}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is resolving Propose Decree…
			</p>
		{/if}

	</ResolvingCard>
{/if}
