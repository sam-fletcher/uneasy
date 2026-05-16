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
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		breakTarget, takeRumorAsset, hideSource,
		type Plan, type Asset, type Player, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import AssetCardSelectable from '../AssetCardSelectable.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import { playerColor } from '$lib/playerColor';
	import { parseResolutionData, playerName, assetName, assetsWithIntactMarginalia } from './shared';

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
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	let performStepsWinnerID = $state<number | null>(null);
	const amChoiceActor = $derived(
		isFocusPlayer || (currentPlayerID != null && currentPlayerID === performStepsWinnerID),
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

	const intactAssets = $derived(assets.filter(a => !a.is_destroyed));

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
	$effect(() => {
		if (prepFilterOwnerID == null && ownersWithAssets.length > 0) {
			prepFilterOwnerID = ownersWithAssets[0].id;
		}
	});

	function selectRumorOwner(pid: number) {
		if (prepFilterOwnerID === pid) return;
		prepFilterOwnerID = pid;
		prepTargetAssetID = null;
	}

	function toggleRumorAsset(a: Asset) {
		prepTargetAssetID = prepTargetAssetID === a.id ? null : a.id;
	}

	async function submitPrep() {
		if (prepBusy) return;
		if (prepTargetAssetID == null) { prepError = 'Pick the asset the rumor is about.'; return; }
		if (!prepNotes.trim()) { prepError = 'Describe the rumor.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'spread_rumors',
				target_asset_id: prepTargetAssetID,
				preparation_notes: prepNotes.trim(),
			});
			prepTargetAssetID = null; prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	// ── Resolve ──────────────────────────────────────────────────────────────
	let counts = $state<Record<string, number>>({
		break_target: 0, leverage_target: 0, take_asset: 0,
		hide_source: 0, reveal_source: 0,
	});
	let choicesBusy = $state(false);
	let resError = $state('');

	function bump(key: string, delta: number) {
		const next = Math.max(0, (counts[key] ?? 0) + delta);
		counts = { ...counts, [key]: next };
	}

	const totalPicked = $derived(Object.values(counts).reduce((a, b) => a + b, 0));

	async function onApplyChoices(p: Plan, outcome: 'make' | 'mar') {
		if (choicesBusy || totalPicked === 0) return;
		choicesBusy = true; resError = '';
		try {
			const flat: string[] = [];
			for (const opt of OPTIONS) {
				for (let i = 0; i < (counts[opt.key] ?? 0); i++) flat.push(opt.key);
			}
			await makeChoice(p.id, outcome, flat);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not apply choices.';
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

	// ── Sub-flows (counters reset per plan) ──────────────────────────────────
	let btDone = $state(0);
	let taDone = $state(0);
	let hsDone = $state(0);
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			btDone = 0; taDone = 0; hsDone = 0;
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
	// checkboxes; asset identity is derived from the chosen marginalium.
	const btMarginaliaAssets = $derived.by(() => {
		if (rollOutcome === 'mar') {
			return assetsWithIntactMarginalia(preparerAssets);
		}
		return targetAsset ? assetsWithIntactMarginalia([targetAsset]) : [];
	});
	async function submitBreakTarget(p: Plan) {
		if (btBusy || btMargID == null) return;
		if (rollOutcome === 'mar' && btAssetID == null) return;
		btBusy = true; resError = '';
		try {
			await breakTarget(p.id, btMargID, rollOutcome === 'mar' ? btAssetID! : undefined);
			btDone += 1;
			btMargID = null; btAssetID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not break target.';
		} finally { btBusy = false; }
	}

	// take_asset: on make the server transfers plan.target_asset_id; on mar
	// the actor picks one of the preparer's assets to take.
	let taAssetID = $state<number | null>(null);
	let taBusy = $state(false);
	async function submitTakeAsset(p: Plan) {
		if (taBusy) return;
		if (rollOutcome === 'mar' && taAssetID == null) return;
		taBusy = true; resError = '';
		try {
			await takeRumorAsset(p.id, true, rollOutcome === 'mar' ? taAssetID! : undefined);
			taDone += 1;
			taAssetID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not take asset.';
		} finally { taBusy = false; }
	}

	// hide_source: actor hides their own role as source on one of their own
	// assets. Actor = preparer on make, target-asset owner on mar.
	let hsAssetID = $state<number | null>(null);
	let hsSecret = $state('');
	let hsBusy = $state(false);
	const hsAssetOptions = $derived(
		rollOutcome === 'mar'
			? assets.filter(a => a.owner_id === targetAssetOwnerID && !a.is_destroyed)
			: preparerAssets
	);
	async function submitHideSource(p: Plan) {
		if (hsBusy || hsAssetID == null || !hsSecret.trim()) return;
		hsBusy = true; resError = '';
		try {
			await hideSource(p.id, hsAssetID, hsSecret.trim());
			hsDone += 1;
			hsAssetID = null; hsSecret = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not hide source.';
		} finally { hsBusy = false; }
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
	<div class="plan-form">
		{#if prepError}<p class="res-error">{prepError}</p>{/if}
		<div class="form-label">
			<span class="form-label-text">Asset owner:</span>
			<PlayerChips
				players={ownersWithAssets}
				isActive={(p) => prepFilterOwnerID === p.id}
				onSelect={(p) => selectRumorOwner(p.id)}
			/>
		</div>

		{#if prepFilterOwnerID != null}
			<div class="form-label">
				<span class="form-label-text">Asset the rumor is about:</span>
				{#if filteredIntactAssets.length === 0}
					<p class="muted" style="margin: 0;">This player has no intact assets.</p>
				{:else}
					<div class="peer-cards">
						{#each filteredIntactAssets as a (a.id)}
							<AssetCardSelectable
								asset={a}
								ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
								// ownerLabel={`Owned by ${playerName(players, a.owner_id)}`}
								selectable
								selected={prepTargetAssetID === a.id}
								onToggle={toggleRumorAsset}
							/>
						{/each}
					</div>
				{/if}
			</div>
		{/if}
		<label class="form-label">
			Rumor:
			<textarea rows={3} bind:value={prepNotes} class="form-textarea"
				placeholder="What are people starting to say?"></textarea>
		</label>
		<div style="text-align: center;">
			<button class="action-btn primary" onclick={submitPrep}
				disabled={prepBusy || prepTargetAssetID == null || !prepNotes.trim()}>
				{prepBusy ? '…' : 'Prepare Plan'}
			</button>
		</div>
	</div>

{:else if plan}
	{@const existingChoices = parseResolutionData(plan).choices ?? []}
	{@const choicesDone = existingChoices.length > 0}
	{@const btNeeded = countIn(existingChoices, 'break_target')}
	{@const taNeeded = countIn(existingChoices, 'take_asset')}
	{@const hsNeeded = countIn(existingChoices, 'hide_source')}
	{@const btRemaining = Math.max(0, btNeeded - btDone)}
	{@const taRemaining = Math.max(0, taNeeded - taDone)}
	{@const hsRemaining = Math.max(0, hsNeeded - hsDone)}
	{@const subflowsDone = btRemaining === 0 && taRemaining === 0 && hsRemaining === 0}

	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		{#if plan.target_asset_id}
			<p class="plan-notes">
				Target: <strong>{assetName(assets, plan.target_asset_id)}</strong>
			</p>
		{/if}

		{#if rollActive && !choicesDone}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome != null && !choicesDone && isActor}
			<div class="choices-section">
				<p class="choices-header">
					Result: <strong class="outcome-{rollOutcome}">
						{rollOutcome === 'make' ? '✓ Make' : '✗ Mar'}
					</strong>
				</p>
				<p class="choices-note">
					Pick options equal to your dice result (repeatable).
					{#if rollOutcome === 'mar'}
						You're driving the counter-rumor; effects apply to the preparer's assets.
					{/if}
				</p>
				{#each OPTIONS as opt}
					<div class="choice-item" style="display:flex;align-items:center;gap:0.5rem;">
						<button class="action-btn" onclick={() => bump(opt.key, -1)}
							disabled={(counts[opt.key] ?? 0) === 0}>−</button>
						<strong style="min-width:1.5rem;text-align:center;">{counts[opt.key] ?? 0}</strong>
						<button class="action-btn" onclick={() => bump(opt.key, 1)}>+</button>
						<span>{opt.label}</span>
					</div>
				{/each}
				<p class="choices-note">Total picks: <strong>{totalPicked}</strong></p>
				<button class="action-btn primary"
					onclick={() => onApplyChoices(plan, rollOutcome!)}
					disabled={choicesBusy || totalPicked === 0}>
					{choicesBusy ? '…' : 'Apply choices'}
				</button>
			</div>

		{:else if choicesDone && isActor}
			<div class="complete-section">
				<p class="choices-applied">
					Choices applied:
					{#each OPTIONS as opt}
						{#if countIn(existingChoices, opt.key) > 0}
							<span>{opt.label} × {countIn(existingChoices, opt.key)}; </span>
						{/if}
					{/each}
				</p>

				{#if btRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							{rollOutcome === 'mar' ? 'Break a preparer asset' : 'Break target asset'} ({btRemaining} remaining)
						</p>
						<div class="form-label">
							<span class="form-label-text">Marginalium to tear:</span>
							{#if btMarginaliaAssets.length === 0}
								<p class="choices-note muted">No intact marginalia available.</p>
							{:else}
								<div class="peer-cards">
									{#each btMarginaliaAssets as a (a.id)}
										<AssetCardSelectable
											asset={a}
											ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
											marginaliaSelectable
											selectedMarginaliaID={btMargID}
											onMarginaliaToggle={(mID, parentAsset) => {
												if (btMargID === mID) {
													btMargID = null;
													btAssetID = null;
												} else {
													btMargID = mID;
													btAssetID = parentAsset.id;
												}
											}}
										/>
									{/each}
								</div>
							{/if}
						</div>
						<button class="action-btn primary"
							onclick={() => submitBreakTarget(plan)}
							disabled={btBusy || btMargID == null}>
							{btBusy ? '…' : 'Tear marginalia'}
						</button>
					</div>
				{/if}

				{#if taRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							{rollOutcome === 'mar' ? 'Take a preparer asset' : 'Take target asset'} ({taRemaining} remaining)
						</p>
						{#if rollOutcome === 'mar'}
							<div class="form-label">
								<span class="form-label-text">Preparer's asset:</span>
								<div class="peer-cards">
									{#each preparerAssets as a (a.id)}
										<AssetCardSelectable
											asset={a}
											ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
											selectable
											selected={taAssetID === a.id}
											onToggle={() => (taAssetID = taAssetID === a.id ? null : a.id)}
										/>
									{/each}
								</div>
							</div>
							<p class="choices-note">
								Confirm the preparer has consented to giving up this asset.
							</p>
						{:else}
							<p class="choices-note">
								Confirm the target player has consented to giving up
								<strong>{assetName(assets, plan.target_asset_id)}</strong>.
							</p>
						{/if}
						<button class="action-btn primary"
							onclick={() => submitTakeAsset(plan)}
							disabled={taBusy || (rollOutcome === 'mar' && taAssetID == null)}>
							{taBusy ? '…' : 'Transfer asset'}
						</button>
					</div>
				{/if}

				{#if hsRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							Hide source ({hsRemaining} remaining)
						</p>
						<div class="form-label">
							<span class="form-label-text">Hide on one of your assets:</span>
							{#if hsAssetOptions.length === 0}
								<p class="choices-note muted">No eligible assets.</p>
							{:else}
								<div class="peer-cards">
									{#each hsAssetOptions as a (a.id)}
										<AssetCardSelectable
											asset={a}
											ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
											selectable
											selected={hsAssetID === a.id}
											onToggle={() => (hsAssetID = hsAssetID === a.id ? null : a.id)}
										/>
									{/each}
								</div>
							{/if}
						</div>
						<label class="form-label">
							Secret text:
							<textarea rows={2} bind:value={hsSecret} class="form-textarea"
								placeholder="Write the secret recording that you're the source…"></textarea>
						</label>
						<button class="action-btn primary"
							onclick={() => submitHideSource(plan)}
							disabled={hsBusy || hsAssetID == null || !hsSecret.trim()}>
							{hsBusy ? '…' : 'Hide source'}
						</button>
					</div>
				{/if}

				{#if subflowsDone}
					{#if isFocusPlayer}
						<p class="complete-note">
							Post any follow-scene narration in the scene thread, then complete.
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

		{:else if choicesDone && isFocusPlayer}
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
