<!-- SpreadRumorsPanel.svelte
  Prep + resolve UI for Spread Rumors (Tier 1, Esteem).

  Prep: target asset (any asset at the table) + the rumor text.
  Resolve: counts picker (repeatable, total = dice result) with options
    break_target / leverage_target / take_asset / hide_source / reveal_source.
    After choices are applied, sub-flows cover break_target (marginalia
    picker on the plan's target asset), take_asset (consent button —
    server transfers plan.target_asset_id), and hide_source (preparer's
    asset picker + secret-text input).

  For simplicity the same picker drives make and mar; the backend
  interprets mar as "counter-rumor about the preparer" and applies
  effects against the preparer's own assets. In the physical game the
  target player would drive mar; we follow the existing SeekAnswers
  pattern and let the preparer (focus player) click through.
-->
<script lang="ts">
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		breakTarget, takeRumorAsset, hideSource,
		type Plan, type Asset, type Player, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import { parseChoices, playerName, assetName } from './shared';

	interface Props {
		mode: 'prep' | 'resolve';
		gameID: number;
		assets: Asset[];
		players: Player[];
		currentPlayerID: number | null;
		plan?: Plan | null;
		isFocusPlayer?: boolean;
		rollActive?: boolean;
		rollOutcome?: 'make' | 'mar' | null;
		onRollCreated?: (roll: DiceRoll) => void;
		onPlansChanged?: () => void;
		onPlanPrepared?: () => void;
	}

	let {
		mode, gameID, assets, players,
		plan = null, isFocusPlayer = false,
		rollActive = false, rollOutcome = null,
		onRollCreated: _or = () => {},
		onPlansChanged = () => {},
		onPlanPrepared = () => {},
	}: Props = $props();

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

	// break_target sub-form
	let btMargID = $state<number | null>(null);
	let btBusy = $state(false);
	const targetAsset = $derived(
		plan?.target_asset_id != null
			? assets.find(a => a.id === plan.target_asset_id) ?? null
			: null
	);
	const targetMarginalia = $derived(
		(targetAsset?.marginalia ?? []).filter(m => !m.is_torn)
	);
	async function submitBreakTarget(p: Plan) {
		if (btBusy || btMargID == null) return;
		btBusy = true; resError = '';
		try {
			await breakTarget(p.id, btMargID);
			btDone += 1;
			btMargID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not break target.';
		} finally { btBusy = false; }
	}

	// take_asset sub-form
	let taBusy = $state(false);
	async function submitTakeAsset(p: Plan) {
		if (taBusy) return;
		taBusy = true; resError = '';
		try {
			await takeRumorAsset(p.id, true);
			taDone += 1;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not take asset.';
		} finally { taBusy = false; }
	}

	// hide_source sub-form
	let hsAssetID = $state<number | null>(null);
	let hsSecret = $state('');
	let hsBusy = $state(false);
	const preparerAssets = $derived(
		plan ? assets.filter(a => a.owner_id === plan.preparer_id && !a.is_destroyed) : []
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
		<label class="form-label">
			Asset the rumor is about:
			<select bind:value={prepTargetAssetID} class="form-select">
				<option value={null}>Select an asset…</option>
				{#each intactAssets as a}
					<option value={a.id}>{a.name} (owner: {playerName(players, a.owner_id)})</option>
				{/each}
			</select>
		</label>
		<label class="form-label">
			Rumor:
			<textarea rows={3} bind:value={prepNotes} class="form-textarea"
				placeholder="What are people starting to say?"></textarea>
		</label>
		<button class="action-btn primary" onclick={submitPrep} disabled={prepBusy}>
			{prepBusy ? '…' : 'Prepare Spread Rumors'}
		</button>
	</div>

{:else if plan}
	{@const existingChoices = parseChoices(plan)}
	{@const choicesDone = existingChoices.length > 0}
	{@const btNeeded = countIn(existingChoices, 'break_target')}
	{@const taNeeded = countIn(existingChoices, 'take_asset')}
	{@const hsNeeded = countIn(existingChoices, 'hide_source')}
	{@const btRemaining = Math.max(0, btNeeded - btDone)}
	{@const taRemaining = Math.max(0, taNeeded - taDone)}
	{@const hsRemaining = Math.max(0, hsNeeded - hsDone)}
	{@const subflowsDone = btRemaining === 0 && taRemaining === 0 && hsRemaining === 0}

	<ResolvingCard {plan} {players} error={resError}>
		{#if plan.target_asset_id}
			<p class="plan-notes">
				Target: <strong>{assetName(assets, plan.target_asset_id)}</strong>
			</p>
		{/if}

		{#if rollActive && !choicesDone}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome != null && !choicesDone && isFocusPlayer}
			<div class="choices-section">
				<p class="choices-header">
					Result: <strong class="outcome-{rollOutcome}">
						{rollOutcome === 'make' ? '✓ Make' : '✗ Mar'}
					</strong>
				</p>
				<p class="choices-note">
					Pick options equal to your dice result (repeatable). For mar, effects
					apply against your own assets (the target spreads a counter-rumor).
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

		{:else if choicesDone && isFocusPlayer}
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
							Break target asset ({btRemaining} remaining)
						</p>
						<label class="form-label">
							Marginalia to tear:
							<select bind:value={btMargID} class="form-select">
								<option value={null}>Select a marginalia…</option>
								{#each targetMarginalia as m}
									<option value={m.id}>#{m.position}: {m.text}</option>
								{/each}
							</select>
						</label>
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
							Take target asset ({taRemaining} remaining)
						</p>
						<p class="choices-note">
							Confirm the target player has consented to giving up
							<strong>{assetName(assets, plan.target_asset_id)}</strong>.
						</p>
						<button class="action-btn primary"
							onclick={() => submitTakeAsset(plan)}
							disabled={taBusy}>
							{taBusy ? '…' : 'Transfer asset'}
						</button>
					</div>
				{/if}

				{#if hsRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							Hide source ({hsRemaining} remaining)
						</p>
						<label class="form-label">
							Hide on one of your assets:
							<select bind:value={hsAssetID} class="form-select">
								<option value={null}>Select your asset…</option>
								{#each preparerAssets as a}
									<option value={a.id}>{a.name}</option>
								{/each}
							</select>
						</label>
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
					<p class="complete-note">
						Post any follow-scene narration in the scene thread, then complete.
					</p>
					<button class="action-btn primary"
						onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				{/if}
			</div>

		{:else if !isFocusPlayer}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is resolving Spread Rumors…
			</p>
		{/if}
	</ResolvingCard>
{/if}
