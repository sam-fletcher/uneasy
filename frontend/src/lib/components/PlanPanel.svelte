<!-- PlanPanel.svelte
  Shown during the main event when a plan is being prepared or resolved.

  Two modes:
  1. PREPARATION — shown inside the focus player's action bar (action step 2).
     Lets the focus player pick a plan type and fill in its details.
  2. RESOLUTION — shown when there is an active 'resolving' plan.
     Walks through: pre-roll narration prompt → fair trade (EC only) →
     dice roll (handled externally by DiceRollPanel) → make/mar choices →
     complete.

  The parent is responsible for showing DiceRollPanel when activeRoll != null.
  This component signals "a roll was created" by calling onRollCreated so the
  parent can set activeRoll.
-->
<script lang="ts">
	import {
		getPlanEligibility, preparePlan, resolvePlan,
		fairTrade, makeChoice, completePlan, messyBreak,
		type Plan, type PlanType, type Asset, type Player,
		type EligiblePlan, type DiceRoll,
	} from '$lib/api';

	interface Props {
		gameID: number;
		currentRow: number;
		/** All plans for the game (used to find a resolving plan). */
		plans: Plan[];
		/** All assets in the game (used for fair-trade peer picker). */
		assets: Asset[];
		players: Player[];
		currentPlayerID: number | null;
		isFocusPlayer: boolean;
		/**
		 * Whether the focus player is allowed to prepare a plan right now.
		 * Should be true only during action step 2 (scene ended, action not yet taken).
		 * Defaults to false so the prep form never shows unexpectedly.
		 */
		prepEnabled?: boolean;
		/** Whether any dice roll is currently active. */
		rollActive: boolean;
		/** Latest roll outcome — set by parent when roll.resolved arrives. */
		rollOutcome: 'make' | 'mar' | null;
		/** Called when this component triggers a plan-linked dice roll. */
		onRollCreated: (roll: DiceRoll) => void;
		/** Called when any plan state changes (resolve, fair trade, choice, messy break, complete). */
		onPlansChanged: () => void;
		/** Called specifically when the focus player prepares a plan (their step-2 action). */
		onPlanPrepared: () => void;
	}

	let {
		gameID,
		currentRow,
		plans,
		assets,
		players,
		currentPlayerID,
		isFocusPlayer,
		prepEnabled = false,
		rollActive,
		rollOutcome,
		onRollCreated,
		onPlansChanged,
		onPlanPrepared,
	}: Props = $props();

	// ── Derived state ─────────────────────────────────────────────────────────

	/** The active resolving plan, if any. */
	const resolvingPlan = $derived(plans.find(p => p.status === 'resolving') ?? null);

	/** Pending plans on the current row (focus player needs to resolve these). */
	const pendingOnRow = $derived(
		plans.filter(p => p.status === 'pending' && p.row_number === currentRow)
	);

	/** True when there are plans on this row that need resolving before regular play. */
	const needsResolution = $derived(resolvingPlan != null || pendingOnRow.length > 0);

	// ── Preparation state ─────────────────────────────────────────────────────

	let eligiblePlans = $state<EligiblePlan[]>([]);
	let eligibilityLoaded = $state(false);
	let eligibilityError = $state('');

	let selectedPlanType = $state<PlanType | null>(null);
	let prepNotes = $state('');
	// EC fields
	let ecTargetPlayerID = $state<number | null>(null);
	let ecTargetAssetID = $state<number | null>(null);
	// MI fields
	let miPeerCount = $state(1);

	let prepBusy = $state(false);
	let prepError = $state('');

	async function loadEligibility() {
		eligibilityError = '';
		try {
			const res = await getPlanEligibility(gameID);
			eligiblePlans = res.eligible;
			eligibilityLoaded = true;
		} catch (e) {
			eligibilityError = e instanceof Error ? e.message : 'Could not load eligibility.';
		}
	}

	// Load eligibility when prep is enabled and there's nothing already resolving.
	$effect(() => {
		if (prepEnabled && isFocusPlayer && !needsResolution && !eligibilityLoaded) {
			loadEligibility();
		}
	});

	// Reset form when row changes.
	$effect(() => {
		if (currentRow) {
			selectedPlanType = null;
			prepNotes = '';
			ecTargetPlayerID = null;
			ecTargetAssetID = null;
			miPeerCount = 1;
			eligibilityLoaded = false;
			eligiblePlans = [];
		}
	});

	const ecTargetPlayerAssets = $derived(
		ecTargetPlayerID != null
			? assets.filter(a => a.owner_id === ecTargetPlayerID && a.asset_type === 'peer' && !a.is_destroyed)
			: []
	);

	const otherPlayers = $derived(players.filter(p => p.id !== currentPlayerID));

	async function submitPreparePlan() {
		if (!selectedPlanType || prepBusy) return;
		prepBusy = true;
		prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: selectedPlanType,
				target_player_id: selectedPlanType === 'exchange_courtiers' ? ecTargetPlayerID : null,
				target_asset_id: selectedPlanType === 'exchange_courtiers' ? ecTargetAssetID : null,
				peer_count: selectedPlanType === 'make_introductions' ? miPeerCount : undefined,
				preparation_notes: prepNotes.trim() || null,
			});
			// Reset form; parent will update plans via WS.
			selectedPlanType = null;
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally {
			prepBusy = false;
		}
	}

	// ── Resolution state ──────────────────────────────────────────────────────

	let resError = $state('');
	let resBusy = $state(false);

	// Fair trade sub-state (EC only).
	let ftOfferedAssetID = $state<number | null>(null);
	let ftOfferBusy = $state(false);
	let ftDecideBusy = $state(false);

	// Make/mar choices.
	let selectedChoices = $state<string[]>([]);
	let choicesBusy = $state(false);

	// Messy break (EC make + "messy"): target selects a marginalia to tear.
	let messyAssetID = $state<number | null>(null);
	let messyMarginaliaID = $state<number | null>(null);
	let messyBusy = $state(false);
	let messyError = $state('');

	async function onResolve(plan: Plan) {
		if (resBusy) return;
		resBusy = true;
		resError = '';
		try {
			const res = await resolvePlan(plan.id);
			if (res.roll) onRollCreated(res.roll);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not begin resolution.';
		} finally {
			resBusy = false;
		}
	}

	async function onFTOffer(plan: Plan) {
		if (!ftOfferedAssetID || ftOfferBusy) return;
		ftOfferBusy = true;
		resError = '';
		try {
			await fairTrade(plan.id, { action: 'offer', offered_asset_id: ftOfferedAssetID });
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not make fair trade offer.';
		} finally {
			ftOfferBusy = false;
		}
	}

	async function onFTAccept(plan: Plan) {
		if (ftDecideBusy) return;
		ftDecideBusy = true;
		resError = '';
		try {
			await fairTrade(plan.id, { action: 'accept' });
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not accept fair trade.';
		} finally {
			ftDecideBusy = false;
		}
	}

	async function onFTDecline(plan: Plan) {
		if (ftDecideBusy) return;
		ftDecideBusy = true;
		resError = '';
		try {
			const res = await fairTrade(plan.id, { action: 'decline' });
			if (res.roll) onRollCreated(res.roll);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not decline fair trade.';
		} finally {
			ftDecideBusy = false;
		}
	}

	async function onMakeChoice(plan: Plan, result: 'make' | 'mar') {
		if (choicesBusy) return;
		choicesBusy = true;
		resError = '';
		try {
			await makeChoice(plan.id, result, selectedChoices);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not apply choices.';
		} finally {
			choicesBusy = false;
		}
	}

	async function onComplete(plan: Plan) {
		if (resBusy) return;
		resBusy = true;
		resError = '';
		try {
			await completePlan(plan.id);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not complete plan.';
		} finally {
			resBusy = false;
		}
	}

	async function onMessyBreak(plan: Plan) {
		if (!messyMarginaliaID || messyBusy) return;
		messyBusy = true;
		messyError = '';
		try {
			await messyBreak(plan.id, messyMarginaliaID);
			messyMarginaliaID = null;
			messyAssetID = null;
			onPlansChanged();
		} catch (e) {
			messyError = e instanceof Error ? e.message : 'Could not complete messy break.';
		} finally {
			messyBusy = false;
		}
	}

	// ── Helpers ───────────────────────────────────────────────────────────────

	const PLAN_LABELS: Record<string, string> = {
		exchange_courtiers:  'Exchange Courtiers (Power, delay 5)',
		make_introductions:  'Make Introductions (Knowledge, delay 3)',
		spread_propaganda:   'Spread Propaganda (Esteem, delay 3)',
	};

	const PLAN_SHORT: Record<string, string> = {
		exchange_courtiers: 'Exchange Courtiers',
		make_introductions: 'Make Introductions',
		spread_propaganda:  'Spread Propaganda',
	};

	// Make options per plan type.
	const MAKE_OPTIONS: Record<string, Array<{key: string; label: string}>> = {
		exchange_courtiers: [
			{ key: 'messy',      label: '(0) Messy — target may break one of your assets' },
			{ key: 'legal',      label: '(1) Legal — everything went to plan' },
			{ key: 'conspiracy', label: '(2) Conspiracy — the peer was in on it' },
		],
		make_introductions: [
			{ key: 'peers_arrive', label: 'Peers arrive — add marginalia to each new peer' },
		],
		spread_propaganda: [
			{ key: 'create_artifact', label: 'Create an artifact representing the societal shift' },
		],
	};

	// Mar options per plan type.
	const MAR_OPTIONS: Record<string, Array<{key: string; label: string}>> = {
		exchange_courtiers: [
			{ key: 'fair_trade', label: '(1) A Fair Trade — the trade goes through anyway' },
			{ key: 'riposte',    label: '(2) Riposte — target takes your peer (you may break it first)' },
			{ key: 'forfeit',    label: '(3) Forfeit — target takes your peer' },
		],
		make_introductions: [
			{ key: 'other_retinue',     label: "(a) Peer enters another player's retinue" },
			{ key: 'broken_arrival',    label: '(b) Arrives broken — another writes marginalia, then one is torn' },
			{ key: 'other_retinue_2',   label: '(c) Delayed → enters another retinue instead (Phase 2 simplification)' },
			{ key: 'broken_journey',    label: '(d) Arrives broken with an arduous journey' },
		],
		spread_propaganda: [
			{ key: 'give_peer',      label: "(a) A peer leaves your retinue (give to another player)" },
			{ key: 'lay_low',        label: '(b) Keep your head down — next plan cannot involve esteem' },
			{ key: 'break_self',     label: '(c) Word of your laughable ideas gets around — break yourself' },
			{ key: 'counter_prop',   label: "(d) Interfering player describes counter-propaganda in the follow-scene" },
		],
	};

	function toggleChoice(key: string) {
		if (selectedChoices.includes(key)) {
			selectedChoices = selectedChoices.filter(k => k !== key);
		} else {
			selectedChoices = [...selectedChoices, key];
		}
	}

	function parseFTAssetID(plan: Plan): number | null {
		if (!plan.resolution_data) return null;
		try {
			const d = JSON.parse(plan.resolution_data);
			return d.fair_trade_asset_id ?? null;
		} catch { return null; }
	}

	function parseFTAccepted(plan: Plan): boolean | null {
		if (!plan.resolution_data) return null;
		try {
			const d = JSON.parse(plan.resolution_data);
			return d.fair_trade_accepted ?? null;
		} catch { return null; }
	}

	function parseChoices(plan: Plan): string[] {
		if (!plan.resolution_data) return [];
		try {
			const d = JSON.parse(plan.resolution_data);
			return d.choices ?? [];
		} catch { return []; }
	}

	function parseMessyBreakRequired(plan: Plan): boolean {
		if (!plan.resolution_data) return false;
		try {
			return JSON.parse(plan.resolution_data).messy_break_required ?? false;
		} catch { return false; }
	}

	function parseMessyBreakDone(plan: Plan): boolean {
		if (!plan.resolution_data) return false;
		try {
			return JSON.parse(plan.resolution_data).messy_break_done ?? false;
		} catch { return false; }
	}

	/** All intact marginalia across all assets the target player is involved in. */
	function intactMarginalia(ownerID: number | null) {
		if (ownerID == null) return [];
		return assets
			.filter(a => a.owner_id === ownerID && !a.is_destroyed)
			.flatMap(a => (a.marginalia ?? [])
				.filter(m => !m.is_torn)
				.map(m => ({ ...m, assetName: a.name, assetID: a.id }))
			);
	}

	function playerName(id: number | null): string {
		if (id == null) return '?';
		return players.find(p => p.id === id)?.display_name ?? '?';
	}

	function assetName(id: number | null): string {
		if (id == null) return '?';
		return assets.find(a => a.id === id)?.name ?? '?';
	}
