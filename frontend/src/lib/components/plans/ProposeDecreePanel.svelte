<!-- ProposeDecreePanel.svelte
  Prep + resolve UI for Propose Decree (Tier 2, Power).

  Prep: notes (the decree text).

  Resolve has three phases:
    1. Council meeting — no dice roll yet. The preparer finalizes the decree's
       text and opens the debate (posting the proposed law to the chat).
       Eligible players (ranked BELOW the preparer on power) leverage one asset
       to join, or decline; this can happen before or during the debate. The
       signatory (highest-power council member; the preparer by default) can
       close the debate and call the roll only once it's open AND every eligible
       player has decided.
    2. Rolling — dice play out.
    3. Post-roll — signatory writes the addendum; preparer completes.
       On mar, the other council members narrate amendments via scene posts
       (we surface a prompt; the actual posting happens in SceneView).

  The law row is created server-side when make-choice is submitted (with an
  empty choices array — PD has no option picks). On make a resource asset
  representing the law is also created.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		startDebate, joinCouncil, declineCouncil, callRoll, setAddendum, amendDecree, skipAmend, namePlanAsset, getAssetSuggestions,
		type Plan, type Asset, type Player, type Ranking, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import CardPicker from './CardPicker.svelte';
	import SuggestionPicker from '../SuggestionPicker.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import { playerName, parseResolutionData, ownerUnleveragedAssets } from './shared';

	import type { PlanPanelProps } from './types';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const rankings = $derived(ctx.rankings);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const rollActive = $derived(ctx.rollActive);
	const rollOutcome = $derived(ctx.rollOutcome);
	const onRollCreated = $derived(ctx.onRollCreated);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	// Prep-draft mirroring (Layer 2). Non-focus viewers read inputs from
	// the broadcast draft; focus emits a snapshot on every change.
	const readOnly = $derived(ctx.readOnly);
	const prepDraft = $derived(ctx.prepDraft as { notes?: string } | null);

	let performStepsWinnerID = $state<number | null>(null);
	// The preparer resolves their own plan; the perform_steps demand winner may
	// drive the choice in their place.
	const isPreparer = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const amChoiceActor = $derived(
		isPreparer || (currentPlayerID != null && currentPlayerID === performStepsWinnerID),
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

	// Mirror the focus player's draft into local state when read-only,
	// and debounce-broadcast our own snapshot when editing.
	$effect(() => {
		if (!readOnly) return;
		prepNotes = prepDraft?.notes ?? '';
	});
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

	// ── Resolve: decoded state ───────────────────────────────────────────────
	type PDState = {
		signatoryID: number | null;
		council: number[];
		declined: number[];
		debateStarted: boolean;
		addendum: string;
		addendumConnector: string;
		addendumPlaced: boolean;
		amendmentOrder: number[];
		amendedBy: number[];
		lawID: number | null;
		lawText: string;
		resourceAssetID: number | null;
		resourceNamed: boolean;
	};
	const pdState = $derived.by<PDState>(() => {
		const rd = parseResolutionData(plan).propose_decree ?? {};
		return {
			signatoryID: rd.signatory_id ?? null,
			council: rd.signatory_player_ids ?? [],
			declined: rd.declined_player_ids ?? [],
			debateStarted: rd.debate_started ?? false,
			addendum: rd.addendum ?? '',
			addendumConnector: rd.addendum_connector ?? '',
			addendumPlaced: rd.addendum_placed ?? false,
			amendmentOrder: rd.amendment_order ?? [],
			amendedBy: rd.amended_by ?? [],
			lawID: rd.law_id ?? null,
			lawText: rd.law_text ?? '',
			resourceAssetID: rd.resource_asset_id ?? null,
			resourceNamed: rd.resource_named ?? false,
		};
	});

	// Next council member who must amend the marred law (0 / null = none left).
	const nextAmender = $derived.by<number | null>(() => {
		for (const id of pdState.amendmentOrder) {
			if (!pdState.amendedBy.includes(id)) return id;
		}
		return null;
	});
	const amendmentsRemaining = $derived(
		pdState.amendmentOrder.filter(id => !pdState.amendedBy.includes(id)).length,
	);
	const myAmendTurn = $derived(currentPlayerID != null && nextAmender === currentPlayerID);

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
	const amDeclined = $derived(
		currentPlayerID != null && pdState.declined.includes(currentPlayerID)
	);
	// Eligible to decide (join or decline): the "other players" — those ranked
	// BELOW the preparer on power (higher rank number). The Monarch and everyone
	// above the preparer are already auto-seated for free, so they never decide.
	const eligibleToDecide = $derived.by(() => {
		if (!plan || currentPlayerID == null) return false;
		if (myRank == null || preparerRank == null) return false;
		return myRank > preparerRank;
	});
	// Still owes a decision: eligible, and has neither joined nor declined.
	const canDecide = $derived(eligibleToDecide && !amMember && !amDeclined);

	// Eligible players (ranked below the preparer) who have not yet joined or
	// declined — the council can't be closed until this is empty. Auto-seated
	// members (Monarch, higher-power players) are already in council, so
	// excluding the council list keeps them out without needing the monarch here.
	const pendingDeciderIDs = $derived.by<number[]>(() => {
		if (preparerRank == null) return [];
		return players
			.filter(p => {
				const r = powerRank(p.id);
				return r != null && r > preparerRank
					&& !pdState.council.includes(p.id)
					&& !pdState.declined.includes(p.id);
			})
			.map(p => p.id);
	});

	const myUnleveragedAssets = $derived(ownerUnleveragedAssets(assets, currentPlayerID));

	// Has the signatory called the roll yet? We treat the presence of the
	// roll (rollActive or rollOutcome set) as the council being closed.
	const councilClosed = $derived(rollActive || rollOutcome != null);
	// The law exists once make-choice has fired. ApplyChoice sets law_id.
	const lawEnacted = $derived(pdState.lawID != null);
	const debateStarted = $derived(pdState.debateStarted);

	// ── Start the debate (preparer finalizes the text) ────────────────────────
	// A textbox pre-populated with the drafted decree; the preparer edits it as
	// desired, then opens the debate (which posts the proposed law to the chat).
	let debateDraft = $state('');
	let debateDraftSeeded = $state<number | null>(null);
	$effect(() => {
		if (!plan || debateStarted) return;
		if (debateDraftSeeded !== plan.id) {
			debateDraft = pdState.lawText || (plan.preparation_notes ?? '');
			debateDraftSeeded = plan.id;
		}
	});
	let debateBusy = $state(false);
	async function submitStartDebate(p: Plan) {
		if (debateBusy || !debateDraft.trim()) return;
		debateBusy = true; resError = '';
		try {
			await startDebate(p.id, debateDraft.trim());
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not open the debate.';
		} finally { debateBusy = false; }
	}

	// ── Join / decline council ────────────────────────────────────────────────
	// Exactly one asset may be leveraged to take a seat; more can be leveraged
	// once the roll is open, so the picker is capped at one.
	let selectedAssetIDs = $state<number[]>([]);
	let joinBusy = $state(false);
	let resError = $state('');

	async function submitJoin(p: Plan) {
		if (joinBusy || selectedAssetIDs.length !== 1) return;
		joinBusy = true; resError = '';
		try {
			await joinCouncil(p.id, selectedAssetIDs);
			selectedAssetIDs = [];
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not join council.';
		} finally { joinBusy = false; }
	}

	async function submitDecline(p: Plan) {
		if (joinBusy) return;
		joinBusy = true; resError = '';
		try {
			await declineCouncil(p.id);
			selectedAssetIDs = [];
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not decline.';
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
	// On a make the preparer also names the resource the decree creates, in the
	// same step: enact, then name the freshly-created asset. (When a perform_steps
	// demand winner enacts in the preparer's stead they can't name it — the
	// preparer does so afterward via the standalone naming fallback.)
	let applyBusy = $state(false);
	async function applyResult(p: Plan, outcome: 'make' | 'mar') {
		if (applyBusy) return;
		const nameAtEnact = outcome === 'make' && isPreparer;
		if (nameAtEnact && !resourceName.trim()) {
			resError = 'Name the resource your decree creates.';
			return;
		}
		applyBusy = true; resError = '';
		try {
			await makeChoice(p.id, outcome, []);
			if (nameAtEnact) {
				await namePlanAsset(p.id, 'name-resource', resourceName.trim());
				resourceName = '';
			}
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not enact the law.';
		} finally { applyBusy = false; }
	}

	// ── Amend (mar, current amender) ─────────────────────────────────────────
	let amendDraft = $state('');
	let amendBusy = $state(false);
	let amendSeededFor = $state<number | null>(null);
	// Seed the amend editor with the current law body when it becomes my turn.
	$effect(() => {
		if (myAmendTurn && amendSeededFor !== plan?.id) {
			amendDraft = pdState.lawText;
			amendSeededFor = plan?.id ?? null;
		}
	});
	async function submitAmend(p: Plan) {
		if (amendBusy || !amendDraft.trim()) return;
		amendBusy = true; resError = '';
		try {
			await amendDecree(p.id, amendDraft.trim());
			amendSeededFor = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not amend the decree.';
		} finally { amendBusy = false; }
	}
	async function submitSkipAmend(p: Plan) {
		if (amendBusy) return;
		amendBusy = true; resError = '';
		try {
			await skipAmend(p.id);
			amendSeededFor = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not skip your amendment.';
		} finally { amendBusy = false; }
	}

	// ── Addendum ─────────────────────────────────────────────────────────────
	let addendumDraft = $state('');
	let addendumConnector = $state<'and' | 'but'>('and');
	let addendumBusy = $state(false);
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			addendumDraft = pdState.addendum;
			if (pdState.addendumConnector === 'but') addendumConnector = 'but';
			selectedAssetIDs = [];
		}
	});
	async function submitAddendum(p: Plan) {
		if (addendumBusy) return;
		addendumBusy = true; resError = '';
		try {
			const text = addendumDraft.trim();
			await setAddendum(p.id, text, text ? addendumConnector : undefined);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not place addendum.';
		} finally { addendumBusy = false; }
	}

	// ── Name the law's resource asset (preparer) ─────────────────────────────
	// On a make the preparer names the resource inline at the Enact step (see
	// applyResult). The standalone block below is a fallback for the cases that
	// can't name at enact: a perform_steps winner enacted, or the inline naming
	// call failed. It's optional — naming does not gate completion.
	const namingAtEnact = $derived(
		isPreparer && rollOutcome === 'make' && !lawEnacted,
	);
	const needsResourceNaming = $derived(
		isPreparer && pdState.resourceAssetID != null && !pdState.resourceNamed,
	);
	let resourceName = $state('');
	let nameBusy = $state(false);
	// Name suggestions (resource pool), fetched once a naming UI first appears —
	// either the inline Enact-step picker or the standalone fallback.
	let nameSuggestions = $state<string[]>([]);
	let nameSuggLoading = $state(false);
	let nameSuggFetched = false;
	$effect(() => {
		if (!(namingAtEnact || needsResourceNaming) || nameSuggFetched) return;
		nameSuggFetched = true;
		nameSuggLoading = true;
		getAssetSuggestions(gameID, 'resource', 'name')
			.then(res => { nameSuggestions = res.suggestions; })
			.catch(() => { nameSuggestions = []; })
			.finally(() => { nameSuggLoading = false; });
	});
	async function submitResourceName(p: Plan) {
		if (nameBusy || !resourceName.trim()) return;
		nameBusy = true; resError = '';
		try {
			await namePlanAsset(p.id, 'name-resource', resourceName.trim());
			resourceName = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not name the resource.';
		} finally { nameBusy = false; }
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
	<fieldset class="plan-form-fieldset" disabled={readOnly}>
		<div class="plan-form">
			{#if prepError}<p class="res-error">{prepError}</p>{/if}
			<label class="form-label">
				Decree:
				<textarea rows={3} bind:value={prepNotes} class="form-textarea"
					placeholder="What law are you drafting?" required></textarea>
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
	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />

		<!-- Council roster (visible to all) ────────────────────────────── -->
		<div class="choices-section">
			<p class="choices-header">
				Council ({pdState.council.length})
				{#if pdState.signatoryID != null}
					· Signatory: {playerName(players, pdState.signatoryID)}
				{/if}
			</p>
			{#if pdState.council.length === 0}
				<p class="choices-note muted">Something has gone wrong; try refreshing the page.</p>
			{:else}
				<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
					{#each pdState.council as pid}
						<li>
							{playerName(players, pid)}
							{#if pid === pdState.signatoryID}(signatory){/if}
							{#if pid === plan.preparer_id}<span class="muted"> (preparer)</span>{/if}
						</li>
					{/each}
				</ul>
			{/if}
			{#if pdState.declined.length > 0}
				<p class="choices-note muted" style="margin-top:0.35rem;">
					Declined: {pdState.declined.map(id => playerName(players, id)).join(', ')}
				</p>
			{/if}
		</div>

		<!-- Council phase: finalize text → debate → join/decline → call-roll ── -->
		{#if !councilClosed}
			<!-- Step 1: the preparer finalizes the decree and opens the debate. -->
			{#if !debateStarted}
				{#if isPreparer}
					<div class="plan-form" style="margin-top:0.5rem;">
						<p class="choices-header">Finalize your decree</p>
						<p class="choices-note">
							Decide the text of the law you're proposing, then open the debate.
						</p>
						<label class="form-label">
							<textarea rows={4} bind:value={debateDraft} class="form-textarea"
								placeholder="The decree's text…"></textarea>
						</label>
						<button class="action-btn primary"
							onclick={() => submitStartDebate(plan)}
							disabled={debateBusy || !debateDraft.trim()}>
							{debateBusy ? '…' : 'Open the debate'}
						</button>
					</div>
				{:else}
					<p class="choices-note muted" style="margin-top:0.5rem;">
						{playerName(players, plan.preparer_id)} is finalizing the decree…
					</p>
				{/if}
			{:else}
				<!-- Debate open: show the proposed law under discussion. -->
				<p class="choices-header" style="margin-top:1rem;">
					Proposed decree being debated:
				</p>
				<p class="plan-notes">"{pdState.lawText}"</p>
			{/if}

			<!-- Join / decline — available while the council forms (before and
			     during the debate). -->
			{#if canDecide}
				<div class="plan-form" style="margin-top:0.5rem;">
					<p class="choices-header">Join the council?</p>
					<p class="choices-note">
						Leverage one asset to join the debate — it becomes a die you can use
						during the roll to help or interfere.
					</p>
					<CardPicker
						label="Leverage one asset to join"
						items={myUnleveragedAssets}
						{players}
						emptyMessage="You have no un-leveraged assets to offer — you can only decline."
						multi
						max={1}
						selectedMulti={selectedAssetIDs}
						onSelectMulti={(ids) => (selectedAssetIDs = ids)}
					/>
					<div class="form-actions">
						{#if myUnleveragedAssets.length > 0}
							<button class="action-btn primary"
								onclick={() => submitJoin(plan)}
								disabled={joinBusy || selectedAssetIDs.length !== 1}>
								{joinBusy ? '…' : 'Join'}
							</button>
						{/if}
						<button class="action-btn secondary"
							onclick={() => submitDecline(plan)}
							disabled={joinBusy}>
							{joinBusy ? '…' : 'Decline'}
						</button>
					</div>
				</div>
			{:else if amDeclined}
				<p class="choices-note muted" style="margin-top:0.5rem;">
					You declined to join the council.
				</p>
			{:else if !amMember && currentPlayerID != null}
				<p class="choices-note muted" style="margin-top:0.5rem;">
					The Monarch and members with higher power than the decree proposer
					are already seated.
				</p>
			{/if}

			<!-- Step 4: the signatory closes the debate (only once it's open and
			     every eligible player has decided). -->
			{#if amSignatory && debateStarted}
				<div class="plan-form" style="margin-top:0.5rem;">
					{#if pendingDeciderIDs.length > 0}
						<p class="choices-note muted">
							Waiting on {pendingDeciderIDs.map(id => playerName(players, id)).join(', ')}
							to decide if they want to join the council.
						</p>
					{:else}
						<p class="choices-note">
							The council members are final. Discuss the proposed decree in the chat.
							When the discussion is done, call for the roll.
						</p>
					{/if}
					<button class="action-btn primary"
						onclick={() => submitCallRoll(plan)}
						disabled={callBusy || pendingDeciderIDs.length > 0}>
						{callBusy ? '…' : 'End the debate'}
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
						The decree becomes law. Create a resource representing what you gain from the law.
					{:else}
						The decree passes, amended by the council.
					{/if}
				</p>
				{#if namingAtEnact}
					<p class="form-label-text">
						Name your new resource:
					</p>
					<SuggestionPicker
						suggestions={nameSuggestions}
						bind:value={resourceName}
						loading={nameSuggLoading}
						customPlaceholder="Name the resource resulting from the law…"
						maxlength={120}
					/>
				{/if}
				<button class="action-btn primary"
					onclick={() => applyResult(plan, rollOutcome!)}
					disabled={applyBusy || (namingAtEnact && !resourceName.trim())}>
					{applyBusy ? '…' : rollOutcome === 'make' ? 'Enact law & create resource' : 'Enact'}
				</button>
			</div>

		<!-- After law enacted: (mar) sequential amendments → addendum → complete ─ -->
		{:else if lawEnacted}
			{@const amendmentsDone = amendmentsRemaining === 0}
			<div class="complete-section">
				<p class="choices-applied">
					Law enacted{#if rollOutcome === 'mar'} (being amended){/if}.
				</p>

				<!-- Current law text (live, reflects amendments so far). -->
				<p class="plan-notes">
					<strong>Law:</strong> {pdState.lawText}
					{#if pdState.addendumPlaced && pdState.addendum}
						<em> — {pdState.addendumConnector} {pdState.addendum}</em>
					{/if}
				</p>

				<!-- Mar amendment chain (sequential, lowest power first). -->
				{#if rollOutcome === 'mar' && !amendmentsDone}
					{#if myAmendTurn}
						<div class="plan-form">
							<p class="choices-header">Amend the law (your turn)</p>
							<p class="choices-note">
								Rewrite the law's text. The next council member amends your version.
							</p>
							<label class="form-label">
								<textarea rows={3} bind:value={amendDraft} class="form-textarea"
									placeholder="The amended law…"></textarea>
							</label>
							<div class="form-actions">
								<button class="action-btn primary"
									onclick={() => submitAmend(plan)}
									disabled={amendBusy || !amendDraft.trim()}>
									{amendBusy ? '…' : 'Submit amendment'}
								</button>
								<button class="action-btn secondary"
									onclick={() => submitSkipAmend(plan)}
									disabled={amendBusy}>
									{amendBusy ? '…' : 'Leave unchanged'}
								</button>
							</div>
						</div>
					{:else}
						<p class="choices-note muted">
							The council is amending the law ({amendmentsRemaining} to go).
							{#if nextAmender != null}Next: {playerName(players, nextAmender)}.{/if}
						</p>
					{/if}
				{/if}

				<!-- Signatory's addendum (after amendments; required step). -->
				{#if amendmentsDone && amSignatory && !pdState.addendumPlaced}
					<div class="plan-form">
						<p class="choices-header">Addendum</p>
						<p class="choices-note">
							Attach an optional rider, or place a blank one to proceed.
						</p>
						<div class="chip-row">
							<button type="button" class="chip-btn" class:active={addendumConnector === 'and'}
								onclick={() => (addendumConnector = 'and')}>and</button>
							<button type="button" class="chip-btn" class:active={addendumConnector === 'but'}
								onclick={() => (addendumConnector = 'but')}>but</button>
						</div>
						<label class="form-label">
							<textarea rows={2} bind:value={addendumDraft} class="form-textarea"
								placeholder="…the rider text (optional)"></textarea>
						</label>
						<button class="action-btn primary"
							onclick={() => submitAddendum(plan)}
							disabled={addendumBusy}>
							{addendumBusy ? '…' : 'Place addendum'}
						</button>
					</div>
				{:else if amendmentsDone && !pdState.addendumPlaced && !amSignatory}
					<p class="choices-note muted">
						Waiting for {playerName(players, pdState.signatoryID)} to place the addendum…
					</p>
				{/if}

				{#if needsResourceNaming}
					<div class="plan-form">
						<p class="choices-header">Name the resource your decree created</p>
						<SuggestionPicker
							suggestions={nameSuggestions}
							bind:value={resourceName}
							loading={nameSuggLoading}
							customPlaceholder="Name the law's resource…"
							maxlength={120}
						/>
						<button class="action-btn primary" onclick={() => submitResourceName(plan)}
							disabled={nameBusy || !resourceName.trim()}>
							{nameBusy ? '…' : 'Name resource'}
						</button>
					</div>
				{/if}

				{#if isPreparer}
					<button class="action-btn primary"
						onclick={() => onComplete(plan)}
						disabled={resBusy || !amendmentsDone || !pdState.addendumPlaced}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
					{#if !amendmentsDone || !pdState.addendumPlaced}
						<p class="choices-note muted">
							{#if !amendmentsDone}The council is still amending the law.
							{:else}Waiting for the signatory's addendum.{/if}
						</p>
					{/if}
				{/if}
			</div>

		{:else if !amChoiceActor && !lawEnacted}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is resolving Propose Decree…
			</p>
		{/if}

	</ResolvingCard>
{/if}
