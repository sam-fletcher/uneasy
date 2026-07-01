<!-- ExchangeCourtiersPanel.svelte
  Prep + resolve UI for the Exchange Courtiers plan.
  Resolve flow: fair trade → dice roll → make/mar choices → messy break
  (when required) → complete.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import './planPanel.css';
	import {
		preparePlan, fairTrade, makeChoice, messyBreak,
		ecRiposteBreak,
		type Plan, type Asset, type Player, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import MakeMarPicker from './MakeMarPicker.svelte';
	import Buffet, { type BuffetTab } from './shared/Buffet.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import CardPicker from './CardPicker.svelte';
	import {
		MAKE_OPTIONS, MAR_OPTIONS, parseResolutionData,
		playerName, assetName, assetsWithIntactMarginalia, playersExcept, }from './shared';

	import type { PlanPanelProps } from './types';
	import FormField from './FormField.svelte';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	// Read-only "what can happen?" reference (shared Buffet), shown across the
	// whole plan so players can weigh the trade / make / mar consequences before
	// they commit. The level numbers and margin caps mirror the handler's
	// ecMakeLevels/ecMarLevels and ValidateChoices.
	const EC_BUFFET_TABS: BuffetTab[] = [
		{
			key: 'trade', label: 'Trade',
			always: "Before any roll, the target may name one of the preparer's peers to receive in exchange for the targeted peer. If the preparer accepts, the two peers swap with no roll. Otherwise, they roll.",
		},
		{
			key: 'make', label: 'Make',
			always: "You take the targeted peer.",
			intro: "Then choose one, based on how much you beat the difficulty by:",
			opts: [
				{ key: 'messy', label: '+0: Messy', desc: " — They break one of your assets." },
				{ key: 'legal', label: '+1: Legal', desc: " — Everything went according to plan." },
				{ key: 'conspiracy', label: '+2: Conspiracy', desc: " — The peer was in on it from the start." },
			],
		},
		{
			key: 'mar', label: 'Mar',
			intro: "The target chooses one, based on how much you missed the difficulty by:",
			opts: [
				{ key: 'fair_trade', label: '-1: A Fair Trade', desc: " — The offered trade goes through." },
				{ key: 'riposte', label: '-2: Riposte', desc: " — They take the requested asset (you may break it first)." },
				{ key: 'forfeit', label: '-3: Forfeit', desc: " — They take the requested asset." },
			],
		},
	];

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const rollActive = $derived(ctx.rollActive);
	const rollOutcome = $derived(ctx.rollOutcome);
	const activeRoll = $derived(ctx.activeRoll);
	const onRollCreated = $derived(ctx.onRollCreated);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	const readOnly = $derived(ctx.readOnly);
	const prepDraft = $derived(ctx.prepDraft as {
		target_player_id?: number | null;
		target_asset_id?: number | null;
		notes?: string;
	} | null);

	// Demand overlay (Stage 4): if a resolved+made demand targets this plan,
	// the perform_steps winner submits make/mar in place of the preparer.
	let performStepsWinnerID = $state<number | null>(null);
	// The preparer resolves their own plan; the perform_steps demand winner may
	// drive the choice in their place.
	const isPreparer = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const amChoiceActor = $derived(
		isPreparer || (currentPlayerID != null && currentPlayerID === performStepsWinnerID),
	);

	// ── Prep state ────────────────────────────────────────────────────────────

	let ecTargetPlayerID = $state<number | null>(null);
	let ecTargetAssetID = $state<number | null>(null);
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	const otherPlayers = $derived(playersExcept(players, currentPlayerID));
	const ecTargetPlayerAssets = $derived(
		ecTargetPlayerID != null
			? assets.filter(a => a.owner_id === ecTargetPlayerID && a.asset_type === 'peer' && !a.is_destroyed)
			: []
	);

	// Pre-select the first other player so peer cards appear without an extra
	// tap. Switching player clears any peer selection (single target overall).
	// Skip in read-only — the draft sync below is authoritative for viewers.
	$effect(() => {
		if (readOnly) return;
		if (ecTargetPlayerID == null && otherPlayers.length > 0) {
			ecTargetPlayerID = otherPlayers[0].id;
		}
	});

	function selectTargetPlayer(pid: number) {
		if (ecTargetPlayerID === pid) return;
		ecTargetPlayerID = pid;
		ecTargetAssetID = null;
	}

	function toggleTargetPeer(a: Asset) {
		ecTargetAssetID = ecTargetAssetID === a.id ? null : a.id;
	}

	async function submitPrep() {
		if (prepBusy) return;
		if (!prepNotes.trim()) { prepError = 'Preparation notes are required.'; return; }
		prepBusy = true;
		prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'exchange_courtiers',
				target_player_id: ecTargetPlayerID,
				target_asset_id: ecTargetAssetID,
				preparation_notes: prepNotes.trim(),
			});
			ecTargetPlayerID = null;
			ecTargetAssetID = null;
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally {
			prepBusy = false;
		}
	}

	$effect(() => {
		if (!readOnly) return;
		ecTargetPlayerID = prepDraft?.target_player_id ?? null;
		ecTargetAssetID = prepDraft?.target_asset_id ?? null;
		prepNotes = prepDraft?.notes ?? '';
	});
	let emitTimer: ReturnType<typeof setTimeout> | null = null;
	$effect(() => {
		if (readOnly || mode !== 'prep') return;
		void ecTargetPlayerID; void ecTargetAssetID; void prepNotes;
		if (emitTimer) clearTimeout(emitTimer);
		emitTimer = setTimeout(() => {
			emitTimer = null;
			ctx.emitPrepDraft({
				target_player_id: ecTargetPlayerID,
				target_asset_id: ecTargetAssetID,
				notes: prepNotes,
			});
		}, 150);
	});
	onDestroy(() => { if (emitTimer) clearTimeout(emitTimer); });

	// ── Resolve state ─────────────────────────────────────────────────────────

	let resError = $state('');

	let ftOfferedAssetID = $state<number | null>(null);
	let ftOfferBusy = $state(false);
	let ftDecideBusy = $state(false);

	let selectedChoices = $state<string[]>([]);
	let choicesBusy = $state(false);

	// Per-rules option levels (mirror ecMakeLevels/ecMarLevels in the handler).
	// The chosen option's level may not exceed the dice margin; the backend
	// enforces this too, but disabling here keeps the UI honest.
	const EC_MAKE_LEVELS: Record<string, number> = { messy: 0, legal: 1, conspiracy: 2 };
	const EC_MAR_LEVELS: Record<string, number> = { fair_trade: 1, riposte: 2, forfeit: 3 };
	const choiceMargin = $derived.by(() => {
		if (!activeRoll || activeRoll.result == null) return null;
		const diff = activeRoll.adjusted_difficulty ?? activeRoll.difficulty;
		return rollOutcome === 'make' ? activeRoll.result - diff : diff - activeRoll.result;
	});
	const disabledChoiceKeys = $derived.by(() => {
		if (choiceMargin == null) return [];
		const levels = rollOutcome === 'make' ? EC_MAKE_LEVELS : EC_MAR_LEVELS;
		return Object.entries(levels)
			.filter(([, lvl]) => lvl > choiceMargin)
			.map(([key]) => key);
	});
	// Why a locked tier can't be picked: it needs a margin at least its level.
	function choiceLockReason(key: string): string | undefined {
		if (!disabledChoiceKeys.includes(key)) return undefined;
		const lvl = (rollOutcome === 'make' ? EC_MAKE_LEVELS : EC_MAR_LEVELS)[key];
		return `needs ${rollOutcome === 'make' ? '+' : '−'}${lvl}`;
	}

	let messyMarginaliaID = $state<number | null>(null);
	let messyBusy = $state(false);
	let messyError = $state('');

	// EC picks exactly one option (single-select), so a choice replaces any
	// prior selection rather than toggling it on/off.
	function selectChoice(key: string) {
		selectedChoices = [key];
	}

	async function onFTOffer(p: Plan) {
		if (!ftOfferedAssetID || ftOfferBusy) return;
		ftOfferBusy = true; resError = '';
		try {
			await fairTrade(p.id, { action: 'offer', offered_asset_id: ftOfferedAssetID });
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not make fair trade offer.';
		} finally { ftOfferBusy = false; }
	}

	async function onFTAccept(p: Plan) {
		if (ftDecideBusy) return;
		ftDecideBusy = true; resError = '';
		try {
			await fairTrade(p.id, { action: 'accept' });
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not accept fair trade.';
		} finally { ftDecideBusy = false; }
	}

	async function onFTDecline(p: Plan) {
		if (ftDecideBusy) return;
		ftDecideBusy = true; resError = '';
		try {
			const res = await fairTrade(p.id, { action: 'decline' });
			if (res.roll) onRollCreated(res.roll);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not decline fair trade.';
		} finally { ftDecideBusy = false; }
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

	async function onMessyBreak(p: Plan) {
		if (!messyMarginaliaID || messyBusy) return;
		messyBusy = true; messyError = '';
		try {
			await messyBreak(p.id, messyMarginaliaID);
			messyMarginaliaID = null;
			onPlansChanged();
		} catch (e) {
			messyError = e instanceof Error ? e.message : 'Could not complete messy break.';
		} finally { messyBusy = false; }
	}

	// ── Mar: on a riposte the preparer breaks/surrenders the requested peer ────
	let riposteMargID = $state<number | null>(null);
	let riposteBusy = $state(false);
	// marginaliaID null = skip the break and surrender the peer intact.
	async function submitRiposte(p: Plan, marginaliaID: number | null) {
		if (riposteBusy) return;
		riposteBusy = true; resError = '';
		try {
			await ecRiposteBreak(p.id, marginaliaID);
			riposteMargID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not break peer.';
		} finally { riposteBusy = false; }
	}
</script>

{#if mode === 'prep'}
	<fieldset class="plan-form-fieldset" disabled={readOnly}>
		<div class="plan-form">
			{#if prepError}<p class="res-error">{prepError}</p>{/if}

			<FormField label="Target player">
				<PlayerChips
					players={otherPlayers}
					isActive={(p) => ecTargetPlayerID === p.id}
					onSelect={(p) => selectTargetPlayer(p.id)}
					{readOnly}
				/>
			</FormField>

			{#if ecTargetPlayerID != null}
				<CardPicker
					label="Target peer"
					items={ecTargetPlayerAssets}
					{players}
					emptyMessage="This player has no peers to exchange."
					selected={ecTargetAssetID}
					onSelect={(id) => (ecTargetAssetID = id)}
					{readOnly}
				/>
			{/if}

			<label class="form-label">
				Preparation:
				<textarea rows={2} bind:value={prepNotes} class="form-textarea"
					placeholder="How are you planning to take them into your retinue?" required></textarea>
			</label>

			{#if !readOnly}
				<div class="form-actions">
					<button class="action-btn primary" onclick={submitPrep}
						disabled={prepBusy || !ecTargetPlayerID || !ecTargetAssetID || !prepNotes.trim()}>
						{prepBusy ? '…' : 'Prepare Plan'}
					</button>
				</div>
			{/if}
		</div>
	</fieldset>
	<!-- Outside the disabled fieldset so its toggle/tabs stay interactive in
	     read-only viewer mode. -->
	<div class="ec-prep-buffet">
		<Buffet tabs={EC_BUFFET_TABS} />
	</div>

{:else if plan}
	{@const isTarget = plan.target_player_id != null && currentPlayerID === plan.target_player_id}
	{@const rd = parseResolutionData(plan)}
	{@const ec = rd.exchange_courtiers ?? {}}
	{@const ftAssetID = ec.fair_trade_asset_id ?? null}
	{@const ftAccepted = ec.fair_trade_accepted ?? null}
	{@const existingChoices = (rd.make_mar_choices ?? []).map(c => c.option)}
	{@const choicesDone = existingChoices.length > 0}
	{@const choiceActor = rollOutcome === 'mar' ? isTarget : amChoiceActor}
	{@const preparerIntactPeers = assets.filter(a => a.owner_id === plan.preparer_id && a.asset_type === 'peer' && !a.is_destroyed)}
	{@const preparerMarginaliaAssets = assetsWithIntactMarginalia(assets, plan.preparer_id)}

	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		<!-- Reference the full make/mar options only up to the roll. Once an
		     outcome is in, the picker carries those descriptions, so showing the
		     Buffet too would state them twice. -->
		{#if rollOutcome == null}
			<Buffet tabs={EC_BUFFET_TABS} />
		{/if}
		{#if ftAccepted == null && !rollActive}
			{#if ftAssetID == null}
				<!-- No offer yet -->
				{#if isTarget}
					<div class="ft-section">
						<p class="ft-prompt">
							{playerName(players, plan.preparer_id)} wants
							<em>{assetName(assets, plan.target_asset_id)}</em>. You may name
							one of their peers to receive in exchange — a fair trade.
						</p>
						<CardPicker
							label="Peer to receive in trade"
							items={preparerIntactPeers}
							{players}
							emptyMessage="The preparer has no peers to trade."
							selected={ftOfferedAssetID}
							onSelect={(id) => (ftOfferedAssetID = id)}
						/>
						<button class="action-btn primary" onclick={() => onFTOffer(plan)}
							disabled={!ftOfferedAssetID || ftOfferBusy}>
							{ftOfferBusy ? '…' : 'Propose trade'}
						</button>
					</div>
				{:else if isPreparer}
					{#if preparerIntactPeers.length === 0}
						<!-- The target can't name one of the preparer's peers if there are
						     none; let the preparer proceed straight to the roll. -->
						<p class="ft-prompt">
							You have no peers for {playerName(players, plan.target_player_id)} to
							request in trade.
						</p>
						<button class="action-btn secondary" onclick={() => onFTDecline(plan)} disabled={ftDecideBusy}>
							{ftDecideBusy ? '…' : 'Proceed to dice roll'}
						</button>
					{:else}
						<p class="ft-prompt">
							Waiting for {playerName(players, plan.target_player_id)} to propose a fair trade for their peer,
							<em>{assetName(assets, plan.target_asset_id)}</em>.
						</p>
					{/if}
				{/if}
			{:else}
				<!-- Offer has been made. Preparer decides. -->
				{#if isPreparer}
					<div class="ft-section">
						<p class="ft-prompt">
							{playerName(players, plan.target_player_id)} will hand over
							<em>{assetName(assets, plan.target_asset_id)}</em> if you give them
							<em>{assetName(assets, ftAssetID)}</em>.
						</p>
						<div class="ft-actions">
							<button class="action-btn primary" onclick={() => onFTAccept(plan)} disabled={ftDecideBusy}>
								{ftDecideBusy ? '…' : 'Accept — exchange without rolling'}
							</button>
							<button class="action-btn secondary" onclick={() => onFTDecline(plan)} disabled={ftDecideBusy}>
								{ftDecideBusy ? '…' : 'Decline — proceed to dice roll'}
							</button>
						</div>
					</div>
				{:else}
					<p class="ft-prompt">
						You asked for <em>{assetName(assets, ftAssetID)}</em> in exchange.
						Waiting for {playerName(players, plan.preparer_id)}'s decision…
					</p>
				{/if}
			{/if}

		{:else if rollActive && !choicesDone}
			<div class="ec-stakes">
				<ul class="ec-stakes-list">
					<li>
						On a make: {playerName(players, plan.preparer_id)} takes <em>{assetName(assets, plan.target_asset_id)}</em>.
					</li>
					{#if ftAssetID != null}
						<li>
							On a mar: {playerName(players, plan.target_player_id)} takes <em>{assetName(assets, ftAssetID)}</em>.
						</li>
					{/if}
				</ul>
			</div>
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome != null && !choicesDone && choiceActor}
			<MakeMarPicker
				outcome={rollOutcome}
				options={(rollOutcome === 'make' ? MAKE_OPTIONS.exchange_courtiers : MAR_OPTIONS.exchange_courtiers) ?? []}
				selected={selectedChoices}
				busy={choicesBusy}
				single
				disabledKeys={disabledChoiceKeys}
				lockReason={choiceLockReason}
				onToggle={selectChoice}
				onSubmit={() => onApplyChoices(plan, rollOutcome!)}
			>
				{#snippet header()}
					{#if rollOutcome === 'make'}
						<p class="choices-note">
							You beat the difficulty{#if choiceMargin != null} by {choiceMargin}{/if} — the targeted peer
							({assetName(assets, plan.target_asset_id)}) will be transferred to you.
						</p>
					{:else}
						<p class="choices-note">
							{playerName(players, plan.preparer_id)}'s plan was marred {#if choiceMargin != null} by {choiceMargin}{/if} —
							choose how you respond.
						</p>
					{/if}
				{/snippet}
			</MakeMarPicker>

		{:else if choicesDone || (rollOutcome == null && ftAccepted === true)}
			{@const messyRequired = ec.messy_break_required ?? false}
			{@const messyDone = ec.messy_break_done ?? false}
			{@const riposteAllowed = ec.riposte_allowed ?? false}
			{@const riposteResolved = ec.riposte_break_resolved ?? false}
			{@const ripostePending = riposteAllowed && !riposteResolved}
			{@const requestedPeerName = assetName(assets, ftAssetID)}
			{@const requestedPeerMarginalia = preparerMarginaliaAssets.filter(a => a.id === ftAssetID)}

			{#if messyRequired && !messyDone}
				{#if isTarget}
					<div class="messy-break-section">
						<p class="ft-prompt">
							The exchange was messy. You may break one of
							{playerName(players, plan.preparer_id)}'s marginalia
							before this plan completes.
						</p>
						{#if messyError}<p class="res-error">{messyError}</p>{/if}
						<CardPicker
							label="Marginalium to break"
							items={preparerMarginaliaAssets}
							{players}
							emptyMessage="The preparer has no marginalia to break."
							marginaliaMode
							selectedMarginaliaID={messyMarginaliaID}
							onSelectMarginalia={(mID) => (messyMarginaliaID = mID)}
						/>
						<button class="action-btn primary" onclick={() => onMessyBreak(plan)}
							disabled={!messyMarginaliaID || messyBusy}>
							{messyBusy ? '…' : 'Break marginalia'}
						</button>
					</div>
				{:else}
					<p class="ft-prompt muted">
						Waiting for {playerName(players, plan.target_player_id)} to break a marginalia…
					</p>
				{/if}

			{:else if ripostePending}
				{#if isPreparer}
					<div class="messy-break-section">
						<p class="ft-prompt">
							Riposte: {playerName(players, plan.target_player_id)} takes
							your <em>{requestedPeerName}</em>. You may break it first, or surrender it intact.
						</p>
						<CardPicker
							label="Marginalium to break"
							items={requestedPeerMarginalia}
							{players}
							emptyMessage="This peer has no marginalia to break."
							marginaliaMode
							selectedMarginaliaID={riposteMargID}
							onSelectMarginalia={(mID) => (riposteMargID = mID)}
						/>
						<div class="ft-actions">
							<button class="action-btn secondary" onclick={() => submitRiposte(plan, riposteMargID)}
								disabled={!riposteMargID || riposteBusy}>
								{riposteBusy ? '…' : 'Break, then surrender'}
							</button>
							<button class="action-btn secondary" onclick={() => submitRiposte(plan, null)}
								disabled={riposteBusy}>
								{riposteBusy ? '…' : 'Surrender intact'}
							</button>
						</div>
					</div>
				{:else}
					<p class="ft-prompt muted">
						Riposte — waiting for {playerName(players, plan.preparer_id)} to break or
						surrender <em>{requestedPeerName}</em>…
					</p>
				{/if}

			{:else}
				<!-- The make/mar choice (and any messy/riposte sub-step) is the last
				     action: the server resolves the plan automatically once nothing
				     remains, so there's no manual "Complete" step. This note only
				     shows in the brief window before the resolved state arrives. -->
				<p class="ft-prompt muted">Resolving…</p>
			{/if}

		{:else if !isPreparer && !isTarget}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is resolving Exchange Courtiers…
			</p>
		{/if}
	</ResolvingCard>
{/if}

<style>
	/* Space the prep-mode reference off the form above it. */
	.ec-prep-buffet {
		margin-top: 0.75rem;
	}
	/* At-a-glance stakes shown while the roll is open, so players can gauge how
	   much to leverage. */
	.ec-stakes {
		margin: 0.9rem 0;
		font-size: 0.85rem;
	}
	.ec-stakes-list {
		margin: 0.25rem 0 0;
		padding-left: 1.1rem;
	}
	.ec-stakes-list li {
		margin: 0.2rem 0;
	}
</style>
