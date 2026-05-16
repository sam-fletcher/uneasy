<!-- ExchangeCourtiersPanel.svelte
  Prep + resolve UI for the Exchange Courtiers plan.
  Resolve flow: fair trade → dice roll → make/mar choices → messy break
  (when required) → complete.
-->
<script lang="ts">
	import './planPanel.css';
	import {
		preparePlan, fairTrade, makeChoice, completePlan, messyBreak,
		type Plan, type Asset, type Player, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import MakeMarPicker from './MakeMarPicker.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import AssetCardSelectable from '../AssetCardSelectable.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import { playerColor } from '$lib/playerColor';
	import {
		MAKE_OPTIONS, MAR_OPTIONS, parseResolutionData,
		playerName, assetName, assetsWithIntactMarginalia,
	} from './shared';

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
	const onRollCreated = $derived(ctx.onRollCreated);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	// Demand overlay (Stage 4): if a resolved+made demand targets this plan,
	// the perform_steps winner submits make/mar in place of the preparer.
	let performStepsWinnerID = $state<number | null>(null);
	const amChoiceActor = $derived(
		isFocusPlayer || (currentPlayerID != null && currentPlayerID === performStepsWinnerID),
	);

	// ── Prep state ────────────────────────────────────────────────────────────

	let ecTargetPlayerID = $state<number | null>(null);
	let ecTargetAssetID = $state<number | null>(null);
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	const otherPlayers = $derived(players.filter(p => p.id !== currentPlayerID));
	// My intact peers — candidates for fair-trade offer in resolve mode.
	const myIntactPeers = $derived(
		assets.filter(a =>
			a.owner_id === currentPlayerID && a.asset_type === 'peer' && !a.is_destroyed,
		),
	);
	// Messy-break candidates: my intact assets with ≥1 untorn marginalium.
	const myAssetsWithMarginalia = $derived(
		assetsWithIntactMarginalia(assets, currentPlayerID),
	);
	const ecTargetPlayerAssets = $derived(
		ecTargetPlayerID != null
			? assets.filter(a => a.owner_id === ecTargetPlayerID && a.asset_type === 'peer' && !a.is_destroyed)
			: []
	);

	// Pre-select the first other player so peer cards appear without an extra
	// tap. Switching player clears any peer selection (single target overall).
	$effect(() => {
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
		prepBusy = true;
		prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'exchange_courtiers',
				target_player_id: ecTargetPlayerID,
				target_asset_id: ecTargetAssetID,
				preparation_notes: prepNotes.trim() || null,
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

	// ── Resolve state ─────────────────────────────────────────────────────────

	let resError = $state('');
	let resBusy = $state(false);

	let ftOfferedAssetID = $state<number | null>(null);
	let ftOfferBusy = $state(false);
	let ftDecideBusy = $state(false);

	let selectedChoices = $state<string[]>([]);
	let choicesBusy = $state(false);

	let messyMarginaliaID = $state<number | null>(null);
	let messyBusy = $state(false);
	let messyError = $state('');

	function toggleChoice(key: string) {
		if (selectedChoices.includes(key)) {
			selectedChoices = selectedChoices.filter(k => k !== key);
		} else {
			selectedChoices = [...selectedChoices, key];
		}
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
</script>

{#if mode === 'prep'}
	<div class="plan-form">
		{#if prepError}<p class="res-error">{prepError}</p>{/if}

		<div class="form-label">
			<span class="form-label-text">Target player:</span>
			<PlayerChips
				players={otherPlayers}
				isActive={(p) => ecTargetPlayerID === p.id}
				onSelect={(p) => selectTargetPlayer(p.id)}
			/>
		</div>

		{#if ecTargetPlayerID != null}
			<div class="form-label">
				<span class="form-label-text">Target peer:</span>
				{#if ecTargetPlayerAssets.length === 0}
					<p class="muted" style="margin: 0;">This player has no peers to exchange.</p>
				{:else}
					<div class="peer-cards">
						{#each ecTargetPlayerAssets as a (a.id)}
							<AssetCardSelectable
								asset={a}
								ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
								// ownerLabel={`Owned by ${playerName(players, a.owner_id)}`}
								selectable
								selected={ecTargetAssetID === a.id}
								onToggle={toggleTargetPeer}
							/>
						{/each}
					</div>
				{/if}
			</div>
		{/if}

		<label class="form-label">
			Preparation:
			<textarea rows={2} bind:value={prepNotes} class="form-textarea"
				placeholder="How are you planning to take them into your retinue?"></textarea>
		</label>

		<div style="text-align: center;">
			<button class="action-btn primary" onclick={submitPrep}
				disabled={prepBusy || !ecTargetPlayerID || !ecTargetAssetID}>
				{prepBusy ? '…' : 'Prepare Plan'}
			</button>
		</div>
	</div>

{:else if plan}
	{@const isPreparer = currentPlayerID === plan.preparer_id}
	{@const isTarget = plan.target_player_id != null && currentPlayerID === plan.target_player_id}
	{@const rd = parseResolutionData(plan)}
	{@const ftAssetID = rd.fair_trade_asset_id ?? null}
	{@const ftAccepted = rd.fair_trade_accepted ?? null}
	{@const existingChoices = rd.choices ?? []}
	{@const choicesDone = existingChoices.length > 0}

	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		{#if ftAccepted == null && !rollActive}
			{#if ftAssetID == null}
				<!-- No offer yet -->
				{#if isTarget}
					<div class="ft-section">
						<p class="ft-prompt">
							<strong>{playerName(players, plan.preparer_id)}</strong> wants one of your peers.
							You may offer a peer as a fair trade.
						</p>
						<div class="form-label">
							<span class="form-label-text">Offer a peer:</span>
							{#if myIntactPeers.length === 0}
								<p class="choices-note muted">You have no peers to offer.</p>
							{:else}
								<div class="peer-cards">
									{#each myIntactPeers as a (a.id)}
										<AssetCardSelectable
											asset={a}
											ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
											selectable
											selected={ftOfferedAssetID === a.id}
											onToggle={() => (ftOfferedAssetID = ftOfferedAssetID === a.id ? null : a.id)}
										/>
									{/each}
								</div>
							{/if}
						</div>
						<button class="action-btn primary" onclick={() => onFTOffer(plan)}
							disabled={!ftOfferedAssetID || ftOfferBusy}>
							{ftOfferBusy ? '…' : 'Offer peer'}
						</button>
					</div>
				{:else if isPreparer}
					<p class="ft-prompt muted">
						Waiting for {playerName(players, plan.target_player_id)} to offer a peer…
					</p>
					<button class="action-btn secondary" onclick={() => onFTDecline(plan)} disabled={ftDecideBusy}>
						{ftDecideBusy ? '…' : 'Skip — proceed to dice roll'}
					</button>
				{/if}
			{:else}
				<!-- Offer has been made. Preparer decides. -->
				{#if isPreparer}
					<div class="ft-section">
						<p class="ft-prompt">
							{playerName(players, plan.target_player_id)} offers
							<strong>{assetName(assets, ftAssetID)}</strong> as a fair trade
							for <strong>{assetName(assets, plan.target_asset_id)}</strong>.
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
					<p class="ft-prompt muted">
						You offered <strong>{assetName(assets, ftAssetID)}</strong>.
						Waiting for {playerName(players, plan.preparer_id)}'s decision…
					</p>
				{/if}
			{/if}

		{:else if rollActive && !choicesDone}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome != null && !choicesDone && amChoiceActor}
			<MakeMarPicker
				outcome={rollOutcome}
				options={(rollOutcome === 'make' ? MAKE_OPTIONS.exchange_courtiers : MAR_OPTIONS.exchange_courtiers) ?? []}
				selected={selectedChoices}
				busy={choicesBusy}
				onToggle={toggleChoice}
				onSubmit={() => onApplyChoices(plan, rollOutcome!)}
			>
				{#snippet header()}
					{#if rollOutcome === 'make'}
						<p class="choices-note">
							The targeted peer ({assetName(assets, plan.target_asset_id)}) will be transferred to you.
						</p>
					{/if}
				{/snippet}
			</MakeMarPicker>

		{:else if choicesDone || (rollOutcome == null && ftAccepted === true)}
			{@const messyRequired = rd.messy_break_required ?? false}
			{@const messyDone = rd.messy_break_done ?? false}

			{#if messyRequired && !messyDone}
				{#if isTarget}
					<div class="messy-break-section">
						<p class="ft-prompt">
							The exchange was messy. You must break one of your own marginalia before this plan completes.
						</p>
						{#if messyError}<p class="res-error">{messyError}</p>{/if}
						<div class="form-label">
							<span class="form-label-text">Marginalium to break:</span>
							{#if myAssetsWithMarginalia.length === 0}
								<p class="choices-note muted">You have no intact marginalia.</p>
							{:else}
								<div class="peer-cards">
									{#each myAssetsWithMarginalia as a (a.id)}
										<AssetCardSelectable
											asset={a}
											ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
											marginaliaSelectable
											selectedMarginaliaID={messyMarginaliaID}
											onMarginaliaToggle={(mID) => (messyMarginaliaID = messyMarginaliaID === mID ? null : mID)}
										/>
									{/each}
								</div>
							{/if}
						</div>
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

			{:else if isFocusPlayer}
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
			{/if}

		{:else if !isFocusPlayer && !isTarget}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is resolving Exchange Courtiers…
			</p>
		{/if}
	</ResolvingCard>
{/if}
