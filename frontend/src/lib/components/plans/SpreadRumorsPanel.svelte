<!-- SpreadRumorsPanel.svelte
  Prep + resolve UI for Spread Rumors (Tier 1, Esteem).

  Prep: target asset (any asset at the table) + the rumor text.
  Resolve: counts picker (repeatable, total = dice result) with options
    break_target / leverage_target / take_asset / hide_source / reveal_source.
    After choices are applied, sub-flows cover break_target (marginalia
    picker on the plan's target asset), take_asset (consent button —
    server transfers plan.target_asset_id), and hide_source (preparer's
    asset picker + secret-text input).

  On make the preparer (focus player) drives the picker. On mar the
  target-asset owner drives the picker instead — they describe a
  counter-rumor about the preparer, and the break_target / take_asset
  sub-flows pick from the preparer's assets.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		breakTarget, requestTakeConsent, respondTakeConsent, hideSource, forfeitSpreadRumorsStep,
		type Plan,
	} from '$lib/api';
	import { parseSpreadRumorsData } from '$lib/plans/resolutionData/spread_rumors';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import CardPicker from './CardPicker.svelte';
	import ChoicesApplied from './ChoicesApplied.svelte';
	import { parseResolutionData, playerName, assetName, assetsWithIntactMarginalia } from './shared';

	import type { PlanPanelProps } from './types';
	import FormField from './FormField.svelte';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const rollActive = $derived(ctx.rollActive);
	const rollOutcome = $derived(ctx.rollOutcome);
	const activeRoll = $derived(ctx.activeRoll);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	const readOnly = $derived(ctx.readOnly);
	const prepDraft = $derived(ctx.prepDraft as {
		filter_owner_id?: number | null;
		target_asset_id?: number | null;
		notes?: string;
		keep_secret?: boolean;
		secret_asset_id?: number | null;
	} | null);

	let performStepsWinnerID = $state<number | null>(null);
	// The preparer resolves their own plan; the perform_steps demand winner may
	// drive the choice in their place.
	const isPreparer = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const amChoiceActor = $derived(
		isPreparer || (currentPlayerID != null && currentPlayerID === performStepsWinnerID),
	);

	const OPTIONS = [
		{ key: 'break_target',    label: 'Break the target asset (tear a marginalia)' },
		{ key: 'leverage_target', label: 'Leverage the target asset' },
		{ key: 'take_asset',      label: 'Take the target asset (requires consent)' },
		{ key: 'hide_source',     label: 'Hide yourself as the source' },
		{ key: 'reveal_source',   label: 'Reveal yourself as the source' },
	] as const;

	// ── Prep ─────────────────────────────────────────────────────────────────
	let prepTargetAssetID = $state<number | null>(null);
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	// "Keep it secret for now": the rumor text is written on the underside of one
	// of the preparer's own assets instead of announced openly. prepSecretAssetID
	// is which own asset holds it.
	let prepKeepSecret = $state(false);
	let prepSecretAssetID = $state<number | null>(null);

	const intactAssets = $derived(assets.filter(a => !a.is_destroyed));

	// The preparer's own intact assets — candidate holders for the secret. In
	// prep mode the focus player drives the form, so currentPlayerID is the
	// preparer (read-only viewers don't pick; they see a summary line instead).
	const ownIntactAssets = $derived(
		intactAssets.filter(a => a.owner_id === currentPlayerID)
	);
	const secretHolderName = $derived(
		prepSecretAssetID == null ? '' : assetName(assets, prepSecretAssetID)
	);

	// Owner filter for the asset picker. Rumours about your own assets are
	// allowed (preserves the dropdown's old behaviour), so every player —
	// including the current one — appears as a chip.
	let prepFilterOwnerID = $state<number | null>(null);
	const ownersWithAssets = $derived(
		players.filter(p => intactAssets.some(a => a.owner_id === p.id))
	);
	const filteredIntactAssets = $derived(
		prepFilterOwnerID == null
			? []
			: intactAssets.filter(a => a.owner_id === prepFilterOwnerID)
	);

	// Pre-select the first owner so cards appear without an extra tap.
	// Skip in read-only — the draft sync below is authoritative for viewers.
	$effect(() => {
		if (readOnly) return;
		if (prepFilterOwnerID == null && ownersWithAssets.length > 0) {
			prepFilterOwnerID = ownersWithAssets[0].id;
		}
	});

	function selectRumorOwner(pid: number) {
		if (prepFilterOwnerID === pid) return;
		prepFilterOwnerID = pid;
		prepTargetAssetID = null;
	}


	async function submitPrep() {
		if (prepBusy) return;
		if (prepTargetAssetID == null) { prepError = 'Pick the asset the rumor is about.'; return; }
		if (!prepNotes.trim()) { prepError = 'Describe the rumor.'; return; }
		if (prepKeepSecret && prepSecretAssetID == null) {
			prepError = 'Pick one of your assets to hide the rumor under.'; return;
		}
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'spread_rumors',
				target_asset_id: prepTargetAssetID,
				preparation_notes: prepNotes.trim(),
				secret_asset_id: prepKeepSecret ? prepSecretAssetID : undefined,
			});
			prepTargetAssetID = null; prepNotes = '';
			prepKeepSecret = false; prepSecretAssetID = null;
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	$effect(() => {
		if (!readOnly) return;
		prepFilterOwnerID = prepDraft?.filter_owner_id ?? null;
		prepTargetAssetID = prepDraft?.target_asset_id ?? null;
		prepNotes = prepDraft?.notes ?? '';
		prepKeepSecret = prepDraft?.keep_secret ?? false;
		prepSecretAssetID = prepDraft?.secret_asset_id ?? null;
	});
	let emitTimer: ReturnType<typeof setTimeout> | null = null;
	$effect(() => {
		if (readOnly || mode !== 'prep') return;
		void prepFilterOwnerID; void prepTargetAssetID; void prepNotes;
		void prepKeepSecret; void prepSecretAssetID;
		if (emitTimer) clearTimeout(emitTimer);
		emitTimer = setTimeout(() => {
			emitTimer = null;
			ctx.emitPrepDraft({
				filter_owner_id: prepFilterOwnerID,
				target_asset_id: prepTargetAssetID,
				notes: prepNotes,
				keep_secret: prepKeepSecret,
				secret_asset_id: prepSecretAssetID,
			});
		}, 150);
	});
	onDestroy(() => { if (emitTimer) clearTimeout(emitTimer); });

	// ── Resolve ──────────────────────────────────────────────────────────────
	let counts = $state<Record<string, number>>({
		break_target: 0, leverage_target: 0, take_asset: 0,
		hide_source: 0, reveal_source: 0,
	});
	let choicesBusy = $state(false);
	let resError = $state('');

	const totalPicked = $derived(Object.values(counts).reduce((a, b) => a + b, 0));

	// Players must pick options equal to the dice result (make) or
	// (difficulty − result) (mar). Compute that exact count so the picker can
	// cap increments and require the full quota before applying.
	const requiredPicks = $derived.by(() => {
		const result = activeRoll?.result;
		if (result == null) return null;
		const difficulty = activeRoll?.adjusted_difficulty ?? activeRoll?.difficulty ?? null;
		if (rollOutcome === 'make') return Math.max(0, result);
		if (rollOutcome === 'mar' && difficulty != null) return Math.max(0, difficulty - result);
		return null;
	});

	function bump(key: string, delta: number) {
		// "Take asset" is locked off once the owner has declined it (no looping).
		if (key === 'take_asset' && takeAssetDenied) return;
		// Don't pick break_target more times than there are marginalia to tear.
		if (delta > 0 && key === 'break_target' && (counts.break_target ?? 0) >= btMarginaliaCap) return;
		// Don't let the running total exceed the dice quota.
		if (delta > 0 && requiredPicks != null && totalPicked >= requiredPicks) return;
		const next = Math.max(0, (counts[key] ?? 0) + delta);
		counts = { ...counts, [key]: next };
	}

	function flatChoices(): string[] {
		const flat: string[] = [];
		for (const opt of OPTIONS) {
			for (let i = 0; i < (counts[opt.key] ?? 0); i++) flat.push(opt.key);
		}
		return flat;
	}

	// Two-step apply: when "take asset" is picked, the aggressor first names the
	// specific assets to take (consent screen), since those can be ANY of the
	// victim's assets. Without a take, choices commit straight away.
	type TakeStep = 'options' | 'assets';
	let takeStep = $state<TakeStep>('options');
	let takeSelected = $state<number[]>([]);

	async function onApplyChoices(p: Plan, outcome: 'make' | 'mar') {
		if (choicesBusy || totalPicked === 0) return;
		if (requiredPicks != null && totalPicked !== requiredPicks) return;
		if ((counts.take_asset ?? 0) > 0) {
			// Advance to the asset-selection screen rather than committing.
			takeSelected = [];
			takeStep = 'assets';
			return;
		}
		choicesBusy = true; resError = '';
		try {
			await makeChoice(p.id, outcome, flatChoices());
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not apply choices.';
		} finally { choicesBusy = false; }
	}

	// Submit the take-asset consent request: the aggressor's full picks plus the
	// chosen victim assets. Nothing commits until the victim agrees.
	async function onRequestConsent(p: Plan, outcome: 'make' | 'mar') {
		if (choicesBusy) return;
		if (takeSelected.length !== (counts.take_asset ?? 0)) return;
		choicesBusy = true; resError = '';
		try {
			await requestTakeConsent(p.id, outcome, flatChoices(), takeSelected);
			takeStep = 'options';
			takeSelected = [];
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not request consent.';
		} finally { choicesBusy = false; }
	}

	// ── Actor routing ────────────────────────────────────────────────────────
	// On make the preparer drives the picker and sub-flows. On mar the
	// target-asset owner does. `isActor` captures whichever applies.
	const targetAssetOwnerID = $derived(
		plan?.target_asset_id != null
			? (assets.find(a => a.id === plan.target_asset_id)?.owner_id ?? null)
			: null
	);
	const isActor = $derived.by(() => {
		if (!plan || currentPlayerID == null) return false;
		if (rollOutcome === 'mar') return currentPlayerID === targetAssetOwnerID;
		// make (or unresolved): preparer drives — or the perform_steps winner
		// of a demand against this plan, when one applies.
		return amChoiceActor;
	});

	// ── Take-asset consent state (from resolution_data) ───────────────────────
	const srData = $derived(parseSpreadRumorsData(plan));
	const pendingConsent = $derived(srData.pending_take_consent ?? null);
	const takeAssetDenied = $derived(srData.take_asset_denied ?? false);
	const isConsentVictim = $derived(
		pendingConsent != null && currentPlayerID === pendingConsent.victim_id,
	);

	// The victim of a take: target-asset owner on make, preparer on mar. The
	// aggressor selects from this player's intact assets on the consent screen.
	const victimID = $derived(
		rollOutcome === 'mar' ? (plan?.preparer_id ?? null) : targetAssetOwnerID,
	);
	const victimAssets = $derived(
		victimID == null ? [] : assets.filter(a => a.owner_id === victimID && !a.is_destroyed),
	);

	// Once the owner declines, blank any take_asset picks so the quota re-fills
	// with other options and the disabled control reads 0.
	$effect(() => {
		if (takeAssetDenied && (counts.take_asset ?? 0) > 0) {
			counts = { ...counts, take_asset: 0 };
		}
	});

	async function onRespondConsent(p: Plan, agree: boolean) {
		if (choicesBusy) return;
		choicesBusy = true; resError = '';
		try {
			await respondTakeConsent(p.id, agree);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not respond.';
		} finally { choicesBusy = false; }
	}

	// ── Sub-flows ─────────────────────────────────────────────────────────────
	// take_asset has no post-commit sub-flow: the transfer happens at consent
	// time, so only break_target and hide_source remain here. Their completion
	// is server-authoritative (resolution_data counters), so a refresh/remount
	// can't re-prompt — and re-submit — a step that already ran.
	const btDone = $derived(srData.break_target_done ?? 0);
	const hsDone = $derived(srData.hide_source_done ?? 0);
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			takeStep = 'options'; takeSelected = [];
		}
	});

	// On mar, break_target / take_asset pick from the preparer's assets; on
	// make they operate on the plan's target asset. btAssetID is only used on
	// mar (to select which preparer asset to tear marginalia on).
	let btAssetID = $state<number | null>(null);
	let btMargID = $state<number | null>(null);
	let btBusy = $state(false);
	const targetAsset = $derived(
		plan?.target_asset_id != null
			? assets.find(a => a.id === plan.target_asset_id) ?? null
			: null
	);
	const preparerAssets = $derived(
		plan ? assets.filter(a => a.owner_id === plan.preparer_id && !a.is_destroyed) : []
	);
	// Candidate assets for the break-target picker.
	//  - make: only the plan's target asset (one card).
	//  - mar:  any of the preparer's intact assets that still have intact
	//          marginalia.
	// AssetCardSelectable's marginalia-pick mode renders the per-line
	// checkboxes; asset identity is derived from the chosen marginalia.
	const btMarginaliaAssets = $derived.by(() => {
		if (rollOutcome === 'mar') {
			return assetsWithIntactMarginalia(preparerAssets);
		}
		return targetAsset ? assetsWithIntactMarginalia([targetAsset]) : [];
	});
	// Each break_target pick tears one marginalia, so the live cap is the total
	// intact marginalia available — picking more would commit a pick with nothing
	// left to tear, which then wedges resolution (CanComplete blocks until every
	// break_target pick is performed or forfeited via sr-forfeit-step).
	const btMarginaliaCap = $derived(
		btMarginaliaAssets.reduce(
			(sum, a) => sum + (a.marginalia ?? []).filter((m) => !m.is_torn).length, 0,
		),
	);
	async function submitBreakTarget(p: Plan) {
		if (btBusy || btMargID == null) return;
		if (rollOutcome === 'mar' && btAssetID == null) return;
		btBusy = true; resError = '';
		try {
			await breakTarget(p.id, btMargID, rollOutcome === 'mar' ? btAssetID! : undefined);
			btMargID = null; btAssetID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not break target.';
		} finally { btBusy = false; }
	}

	// hide_source: actor hides their own role as source on one of their own
	// assets. Actor = preparer on make, target-asset owner on mar.
	let hsAssetID = $state<number | null>(null);
	let hsBusy = $state(false);
	const hsAssetOptions = $derived(
		rollOutcome === 'mar'
			? assets.filter(a => a.owner_id === targetAssetOwnerID && !a.is_destroyed)
			: preparerAssets
	);
	async function submitHideSource(p: Plan) {
		if (hsBusy || hsAssetID == null) return;
		hsBusy = true; resError = '';
		try {
			await hideSource(p.id, hsAssetID);
			hsAssetID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not hide source.';
		} finally { hsBusy = false; }
	}

	// Discharge a depletable step that has remaining picks but no live target —
	// break_target with no intact marginalia, or hide_source with no own asset.
	// The server re-checks no target exists, so this only resolves a genuine
	// dead-end. Without it the plan wedges (completion is now server-gated).
	let forfeitBusy = $state(false);
	async function forfeitStep(p: Plan, step: 'break_target' | 'hide_source') {
		if (forfeitBusy) return;
		forfeitBusy = true; resError = '';
		try {
			await forfeitSpreadRumorsStep(p.id, step);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not skip step.';
		} finally { forfeitBusy = false; }
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

	function countIn(choices: string[], key: string) {
		return choices.filter(c => c === key).length;
	}
</script>

{#if mode === 'prep'}
	<fieldset class="plan-form-fieldset" disabled={readOnly}>
		<div class="plan-form">
			{#if prepError}<p class="res-error">{prepError}</p>{/if}
			<FormField label="Asset owner">
				<PlayerChips
					players={ownersWithAssets}
					isActive={(p) => prepFilterOwnerID === p.id}
					onSelect={(p) => selectRumorOwner(p.id)}
					{readOnly}
				/>
			</FormField>

			{#if prepFilterOwnerID != null}
				<CardPicker
					label="Asset the rumor is about"
					items={filteredIntactAssets}
					{players}
					emptyMessage="This player has no intact assets."
					selected={prepTargetAssetID}
					onSelect={(id) => (prepTargetAssetID = id)}
					{readOnly}
				/>
			{/if}
			<label class="form-label">
				Rumor:
				<textarea rows={3} bind:value={prepNotes} class="form-textarea" maxlength={1000}
					placeholder={prepKeepSecret
						? 'Write the rumor — only you will see it until you spread it.'
						: 'What are people starting to say?'}></textarea>
			</label>

			<label class="sr-secret-toggle">
				<input type="checkbox" bind:checked={prepKeepSecret} disabled={readOnly} />
				<span>
					Keep it secret for now
					<small>Hide the rumor under one of your own assets instead of
						announcing it. Others won't see the text until you spread it.</small>
				</span>
			</label>

			{#if prepKeepSecret}
				{#if readOnly}
					<p class="plan-notes">
						Hidden under: <em>{secretHolderName || '—'}</em>
					</p>
				{:else}
					<CardPicker
						label="Hide the rumor under"
						items={ownIntactAssets}
						{players}
						emptyMessage="You have no intact assets to hide it under."
						selected={prepSecretAssetID}
						onSelect={(id) => (prepSecretAssetID = id)}
						{readOnly}
					/>
				{/if}
			{/if}

			{#if !readOnly}
				<div class="form-actions">
					<button class="action-btn primary" onclick={submitPrep}
						disabled={prepBusy || prepTargetAssetID == null || !prepNotes.trim()
							|| (prepKeepSecret && prepSecretAssetID == null)}>
						{prepBusy ? '…' : 'Prepare Plan'}
					</button>
				</div>
			{/if}
		</div>
	</fieldset>

{:else if plan}
	{@const existingChoices = (parseResolutionData(plan).make_mar_choices ?? []).map(c => c.option)}
	{@const choicesDone = existingChoices.length > 0}
	{@const btNeeded = countIn(existingChoices, 'break_target')}
	{@const hsNeeded = countIn(existingChoices, 'hide_source')}
	{@const btRemaining = Math.max(0, btNeeded - btDone)}
	{@const hsRemaining = Math.max(0, hsNeeded - hsDone)}
	{@const subflowsDone = btRemaining === 0 && hsRemaining === 0}

	<ResolvingCard {plan} {players} error={resError} showNotes={false}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		<div class="rumor-summary">
			{#if plan.preparation_notes}
				<p class="rumor-label">Rumor</p>
				<blockquote class="rumor-quote">{plan.preparation_notes}</blockquote>
			{/if}
			{#if plan.target_asset_id}
				<p class="rumor-target">
					Targeting: <em>{assetName(assets, plan.target_asset_id)}</em>
				</p>
			{/if}
		</div>

		{#if rollActive && !choicesDone}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if pendingConsent != null}
			<!-- A take-asset request is open: everything holds until the victim
			     (the asset owner) agrees or disagrees. -->
			{#if isConsentVictim}
				{@const takeCount = pendingConsent.asset_ids.length}
				{@const breakCount = countIn(pendingConsent.choices, 'break_target')}
				{@const leverageCount = countIn(pendingConsent.choices, 'leverage_target')}
				{@const aboutName = plan.target_asset_id != null ? assetName(assets, plan.target_asset_id) : null}

				<!-- Secondary effects hit the victim's own target asset but need no
				     consent — kept out of the callout as a plain FYI line. -->
				{#if (breakCount > 0 || leverageCount > 0) && aboutName}
					<p class="choices-note">
						No consent on these — {rollOutcome === 'mar' ? 'their counter-rumor' : 'this rumor'}
						also hits <em>{aboutName}</em>:
					</p>
					<ul class="effect-fyi">
						{#if breakCount > 0}<li>Tears a marginalia{#if breakCount > 1} (×{breakCount}){/if}</li>{/if}
						{#if leverageCount > 0}<li>Leverages it{#if leverageCount > 1} (×{leverageCount}){/if}</li>{/if}
					</ul>
				{/if}

				<!-- The callout: the one thing the victim actually decides. -->
				<div class="consent-box">
					<p class="choices-header">Consent needed</p>
					<p class="choices-note">
						{playerName(players, pendingConsent.requested_by)} wants to take
						{takeCount === 1 ? 'this asset' : `these ${takeCount} assets`} from you:
					</p>
					<ul class="consent-assets">
						{#each pendingConsent.asset_ids as aid}
							<li><em>{assetName(assets, aid)}</em></li>
						{/each}
					</ul>
					<div class="form-actions start">
						<button class="action-btn primary"
							onclick={() => onRespondConsent(plan, true)} disabled={choicesBusy}>
							{choicesBusy ? '…' : 'Agree'}
						</button>
						<button class="action-btn secondary"
							onclick={() => onRespondConsent(plan, false)} disabled={choicesBusy}>
							Disagree
						</button>
					</div>
				</div>
			{:else}
				<p class="ft-prompt muted">
					Waiting for {playerName(players, pendingConsent.victim_id)} to agree to give up
					{pendingConsent.asset_ids.length === 1
						? assetName(assets, pendingConsent.asset_ids[0])
						: `${pendingConsent.asset_ids.length} assets`}…
				</p>
			{/if}

		{:else if rollOutcome != null && !choicesDone && isActor && takeStep === 'assets'}
			<!-- Step 2 of a take: name the specific assets to take from the victim. -->
			{@const k = counts.take_asset ?? 0}
			<div class="choices-section">
				<p class="choices-header">Choose {k} asset{k === 1 ? '' : 's'} to take</p>
				<p class="choices-note">
					From {playerName(players, victimID)}'s assets — they'll be asked to consent.
				</p>
				<CardPicker
					label="Assets to take"
					items={victimAssets}
					{players}
					emptyMessage="This player has no assets to take."
					multi
					max={k}
					selectedMulti={takeSelected}
					onSelectMulti={(ids) => (takeSelected = ids)}
				/>
				<p class="choices-note">
					Selected: <strong>{takeSelected.length}</strong> / {k}
				</p>
				<div class="form-actions" style="display:flex;gap:0.5rem;">
					<button class="action-btn primary"
						onclick={() => onRequestConsent(plan, rollOutcome!)}
						disabled={choicesBusy || takeSelected.length !== k}>
						{choicesBusy ? '…' : 'Request consent'}
					</button>
					<button class="action-btn" onclick={() => { takeStep = 'options'; }}>
						Back
					</button>
				</div>
			</div>

		{:else if rollOutcome != null && !choicesDone && isActor}
			<div class="choices-section">
				<p class="choices-header">
					Result: <span class="outcome-{rollOutcome}">
						{rollOutcome === 'make' ? '✓ Make' : '✗ Mar'}
					</span>
				</p>
				<p class="choices-note">
					Pick options equal to your dice result (repeatable){#if requiredPicks != null}: choose <strong>{requiredPicks}</strong>{/if}.
					{#if rollOutcome === 'mar'}
						You're driving the counter-rumor; effects apply to the preparer's assets.
					{/if}
				</p>
				{#if takeAssetDenied}
					<p class="choices-note muted">
						The owner declined the asset transfer — choose other options.
					</p>
				{/if}
				{#each OPTIONS as opt}
					{@const optDisabled = opt.key === 'take_asset' && takeAssetDenied}
					{@const atBreakCap = opt.key === 'break_target' && (counts.break_target ?? 0) >= btMarginaliaCap}
					<div class="stepper-row" class:muted={optDisabled}>
						<button class="action-btn" onclick={() => bump(opt.key, -1)}
							disabled={(counts[opt.key] ?? 0) === 0}>−</button>
						<strong style="min-width:1.5rem;text-align:center;">{counts[opt.key] ?? 0}</strong>
						<button class="action-btn" onclick={() => bump(opt.key, 1)}
							disabled={optDisabled || atBreakCap || (requiredPicks != null && totalPicked >= requiredPicks)}>+</button>
						<span>{opt.label}{#if optDisabled} <em>(declined)</em>{:else if atBreakCap}<span class="choices-note muted"> — no marginalia left</span>{/if}</span>
					</div>
				{/each}
				<p class="choices-note">
					Total picks: <strong>{totalPicked}</strong>{#if requiredPicks != null} / {requiredPicks}{/if}
				</p>
				<button class="action-btn primary"
					onclick={() => onApplyChoices(plan, rollOutcome!)}
					disabled={choicesBusy || totalPicked === 0 || (requiredPicks != null && totalPicked !== requiredPicks)}>
					{choicesBusy ? '…' : ((counts.take_asset ?? 0) > 0 ? 'Next: choose assets' : 'Apply choices')}
				</button>
			</div>

		{:else if choicesDone && isActor}
			<div class="complete-section">
				<ChoicesApplied choices={existingChoices} options={OPTIONS} />

				{#if btRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							{rollOutcome === 'mar' ? 'Break a preparer asset' : 'Break target asset'} ({btRemaining} remaining)
						</p>
						<CardPicker
							label="Marginalium to tear"
							items={btMarginaliaAssets}
							{players}
							emptyMessage="No intact marginalia available."
							marginaliaMode
							selectedMarginaliaID={btMargID}
							onSelectMarginalia={(mID, parent) => {
								btMargID = mID;
								btAssetID = parent?.id ?? null;
							}}
						/>
						{#if btMarginaliaAssets.length === 0}
							<p class="choices-note muted">No intact marginalia left to tear — this pick has no valid target.</p>
							<button class="action-btn primary"
								onclick={() => forfeitStep(plan, 'break_target')} disabled={forfeitBusy}>
								{forfeitBusy ? '…' : 'Skip — no valid targets'}
							</button>
						{:else}
							<button class="action-btn primary"
								onclick={() => submitBreakTarget(plan)}
								disabled={btBusy || btMargID == null}>
								{btBusy ? '…' : 'Tear marginalia'}
							</button>
						{/if}
					</div>
				{/if}

				{#if hsRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							Hide source ({hsRemaining} remaining)
						</p>
						<p class="choices-note muted">
							An asset of your choice will hide the Secret that you spread the rumor.
						</p>
						<CardPicker
							label="Hide on one of your assets"
							items={hsAssetOptions}
							{players}
							selected={hsAssetID}
							onSelect={(id) => (hsAssetID = id)}
						/>
						{#if hsAssetOptions.length === 0}
							<p class="choices-note muted">You have no asset to hide the source under — this pick has no valid target.</p>
							<button class="action-btn primary"
								onclick={() => forfeitStep(plan, 'hide_source')} disabled={forfeitBusy}>
								{forfeitBusy ? '…' : 'Skip — no valid targets'}
							</button>
						{:else}
							<button class="action-btn primary"
								onclick={() => submitHideSource(plan)}
								disabled={hsBusy || hsAssetID == null}>
								{hsBusy ? '…' : 'Hide source'}
							</button>
						{/if}
					</div>
				{/if}

				{#if subflowsDone}
					{#if isPreparer}
						<p class="complete-note">
							Post any follow-scene narration in the chat, then complete.
						</p>
						<button class="action-btn primary"
							onclick={() => onComplete(plan)} disabled={resBusy}>
							{resBusy ? '…' : 'Complete plan'}
						</button>
					{:else}
						<p class="complete-note muted">
							Sub-flows done. {playerName(players, plan.preparer_id)} will complete the plan.
						</p>
					{/if}
				{/if}
			</div>

		{:else if choicesDone && isPreparer}
			<!-- Mar: preparer watches the counter-rumor unfold, then completes. -->
			<div class="complete-section">
				{#if subflowsDone}
					<button class="action-btn primary"
						onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				{:else}
					<p class="ft-prompt muted">
						{#if targetAssetOwnerID != null}
							{playerName(players, targetAssetOwnerID)} is applying the counter-rumor…
						{/if}
					</p>
				{/if}
			</div>

		{:else if !isActor}
			<p class="ft-prompt muted">
				{#if rollOutcome === 'mar' && targetAssetOwnerID != null}
					{playerName(players, targetAssetOwnerID)} is resolving the counter-rumor…
				{:else}
					{playerName(players, plan.preparer_id)} is resolving Spread Rumors…
				{/if}
			</p>
		{/if}
	</ResolvingCard>
{/if}

<style>
	/* The rumor as a pull-quote — in-world prose, set apart from the action UI.
	   A muted left bar distinguishes it from the accent bar on the consent box. */
	.rumor-summary { display: flex; flex-direction: column; gap: 0.3rem; }
	.rumor-label {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--color-text-faint);
		margin: 0;
	}
	.rumor-quote {
		margin: 0;
		padding: 0.1rem 0 0.1rem 0.7rem;
		border-left: 3px solid var(--color-border-strong);
		font-family: var(--font-serif);
		font-style: italic;
		font-size: 0.95rem;
		line-height: 1.4;
		color: var(--color-text);
	}
	.rumor-target {
		font-size: 0.82rem;
		color: var(--color-text-muted);
		margin: 0;
	}

	/* Take-asset consent callout shown to the victim. A single left accent bar
	   marks it as the actionable block — no nested boxes, and the type scale
	   reuses the shared .choices-header / .choices-note sizes. */
	.consent-box {
		border: 1px solid var(--color-border);
		border-left: 3px solid var(--color-accent);
		border-radius: 6px;
		background: var(--color-surface-2);
		padding: 0.6rem 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.consent-assets {
		margin: 0;
		padding-left: 1.2rem;
		font-size: 0.85rem;
	}
	.consent-box .form-actions button { min-height: 44px; }

	/* Secondary (no-consent) effects listed above the callout. */
	.effect-fyi {
		margin: 0;
		padding-left: 1.2rem;
		font-size: 0.82rem;
		color: var(--color-text-muted);
		line-height: 1.4;
	}

	/* Tappable "keep it secret" row: ≥44px target, generous hit area on mobile. */
	.sr-secret-toggle {
		display: flex;
		align-items: flex-start;
		gap: 0.6rem;
		min-height: 44px;
		padding: 0.5rem 0.6rem;
		border: 1px solid var(--color-border, #444);
		border-radius: 6px;
		background: var(--color-surface-2, #2a2a2a);
		cursor: pointer;
	}
	.sr-secret-toggle input[type='checkbox'] {
		width: 1.15rem;
		height: 1.15rem;
		margin-top: 0.15rem;
		flex: none;
		cursor: pointer;
	}
	.sr-secret-toggle span { display: flex; flex-direction: column; gap: 0.15rem; }
	.sr-secret-toggle small { color: var(--color-text-muted, #999); line-height: 1.35; }

	/* Choices-applied list styling */
</style>
