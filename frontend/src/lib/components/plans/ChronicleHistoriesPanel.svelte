<!-- ChronicleHistoriesPanel.svelte
  Prep + resolve UI for Chronicle Histories (Tier 1, Knowledge).

  Prep: notes only (the historical problem).

  Resolve:
    - While resolving, the preparer can invoke artifacts at any time via
      invokeArtifact. Non-preparers see the running list of invoked
      artifacts.
    - Make (preparer): counts picker over [break_artifact, invoke_another,
      echo_present, total_control] (repeatable, total = dice result).
      After apply: sub-flow for break_artifact (marginalia on an invoked
      artifact). invoke_another is a scene_post + invokeArtifact followup
      we reuse the main invoke picker for.
    - Mar (all players): every non-preparer gets a single-choice picker
      and submits via marChoice. The preparer sees the list of received
      mar choices (entries in resolution_data.make_mar_choices with player_id set).

  Pre-roll artifact-invocation is gated by resolution_data.invoke_phase_closed,
  which the server flips true inside OnResolve (before the roll is created).
  The pre-roll invoke picker is hidden once that flag is set; post-roll
  "invoke another" invocations go through the mar-choice route instead.
-->
<script lang="ts">
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		invokeArtifact, breakArtifact, marChoice,
		type Plan, type Asset, type Player, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import CardPicker from './CardPicker.svelte';
	import { parseResolutionData, playerName, assetsWithIntactMarginalia } from './shared';
	import type { PlanPanelProps } from './types';
	import FormField from './FormField.svelte';

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
		{ key: 'break_artifact',  label: 'Break an invoked artifact (tear a marginalia)' },
		{ key: 'invoke_another',  label: 'Invoke another artifact and introduce it' },
		{ key: 'echo_present',    label: "Cut to the present to show history's echo" },
		{ key: 'total_control',   label: 'Take total narrative control (requires consent)' },
	] as const;

	// ── Prep ─────────────────────────────────────────────────────────────────
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	async function submitPrep() {
		if (prepBusy) return;
		if (!prepNotes.trim()) { prepError = 'Describe the historical problem.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'chronicle_histories',
				preparation_notes: prepNotes.trim(),
			});
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	// ── Resolve: invoked-artifact tracking ───────────────────────────────────
	// The server stores invoked_artifact_ids in resolution_data. We recompute
	// from the plan on every change.
	const resolutionState = $derived.by<{ ids: number[]; closed: boolean }>(() => {
		const ch = parseResolutionData(plan).chronicle_histories ?? {};
		return {
			ids: ch.invoked_artifact_ids ?? [],
			closed: ch.invoke_phase_closed ?? false,
		};
	});
	const invokedIDs = $derived(resolutionState.ids);
	const invokePhaseClosed = $derived(resolutionState.closed);
	const invokedArtifacts = $derived(
		invokedIDs
			.map(id => assets.find(a => a.id === id))
			.filter((a): a is Asset => a != null)
	);
	const allArtifacts = $derived(
		assets.filter(a => a.asset_type === 'artifact' && !a.is_destroyed)
	);
	const uninvokedArtifacts = $derived(
		allArtifacts.filter(a => !invokedIDs.includes(a.id))
	);

	let invokeAssetID = $state<number | null>(null);
	let invokeBusy = $state(false);
	async function submitInvoke(p: Plan) {
		if (invokeBusy || invokeAssetID == null) return;
		invokeBusy = true; resError = '';
		try {
			await invokeArtifact(p.id, invokeAssetID);
			invokeAssetID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not invoke artifact.';
		} finally { invokeBusy = false; }
	}

	// ── Make picker ──────────────────────────────────────────────────────────
	let counts = $state<Record<string, number>>({
		break_artifact: 0, invoke_another: 0, echo_present: 0, total_control: 0,
	});
	let choicesBusy = $state(false);
	let resError = $state('');

	function bump(key: string, delta: number) {
		const next = Math.max(0, (counts[key] ?? 0) + delta);
		counts = { ...counts, [key]: next };
	}
	const totalPicked = $derived(Object.values(counts).reduce((a, b) => a + b, 0));

	async function onApplyMake(p: Plan) {
		if (choicesBusy || totalPicked === 0) return;
		choicesBusy = true; resError = '';
		try {
			const flat: string[] = [];
			for (const opt of OPTIONS) {
				for (let i = 0; i < (counts[opt.key] ?? 0); i++) flat.push(opt.key);
			}
			await makeChoice(p.id, 'make', flat);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not apply choices.';
		} finally { choicesBusy = false; }
	}

	// ── Mar picker (per-player single choice) ────────────────────────────────
	let marSelected = $state<string>('');
	let marAssetID = $state<number | null>(null);
	let marBusy = $state(false);
	async function submitMarChoice(p: Plan) {
		if (marBusy || !marSelected) return;
		marBusy = true; resError = '';
		try {
			const needsAsset = marSelected === 'break_artifact' || marSelected === 'invoke_another';
			await marChoice(p.id, marSelected, needsAsset && marAssetID != null ? marAssetID : undefined);
			marSelected = ''; marAssetID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not submit mar choice.';
		} finally { marBusy = false; }
	}

	// Decoded mar-choice entries. Both make and mar store into make_mar_choices;
	// mar entries are the ones with player_id set (written by Chronicle's own
	// handler), while make entries leave player_id null.
	type MarEntry = { playerID: number; choice: string };
	const marEntries = $derived.by<MarEntry[]>(() => {
		if (!plan) return [];
		return (parseResolutionData(plan).make_mar_choices ?? [])
			.filter(c => c.player_id != null)
			.map(c => ({ playerID: c.player_id as number, choice: c.option }));
	});
	const myMarSubmitted = $derived(
		currentPlayerID != null && marEntries.some(e => e.playerID === currentPlayerID)
	);

	// ── break_artifact sub-flow (make) ───────────────────────────────────────
	let baDone = $state(0);
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			baDone = 0;
			counts = { break_artifact: 0, invoke_another: 0, echo_present: 0, total_control: 0 };
		}
	});
	let baAssetID = $state<number | null>(null);
	let baMargID = $state<number | null>(null);
	let baBusy = $state(false);
	// Invoked artifacts that still have intact marginalia — candidates for
	// the break_artifact sub-flow's marginalia-pick mode.
	const baArtifactsWithMarginalia = $derived(
		assetsWithIntactMarginalia(invokedArtifacts),
	);
	async function submitBreakArtifact(p: Plan) {
		if (baBusy || baAssetID == null || baMargID == null) return;
		baBusy = true; resError = '';
		try {
			await breakArtifact(p.id, baAssetID, baMargID);
			baDone += 1;
			baAssetID = null; baMargID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not break artifact.';
		} finally { baBusy = false; }
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
	// Make-path choices are the entries with player_id null; mar entries
	// (player_id set) are tracked separately via marEntries.
	const makeChoices = $derived(
		plan
			? (parseResolutionData(plan).make_mar_choices ?? [])
				.filter(c => c.player_id == null)
				.map(c => c.option)
			: []
	);
</script>

{#if mode === 'prep'}
	<div class="plan-form">
		{#if prepError}<p class="res-error">{prepError}</p>{/if}
		<label class="form-label">
			Area of study:
			<textarea rows={3} bind:value={prepNotes} class="form-textarea"
				placeholder="What problem are you solving? What part of history are you investigating or recording?" required></textarea>
		</label>
		<div class="form-actions">
			<button class="action-btn primary" onclick={submitPrep} disabled={prepBusy || !prepNotes.trim()}>
				{prepBusy ? '…' : 'Prepare Plan'}
			</button>
		</div>
	</div>

{:else if plan}
	{@const choicesDone = makeChoices.length > 0}
	{@const baNeeded = countIn(makeChoices, 'break_artifact')}
	{@const baRemaining = Math.max(0, baNeeded - baDone)}
	{@const subflowsDone = baRemaining === 0}

	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		<!-- Invoked artifacts (visible to all) ──────────────────────────── -->
		<div class="choices-section">
			<p class="choices-header">
				Invoked artifacts ({invokedArtifacts.length})
			</p>
			{#if invokedArtifacts.length === 0}
				<p class="choices-note muted">None yet.</p>
			{:else}
				<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
					{#each invokedArtifacts as a}
						<li>{a.name} <span class="muted">({playerName(players, a.owner_id)})</span></li>
					{/each}
				</ul>
			{/if}

			{#if isFocusPlayer && !invokePhaseClosed}
				<div class="plan-form" style="margin-top:0.5rem;">
					<CardPicker
						label="Invoke an artifact (pre-roll)"
						items={uninvokedArtifacts}
						{players}
						emptyMessage="No artifacts available to invoke."
						ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
						selected={invokeAssetID}
						onSelect={(id) => (invokeAssetID = id)}
					/>
					<button class="action-btn"
						onclick={() => submitInvoke(plan)}
						disabled={invokeBusy || invokeAssetID == null}>
						{invokeBusy ? '…' : 'Invoke'}
					</button>
					<p class="choices-note muted">
						The invoke phase closes when the dice are rolled. After that,
						additional invocations happen through the mar "invoke another" option.
					</p>
				</div>
			{:else if isFocusPlayer && invokePhaseClosed}
				<p class="choices-note muted" style="margin-top:0.5rem;">
					Invoke phase closed (dice have been rolled).
				</p>
			{/if}
		</div>

		{#if rollActive && !choicesDone}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome === 'make' && !choicesDone && amChoiceActor}
			<div class="choices-section">
				<p class="choices-header">
					Result: <strong class="outcome-make">✓ Make</strong>
				</p>
				<p class="choices-note">
					Pick options equal to your dice result (repeatable).
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
					onclick={() => onApplyMake(plan)}
					disabled={choicesBusy || totalPicked === 0}>
					{choicesBusy ? '…' : 'Apply choices'}
				</button>
			</div>

		{:else if rollOutcome === 'mar'}
			<!-- Every player picks one option during mar ─────────────── -->
			<div class="choices-section">
				<p class="choices-header">
					Result: <strong class="outcome-mar">✗ Mar</strong>
				</p>
				<p class="choices-note">
					Each player (including the preparer) chooses one option from the
					make list during the scene.
				</p>

				{#if !myMarSubmitted && currentPlayerID != null}
					<div class="plan-form">
						<FormField label="Your choice">
							<div class="chip-row">
								{#each OPTIONS as opt}
									<button
										type="button"
										class="chip-btn"
										class:active={marSelected === opt.key}
										onclick={() => {
											marSelected = marSelected === opt.key ? '' : opt.key;
											marAssetID = null;
										}}
									>{opt.label}</button>
								{/each}
							</div>
						</FormField>
						{#if marSelected === 'break_artifact' || marSelected === 'invoke_another'}
							{@const marArtifacts = marSelected === 'break_artifact' ? invokedArtifacts : uninvokedArtifacts}
							<CardPicker
								label="Artifact"
								items={marArtifacts}
								{players}
								emptyMessage="No eligible artifacts."
								ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
								selected={marAssetID}
								onSelect={(id) => (marAssetID = id)}
							/>
						{/if}
						<button class="action-btn primary"
							onclick={() => submitMarChoice(plan)}
							disabled={marBusy || !marSelected
								|| ((marSelected === 'break_artifact' || marSelected === 'invoke_another')
									&& marAssetID == null)}>
							{marBusy ? '…' : 'Submit choice'}
						</button>
					</div>
				{:else}
					<p class="choices-note muted">Your choice has been submitted.</p>
				{/if}

				<p class="choices-header">Submitted so far ({marEntries.length}):</p>
				{#if marEntries.length === 0}
					<p class="choices-note muted">None yet.</p>
				{:else}
					<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
						{#each marEntries as e}
							<li>{playerName(players, e.playerID)}: {e.choice}</li>
						{/each}
					</ul>
				{/if}

				{#if isFocusPlayer}
					<p class="complete-note" style="margin-top:0.5rem;">
						When every player has chosen, complete the plan.
					</p>
					<button class="action-btn primary"
						onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				{/if}
			</div>

		{:else if choicesDone && isFocusPlayer}
			<div class="complete-section">
				<p class="choices-applied">
					Choices applied:
					{#each OPTIONS as opt}
						{#if countIn(makeChoices, opt.key) > 0}
							<span>{opt.label} × {countIn(makeChoices, opt.key)}; </span>
						{/if}
					{/each}
				</p>

				{#if baRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							Break an invoked artifact ({baRemaining} remaining)
						</p>
						<CardPicker
							label="Marginalium to tear"
							items={baArtifactsWithMarginalia}
							{players}
							emptyMessage="No intact marginalia on invoked artifacts."
							ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
							marginaliaMode
							selectedMarginaliaID={baMargID}
							onSelectMarginalia={(mID, parent) => {
								baMargID = mID;
								baAssetID = parent?.id ?? null;
							}}
						/>
						<button class="action-btn primary"
							onclick={() => submitBreakArtifact(plan)}
							disabled={baBusy || baAssetID == null || baMargID == null}>
							{baBusy ? '…' : 'Tear marginalia'}
						</button>
					</div>
				{/if}

				{#if subflowsDone}
					<p class="complete-note">
						Invoke additional artifacts above if you picked "invoke another",
						then complete the plan.
					</p>
					<button class="action-btn primary"
						onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				{/if}
			</div>

		{:else if !amChoiceActor && !choicesDone}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is resolving Chronicle Histories…
			</p>
		{/if}
	</ResolvingCard>
{/if}
