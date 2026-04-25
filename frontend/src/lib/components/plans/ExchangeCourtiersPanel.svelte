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
	import {
		MAKE_OPTIONS, MAR_OPTIONS, parseResolutionData,
		playerName, assetName, intactMarginalia,
	} from './shared';

	interface Props {
		mode: 'prep' | 'resolve';
		gameID: number;
		assets: Asset[];
		players: Player[];
		currentPlayerID: number | null;
		// Resolve mode
		plan?: Plan | null;
		isFocusPlayer?: boolean;
		rollActive?: boolean;
		rollOutcome?: 'make' | 'mar' | null;
		onRollCreated?: (roll: DiceRoll) => void;
		onPlansChanged?: () => void;
		// Prep mode
		onPlanPrepared?: () => void;
	}

	let {
		mode,
		gameID,
		assets,
		players,
		currentPlayerID,
		plan = null,
		isFocusPlayer = false,
		rollActive = false,
		rollOutcome = null,
		onRollCreated = () => {},
		onPlansChanged = () => {},
		onPlanPrepared = () => {},
	}: Props = $props();

	// ── Prep state ────────────────────────────────────────────────────────────

	let ecTargetPlayerID = $state<number | null>(null);
	let ecTargetAssetID = $state<number | null>(null);
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	const otherPlayers = $derived(players.filter(p => p.id !== currentPlayerID));
	const ecTargetPlayerAssets = $derived(
		ecTargetPlayerID != null
			? assets.filter(a => a.owner_id === ecTargetPlayerID && a.asset_type === 'peer' && !a.is_destroyed)
			: []
	);

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

	let messyAssetID = $state<number | null>(null);
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
			messyAssetID = null;
			onPlansChanged();
		} catch (e) {
			messyError = e instanceof Error ? e.message : 'Could not complete messy break.';
		} finally { messyBusy = false; }
	}
</script>

{#if mode === 'prep'}
	<div class="plan-form">
		{#if prepError}<p class="res-error">{prepError}</p>{/if}

		<label class="form-label">
			Target player:
			<select bind:value={ecTargetPlayerID} class="form-select">
				<option value={null}>— select player —</option>
				{#each otherPlayers as p}
					<option value={p.id}>{p.display_name}</option>
				{/each}
			</select>
		</label>

		{#if ecTargetPlayerID != null}
			<label class="form-label">
				Target peer:
				<select bind:value={ecTargetAssetID} class="form-select">
					<option value={null}>— select peer —</option>
					{#each ecTargetPlayerAssets as a}
						<option value={a.id}>{a.name}</option>
					{/each}
				</select>
			</label>
		{/if}

		<label class="form-label">
			Preparation notes (optional):
			<textarea rows={2} bind:value={prepNotes} class="form-textarea"
				placeholder="Describe your approach, target, or intent…"></textarea>
		</label>

		<button class="action-btn primary" onclick={submitPrep}
			disabled={prepBusy || !ecTargetPlayerID || !ecTargetAssetID}>
			{prepBusy ? '…' : 'Prepare Exchange Courtiers'}
		</button>
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
		{#if ftAccepted == null && !rollActive}
			{#if ftAssetID == null}
				<!-- No offer yet -->
				{#if isTarget}
					<div class="ft-section">
						<p class="ft-prompt">
							<strong>{playerName(players, plan.preparer_id)}</strong> wants one of your peers.
							You may offer a peer as a fair trade.
						</p>
						<label class="ft-label">
							Offer a peer:
							<select bind:value={ftOfferedAssetID} class="ft-select">
								<option value={null}>— choose a peer —</option>
								{#each assets.filter(a => a.owner_id === currentPlayerID && a.asset_type === 'peer' && !a.is_destroyed) as a}
									<option value={a.id}>{a.name}</option>
								{/each}
							</select>
						</label>
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

		{:else if rollOutcome != null && !choicesDone && isFocusPlayer}
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
						<label class="form-label">
							Choose an asset:
							<select bind:value={messyAssetID} class="form-select">
								<option value={null}>— select asset —</option>
								{#each assets.filter(a => a.owner_id === currentPlayerID && !a.is_destroyed && (a.marginalia ?? []).some(m => !m.is_torn)) as a}
									<option value={a.id}>{a.name}</option>
								{/each}
							</select>
						</label>
						{#if messyAssetID != null}
							{@const mList = intactMarginalia(assets, currentPlayerID).filter(m => m.assetID === messyAssetID)}
							<label class="form-label">
								Choose marginalia to break:
								<select bind:value={messyMarginaliaID} class="form-select">
									<option value={null}>— select marginalia —</option>
									{#each mList as m}
										<option value={m.id}>{m.text}</option>
									{/each}
								</select>
							</label>
						{/if}
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