</script>

<!-- ── Resolution panel ──────────────────────────────────────────────────── -->
{#if resolvingPlan}
	{@const plan = resolvingPlan}
	{@const isEC = plan.plan_type === 'exchange_courtiers'}
	{@const isPreparer = currentPlayerID === plan.preparer_id}
	{@const isTarget = plan.target_player_id != null && currentPlayerID === plan.target_player_id}
	{@const ftAssetID = parseFTAssetID(plan)}
	{@const ftAccepted = parseFTAccepted(plan)}
	{@const existingChoices = parseChoices(plan)}
	{@const choicesDone = existingChoices.length > 0}

	<div class="plan-panel resolving">
		<div class="plan-header">
			<span class="plan-badge resolving-badge">Resolving</span>
			<strong class="plan-title">{PLAN_SHORT[plan.plan_type] ?? plan.plan_type}</strong>
			<span class="plan-preparer">by {playerName(plan.preparer_id)}</span>
		</div>

		{#if plan.preparation_notes}
			<p class="plan-notes">"{plan.preparation_notes}"</p>
		{/if}

		{#if resError}
			<p class="res-error">{resError}</p>
		{/if}

		<!-- ── Fair trade step (EC only) ────────────────────────────────────── -->
		{#if isEC && ftAccepted == null && !rollActive}

			{#if ftAssetID == null}
				<!-- No offer yet. -->
				{#if isTarget}
					<!-- Target player offers a peer. -->
					<div class="ft-section">
						<p class="ft-prompt">
							<strong>{playerName(plan.preparer_id)}</strong> wants one of your peers.
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
					<p class="ft-prompt muted">Waiting for {playerName(plan.target_player_id)} to offer a peer…</p>
					<button class="action-btn secondary" onclick={() => onFTDecline(plan)} disabled={ftDecideBusy}>
						{ftDecideBusy ? '…' : 'Skip — proceed to dice roll'}
					</button>
				{/if}

			{:else}
				<!-- Offer has been made. Preparer decides. -->
				{#if isPreparer}
					<div class="ft-section">
						<p class="ft-prompt">
							{playerName(plan.target_player_id)} offers <strong>{assetName(ftAssetID)}</strong> as a fair trade
							for <strong>{assetName(plan.target_asset_id)}</strong>.
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
						You offered <strong>{assetName(ftAssetID)}</strong>. Waiting for {playerName(plan.preparer_id)}'s decision…
					</p>
				{/if}
			{/if}

		<!-- ── Dice roll in progress ─────────────────────────────────────── -->
		{:else if rollActive && !choicesDone}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		<!-- ── Make/mar choices (after roll resolves, or after FT declined + rolled) ─ -->
		{:else if rollOutcome != null && !choicesDone && isFocusPlayer}
			{@const outcome = rollOutcome}
			{@const optionList = outcome === 'make'
				? (MAKE_OPTIONS[plan.plan_type] ?? [])
				: (MAR_OPTIONS[plan.plan_type] ?? [])}

			<div class="choices-section">
				<p class="choices-header">
					Result: <strong class="outcome-{outcome}">{outcome === 'make' ? '✓ Make' : '✗ Mar'}</strong>
				</p>

				{#if plan.plan_type === 'exchange_courtiers' && outcome === 'make'}
					<p class="choices-note">The targeted peer ({assetName(plan.target_asset_id)}) will be transferred to you.</p>
				{/if}

				{#if optionList.length > 0}
					<p class="choices-note">Select options to apply:</p>
					{#each optionList as opt}
						<label class="choice-item">
							<input type="checkbox"
								checked={selectedChoices.includes(opt.key)}
								onchange={() => toggleChoice(opt.key)}
							/>
							{opt.label}
						</label>
					{/each}
				{/if}

				<button class="action-btn primary"
					onclick={() => onMakeChoice(plan, outcome)}
					disabled={choicesBusy}>
					{choicesBusy ? '…' : 'Apply choices'}
				</button>
			</div>

		<!-- ── Choices applied — messy break check, then complete ──────────── -->
		{:else if choicesDone || (rollOutcome == null && ftAccepted === true)}
			{@const messyRequired = parseMessyBreakRequired(plan)}
			{@const messyDone = parseMessyBreakDone(plan)}

			{#if messyRequired && !messyDone}
				<!-- Messy break: target must tear a marginalia before the plan completes. -->
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
							{@const mList = intactMarginalia(currentPlayerID).filter(m => m.assetID === messyAssetID)}
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
					<p class="ft-prompt muted">Waiting for {playerName(plan.target_player_id)} to break a marginalia…</p>
				{/if}

			{:else if isFocusPlayer}
				<!-- No messy break pending (or already done) — focus player may complete. -->
				<div class="complete-section">
					{#if existingChoices.length > 0}
						<p class="choices-applied">
							Choices applied: {existingChoices.join(', ')}
						</p>
					{/if}
					<p class="complete-note">Write any follow-scene narration in the scene thread, then complete the plan.</p>
					<button class="action-btn primary" onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				</div>
			{/if}

		<!-- ── Non-focus / non-target player view ────────────────────────── -->
		{:else if !isFocusPlayer && !isTarget}
			<p class="ft-prompt muted">
				{playerName(plan.preparer_id)} is resolving {PLAN_SHORT[plan.plan_type] ?? plan.plan_type}…
			</p>
		{/if}
	</div>

<!-- ── Pending plans on current row (need to resolve before regular play) ── -->
{:else if pendingOnRow.length > 0 && isFocusPlayer}
	{@const nextPlan = pendingOnRow[0]}
	<div class="plan-panel pending">
		<div class="plan-header">
			<span class="plan-badge pending-badge">Resolve first</span>
			<strong class="plan-title">{PLAN_SHORT[nextPlan.plan_type] ?? nextPlan.plan_type}</strong>
			<span class="plan-preparer">by {playerName(nextPlan.preparer_id)}</span>
		</div>
		{#if nextPlan.preparation_notes}
			<p class="plan-notes">"{nextPlan.preparation_notes}"</p>
		{/if}
		{#if resError}
			<p class="res-error">{resError}</p>
		{/if}
		<p class="resolve-note">This plan must be resolved before the regular scene.</p>
		<button class="action-btn primary" onclick={() => onResolve(nextPlan)} disabled={resBusy}>
			{resBusy ? '…' : 'Begin resolution'}
		</button>
	</div>

{/if}

<!-- ── Preparation form — only when parent explicitly enables it ─────────── -->
{#if prepEnabled && !needsResolution && isFocusPlayer}
	<div class="prep-section">
		{#if !eligibilityLoaded}
			<p class="muted">Checking eligibility…</p>
		{:else if eligibilityError}
			<p class="res-error">{eligibilityError}</p>
		{:else if eligiblePlans.length === 0}
			<p class="muted">No plans available to prepare this turn.</p>
		{:else}
			<div class="plan-picker">
				<span class="picker-label">Prepare a plan:</span>
				{#each eligiblePlans as ep}
					<button
						class="plan-option-btn"
						class:selected={selectedPlanType === ep.plan_type}
						onclick={() => {
							selectedPlanType = selectedPlanType === ep.plan_type ? null : ep.plan_type;
						}}
					>
						{PLAN_SHORT[ep.plan_type] ?? ep.plan_type}
						<span class="plan-row-hint">→ row {ep.target_row}</span>
					</button>
				{/each}
			</div>
		{/if}

		{#if selectedPlanType}
			<div class="plan-form">
				{#if prepError}
					<p class="res-error">{prepError}</p>
				{/if}

				<!-- Exchange Courtiers: target player + peer -->
				{#if selectedPlanType === 'exchange_courtiers'}
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
				{/if}

				<!-- Make Introductions: peer count -->
				{#if selectedPlanType === 'make_introductions'}
					<label class="form-label">
						Number of peers (1–4):
						<input type="number" min="1" max="4" bind:value={miPeerCount} class="form-num" />
					</label>
					<p class="form-hint">Difficulty will be {2 + miPeerCount}.</p>
				{/if}

				<!-- Optional prep notes -->
				<label class="form-label">
					Preparation notes (optional):
					<textarea rows={2} bind:value={prepNotes} class="form-textarea"
						placeholder="Describe your approach, target, or intent…"></textarea>
				</label>

				<button class="action-btn primary" onclick={submitPreparePlan}
					disabled={prepBusy
						|| (selectedPlanType === 'exchange_courtiers' && (!ecTargetPlayerID || !ecTargetAssetID))
					}>
					{prepBusy ? '…' : `Prepare ${PLAN_SHORT[selectedPlanType] ?? selectedPlanType}`}
				</button>
			</div>
		{/if}
	</div>
{/if}

<style>
	/* ── Shared ──────────────────────────────────────────────────────────────── */

	.plan-panel {
		border-radius: 6px;
		padding: 0.6rem 0.75rem;
		margin-bottom: 0.4rem;
		flex-shrink: 0;
	}

	.plan-panel.resolving {
		background: #1e1a10;
		border: 1px solid #8a6a3a;
	}

	.plan-panel.pending {
		background: #1a1e1a;
		border: 1px solid #4a6a4a;
	}

	.plan-header {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-bottom: 0.4rem;
	}

	.plan-badge {
		font-size: 0.65rem;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		padding: 0.1rem 0.4rem;
		border-radius: 4px;
	}

	.resolving-badge { background: #3a2800; color: #e0a040; border: 1px solid #8a6030; }
	.pending-badge   { background: #1a2a1a; color: #6dbf6d; border: 1px solid #3a6a3a; }

	.plan-title {
		font-size: 0.9rem;
		color: #e8e4d9;
	}

	.plan-preparer {
		font-size: 0.78rem;
		color: #888;
	}

	.plan-notes {
		font-size: 0.82rem;
		color: #aaa;
		font-style: italic;
		margin: 0 0 0.4rem;
	}

	.res-error {
		color: #e07070;
		font-size: 0.82rem;
		margin: 0.25rem 0;
	}

	.muted { color: #666; font-size: 0.82rem; font-style: italic; }

	/* ── Fair trade ──────────────────────────────────────────────────────────── */

	.ft-section, .choices-section, .complete-section {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.ft-prompt {
		font-size: 0.85rem;
		line-height: 1.45;
		margin: 0;
	}

	.ft-label, .form-label {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-size: 0.82rem;
		color: #c8a96e;
	}

	.ft-select, .form-select {
		background: #2a2a2a;
		border: 1px solid #555;
		border-radius: 4px;
		color: inherit;
		font-size: 0.85rem;
		padding: 0.25rem 0.4rem;
	}

	.ft-actions {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	/* ── Choices ─────────────────────────────────────────────────────────────── */

	.choices-header {
		font-size: 0.88rem;
		margin: 0;
	}

	.outcome-make { color: #6dbf7a; }
	.outcome-mar  { color: #e07070; }

	.choices-note {
		font-size: 0.82rem;
		color: #aaa;
		margin: 0;
	}

	.choice-item {
		display: flex;
		align-items: flex-start;
		gap: 0.4rem;
		font-size: 0.82rem;
		color: #ccc;
		cursor: pointer;
		line-height: 1.4;
	}

	.choice-item input { accent-color: #c8a96e; margin-top: 0.15em; flex-shrink: 0; }

	.choices-applied {
		font-size: 0.8rem;
		color: #888;
		margin: 0;
	}

	.complete-note {
		font-size: 0.82rem;
		color: #aaa;
		margin: 0;
	}

	/* ── Messy break ─────────────────────────────────────────────────────────── */

	.messy-break-section {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		padding: 0.5rem;
		background: #1e1010;
		border: 1px solid #6a3030;
		border-radius: 5px;
	}

	.resolve-note {
		font-size: 0.82rem;
		color: #aaa;
		margin: 0 0 0.25rem;
	}

	/* ── Preparation ─────────────────────────────────────────────────────────── */

	.prep-section {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.plan-picker {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		align-items: center;
	}

	.picker-label {
		font-size: 0.8rem;
		color: #c8a96e;
		font-style: italic;
	}

	.plan-option-btn {
		font-size: 0.8rem;
		padding: 0.25rem 0.55rem;
		border-radius: 4px;
		background: #2a2a2a;
		border: 1px solid #555;
		color: #ccc;
		cursor: pointer;
		display: flex;
		align-items: center;
		gap: 0.35rem;
		transition: border-color 0.12s, background 0.12s;
	}

	.plan-option-btn:hover { border-color: #c8a96e; }

	.plan-option-btn.selected {
		background: #2e2510;
		border-color: #c8a96e;
		color: #e8e4d9;
	}

	.plan-row-hint {
		font-size: 0.7rem;
		color: #888;
	}

	.plan-form {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		padding: 0.5rem;
		background: #1e1a10;
		border: 1px solid #4a3a20;
		border-radius: 5px;
	}

	.form-hint {
		font-size: 0.78rem;
		color: #888;
		margin: 0;
	}

	.form-num {
		width: 60px;
		padding: 0.2rem 0.4rem;
		background: #2a2a2a;
		border: 1px solid #555;
		border-radius: 4px;
		color: inherit;
		font-size: 0.85rem;
	}

	.form-textarea {
		background: #2a2a2a;
		border: 1px solid #555;
		border-radius: 4px;
		color: inherit;
		font-family: inherit;
		font-size: 0.85rem;
		padding: 0.3rem 0.5rem;
		resize: none;
	}

	/* ── Shared action button ────────────────────────────────────────────────── */

	.action-btn {
		padding: 0.35rem 0.7rem;
		border-radius: 5px;
		font-size: 0.83rem;
		font-weight: 600;
		cursor: pointer;
		align-self: flex-start;
	}

	.action-btn.primary {
		background: #c8a96e;
		color: #1a1a1a;
	}

	.action-btn.secondary {
		background: #333;
		color: #c8a96e;
		border: 1px solid #4a4030;
	}

	.action-btn:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
