<!-- ChronicleHistoriesPanel.svelte
  Prep + resolve UI for Chronicle Histories (Tier 1, Knowledge).

  Prep: notes only (the historical problem).

  Resolve:
    - Pre-roll: every player sees a live difficulty meter
      (max(knowledge rank, #invoked)) and the shared Buffet reference. The
      preparer checkboxes the artifacts to invoke (the meter updates live from
      the selection), writes the scene, and submits once: castChronicleRoll
      records the invocations, posts the scene as the preparer's narration, and
      casts the dice.
    - Make (preparer): options are submitted ONE AT A TIME via makeStep
      (result-many total), each with optional narration folded into its log
      entry, leaving breathing room for chat narration between picks. An
      in-panel counter shows how many remain; completion is gated server-side
      (make_budget / make_choices_done). break_artifact / invoke_another carry
      their target in the same submit (no separate sub-flow).
    - Mar (all players): every non-preparer gets a single-choice picker and
      submits via marChoice. The preparer sees the received choices.

  Pre-roll artifact-invocation is gated by resolution_data.invoke_phase_closed,
  which the server flips true inside the cast-roll route.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import './planPanel.css';
	import {
		preparePlan, completePlan,
		castChronicleRoll, makeStep, marChoice,
		type Plan, type Asset, type Player, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import CardPicker from './CardPicker.svelte';
	import ChoicesApplied from './ChoicesApplied.svelte';
	import DifficultyMeter from './shared/DifficultyMeter.svelte';
	import Buffet, { type BuffetTab } from './shared/Buffet.svelte';
	import { parseResolutionData, playerName, assetsWithIntactMarginalia } from './shared';
	import { destructionWarning } from '$lib/assetRisk';
	import type { PlanPanelProps } from './types';
	import FormField from './FormField.svelte';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const rankings = $derived(ctx.rankings);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const rollActive = $derived(ctx.rollActive);
	const rollOutcome = $derived(ctx.rollOutcome);
	const activeRoll = $derived(ctx.activeRoll);
	const onRollCreated = $derived(ctx.onRollCreated);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	// Prep-draft mirroring (Layer 2).
	const readOnly = $derived(ctx.readOnly);
	const prepDraft = $derived(ctx.prepDraft as { notes?: string } | null);

	let performStepsWinnerID = $state<number | null>(null);
	// The preparer resolves their own plan; the perform_steps demand winner may
	// drive the choice in their place.
	const isPreparer = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const amChoiceActor = $derived(
		isPreparer || (currentPlayerID != null && currentPlayerID === performStepsWinnerID),
	);

	const OPTIONS = [
		{ key: 'break_artifact',  label: 'Break an invoked artifact',       	desc: ' — Choose a marginalia to tear.' },
		{ key: 'invoke_another',  label: 'Invoke another artifact', 			desc: '  and introduce it to the scene.' },
		{ key: 'echo_present',    label: "Cut to the present to show history's impact", desc: '' },
		{ key: 'total_control',   label: 'Take narrative control of a moment',	desc: " — Dictating a character's actions requires that player's consent." },
	] as const;

	// Read-only "what can happen?" reference (shared Buffet).
	const buffetTabs: BuffetTab[] = [
		{
			key: 'make', label: 'Make', intro: 'Choose X options equal to your result:',
			opts: OPTIONS.map(o => ({ key: o.key, label: o.label, desc: o.desc })),
		},
		{
			key: 'mar', label: 'Mar',
			always: 'Every OTHER player chooses one of these instead:',
			opts: OPTIONS.map(o => ({ key: o.key, label: o.label })),
		},
	];

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

	$effect(() => { if (readOnly) prepNotes = prepDraft?.notes ?? ''; });
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

	// ── Resolve: invoked-artifact tracking ───────────────────────────────────
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

	// ── Pre-roll: choose artifacts + set the scene, then cast in one submit ───
	// The preparer checkboxes the artifacts to invoke (the meter updates live
	// from the local selection), writes the scene, then submits once: the
	// server records the invocations, posts the scene, and casts the dice.
	let selectedInvokeIDs = $state<number[]>([]);
	let sceneText = $state('');
	let preRollBusy = $state(false);

	// ── Live difficulty (read-only to everyone) ──────────────────────────────
	// Mirrors the server's ChronicleHistoriesDifficulty: max(rank, #invoked).
	const knowledgeRank = $derived(
		plan == null ? 0
			: (rankings.find(r => r.category === 'knowledge' && r.player_id === plan.preparer_id)?.rank ?? 0)
	);
	// Pre-roll difficulty tracks the preparer's live (uncommitted) checkbox
	// selection; for everyone else it reflects what's been committed so far.
	const meterCount = $derived(isPreparer ? selectedInvokeIDs.length : invokedIDs.length);
	const difficulty = $derived(Math.max(knowledgeRank, meterCount));
	const meterReason = $derived(
		`knowledge rank ${knowledgeRank} · ${meterCount} artifact${meterCount === 1 ? '' : 's'} invoked → max = ${difficulty}`
	);
	async function submitPreRoll(p: Plan) {
		if (preRollBusy) return;
		preRollBusy = true; resError = '';
		try {
			const res = await castChronicleRoll(p.id, selectedInvokeIDs, sceneText.trim());
			if (res.roll) onRollCreated(res.roll);
			selectedInvokeIDs = [];
			sceneText = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not set the scene.';
		} finally { preRollBusy = false; }
	}

	// ── Make: one option at a time ───────────────────────────────────────────
	let resError = $state('');
	let makeOption = $state<string>('');
	let makeNarration = $state('');
	let makeAssetID = $state<number | null>(null);
	let makeMargID = $state<number | null>(null);
	let makeBusy = $state(false);

	const makeNeedsAsset = $derived(makeOption === 'break_artifact' || makeOption === 'invoke_another');
	const makeNeedsMarg = $derived(makeOption === 'break_artifact');
	const makeReady = $derived(
		!!makeOption
		&& (!makeNeedsAsset || makeAssetID != null)
		&& (!makeNeedsMarg || makeMargID != null),
	);
	const makeBreakWarn = $derived(destructionWarning(invokedArtifacts.find(a => a.id === makeAssetID)));

	// Server-authoritative progress. Budget = the dice result (captured server-
	// side on the first step; fall back to the live roll result before then).
	const makeChoicesDone = $derived(
		plan ? (parseResolutionData(plan).chronicle_histories?.make_choices_done ?? 0) : 0,
	);
	const makeBudget = $derived(
		(plan ? (parseResolutionData(plan).chronicle_histories?.make_budget ?? 0) : 0)
			|| (activeRoll?.result ?? 0),
	);
	const makeRemaining = $derived(Math.max(0, makeBudget - makeChoicesDone));
	const makeStarted = $derived(makeChoicesDone > 0);

	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			makeOption = ''; makeNarration = ''; makeAssetID = null; makeMargID = null;
		}
	});

	async function submitMakeStep(p: Plan) {
		if (makeBusy || !makeReady) return;
		makeBusy = true; resError = '';
		try {
			await makeStep(p.id, makeOption, {
				narration: makeNarration.trim() || undefined,
				assetID: makeNeedsAsset ? makeAssetID ?? undefined : undefined,
				marginaliaID: makeNeedsMarg ? makeMargID ?? undefined : undefined,
			});
			makeOption = ''; makeNarration = ''; makeAssetID = null; makeMargID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not submit choice.';
		} finally { makeBusy = false; }
	}

	// ── Mar picker (per-player single choice) ────────────────────────────────
	let marSelected = $state<string>('');
	let marAssetID = $state<number | null>(null);
	let marMargID = $state<number | null>(null);
	let marBusy = $state(false);

	const marNeedsAsset = $derived(marSelected === 'break_artifact' || marSelected === 'invoke_another');
	const marNeedsMarg = $derived(marSelected === 'break_artifact');
	const marReady = $derived(
		!!marSelected
		&& (!marNeedsAsset || marAssetID != null)
		&& (!marNeedsMarg || marMargID != null),
	);

	async function submitMarChoice(p: Plan) {
		if (marBusy || !marReady) return;
		marBusy = true; resError = '';
		try {
			await marChoice(
				p.id,
				marSelected,
				marNeedsAsset && marAssetID != null ? marAssetID : undefined,
				marNeedsMarg && marMargID != null ? marMargID : undefined,
			);
			marSelected = ''; marAssetID = null; marMargID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not submit mar choice.';
		} finally { marBusy = false; }
	}

	// Destruction warning for the selected mar break target.
	const marBreakWarn = $derived(destructionWarning(invokedArtifacts.find(a => a.id === marAssetID)));

	// Decoded mar-choice entries (player_id set). Make entries leave player_id null.
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

	const marRequired = $derived(
		plan ? (parseResolutionData(plan).chronicle_histories?.mar_required_choices ?? 0) : 0,
	);
	const allMarSubmitted = $derived(marRequired > 0 && marEntries.length >= marRequired);

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

	// Make-path choices are the entries with player_id null.
	const makeChoices = $derived(
		plan
			? (parseResolutionData(plan).make_mar_choices ?? [])
				.filter(c => c.player_id == null)
				.map(c => c.option)
			: []
	);
</script>

{#if mode === 'prep'}
	<fieldset class="plan-form-fieldset" disabled={readOnly}>
		<div class="plan-form">
			{#if prepError}<p class="res-error">{prepError}</p>{/if}
			<label class="form-label">
				Area of study:
				<textarea rows={3} bind:value={prepNotes} class="form-textarea"
					placeholder="What problem are you solving? What part of history are you investigating or recording?" required></textarea>
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

		<!-- Pre-roll: live difficulty meter (read-only to all) ─────────────── -->
		{#if !invokePhaseClosed}
			<div class="choices-section">
				<DifficultyMeter
					headline="Starting difficulty"
					value={difficulty}
					floor={knowledgeRank}
					reason={meterReason}
				/>
			</div>
		{/if}

		<!-- "What can happen?" reference (shared) ──────────────────────────── -->
		<Buffet tabs={buffetTabs} />

		<!-- Invoked artifacts — context once the dice are cast ──────────────── -->
		{#if invokePhaseClosed && invokedArtifacts.length > 0}
			<div class="choices-section">
				<p class="choices-header">Invoked artifacts ({invokedArtifacts.length})</p>
				<ul class="plan-notes" style="margin:0;padding-left:1.25rem;">
					{#each invokedArtifacts as a}
						<li>{a.name} <span class="muted">({playerName(players, a.owner_id)})</span></li>
					{/each}
				</ul>
			</div>
		{/if}

		<!-- Pre-roll: choose artifacts + set the scene, one submit. Everyone sees
		     the same UI; non-preparers see it read-only (no live selection). ── -->
		{#if !invokePhaseClosed}
			<div class="choices-section plan-form">
				<CardPicker
					label="Invoke artifacts to shed light on (each raises the difficulty)"
					items={allArtifacts}
					{players}
					emptyMessage="No artifacts available to invoke."
					ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
					multi
					selectedMulti={selectedInvokeIDs}
					onSelectMulti={(ids) => (selectedInvokeIDs = ids)}
					readOnly={!isPreparer}
				/>
				<FormField label="Set the scene">
					<textarea rows={3} bind:value={sceneText} class="form-textarea"
						placeholder="Describe the moment from the past you're shedding light on…"
						disabled={!isPreparer}></textarea>
				</FormField>
				{#if isPreparer}
					<button class="action-btn primary"
						onclick={() => submitPreRoll(plan)}
						disabled={preRollBusy}>
						{preRollBusy ? '…' : 'Set the scene & gather your dice'}
					</button>
				{:else}
					<p class="ft-prompt muted">
						{playerName(players, plan.preparer_id)} is setting the scene…
					</p>
				{/if}
			</div>
		{/if}

		{#if rollActive && rollOutcome == null}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome === 'make'}
			<div class="choices-section">
				<p class="choices-header">
					Result: <strong class="outcome-make">✓ Make</strong>
				</p>

				{#if makeRemaining > 0}
					<p class="choices-note">
						<strong>{makeRemaining}</strong> of {makeBudget}
						choice{makeBudget === 1 ? '' : 's'} remaining — submit them one at a time.
					</p>
					{#if amChoiceActor}
						<div class="plan-form">
							<FormField label="Choose an option">
								<div class="chip-row">
									{#each OPTIONS as opt}
										<button
											type="button"
											class="chip-btn"
											class:active={makeOption === opt.key}
											onclick={() => {
												makeOption = makeOption === opt.key ? '' : opt.key;
												makeAssetID = null;
												makeMargID = null;
											}}
										>{opt.label}</button>
									{/each}
								</div>
							</FormField>
							{#if makeOption === 'break_artifact'}
								<CardPicker
									label="Invoked artifact to break (tear a marginalia)"
									items={assetsWithIntactMarginalia(invokedArtifacts)}
									{players}
									emptyMessage="No intact marginalia on invoked artifacts."
									ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
									marginaliaMode
									selectedMarginaliaID={makeMargID}
									onSelectMarginalia={(mID, parent) => {
										makeMargID = mID;
										makeAssetID = parent?.id ?? null;
									}}
								/>
								{#if makeBreakWarn}<p class="res-warning">{makeBreakWarn}</p>{/if}
							{:else if makeOption === 'invoke_another'}
								<CardPicker
									label="Artifact to invoke"
									items={uninvokedArtifacts}
									{players}
									emptyMessage="No eligible artifacts."
									ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
									selected={makeAssetID}
									onSelect={(id) => (makeAssetID = id)}
								/>
							{/if}
							<FormField label="Narration (optional — posted with this choice)">
								<textarea rows={2} bind:value={makeNarration} class="form-textarea"
									placeholder="Describe this beat of the scene…"></textarea>
							</FormField>
							<button class="action-btn primary"
								onclick={() => submitMakeStep(plan)}
								disabled={makeBusy || !makeReady}>
								{makeBusy ? '…' : 'Submit choice'}
							</button>
						</div>
					{:else}
						<p class="ft-prompt muted">
							{playerName(players, plan.preparer_id)} is choosing options…
						</p>
					{/if}
				{/if}

				{#if makeStarted}
					<ChoicesApplied choices={makeChoices} options={OPTIONS} />
				{/if}

				{#if makeRemaining === 0 && makeStarted}
					{#if isPreparer}
						<p class="complete-note">All choices submitted — complete the plan.</p>
						<button class="action-btn primary"
							onclick={() => onComplete(plan)} disabled={resBusy}>
							{resBusy ? '…' : 'Complete plan'}
						</button>
					{:else}
						<p class="ft-prompt muted">
							{playerName(players, plan.preparer_id)} has finished —
							completing Chronicle Histories…
						</p>
					{/if}
				{/if}
			</div>

		{:else if rollOutcome === 'mar'}
			<!-- Every player picks one option during mar ─────────────────── -->
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
											marMargID = null;
										}}
									>{opt.label}</button>
								{/each}
							</div>
						</FormField>
						{#if marSelected === 'break_artifact'}
							<CardPicker
								label="Invoked artifact to break (tear a marginalia)"
								items={assetsWithIntactMarginalia(invokedArtifacts)}
								{players}
								emptyMessage="No intact marginalia on invoked artifacts."
								ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
								marginaliaMode
								selectedMarginaliaID={marMargID}
								onSelectMarginalia={(mID, parent) => {
									marMargID = mID;
									marAssetID = parent?.id ?? null;
								}}
							/>
							{#if marBreakWarn}<p class="res-warning">{marBreakWarn}</p>{/if}
						{:else if marSelected === 'invoke_another'}
							<CardPicker
								label="Artifact to invoke"
								items={uninvokedArtifacts}
								{players}
								emptyMessage="No eligible artifacts."
								ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
								selected={marAssetID}
								onSelect={(id) => (marAssetID = id)}
							/>
						{/if}
						<button class="action-btn primary"
							onclick={() => submitMarChoice(plan)}
							disabled={marBusy || !marReady}>
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

				{#if isPreparer}
					<p class="complete-note" style="margin-top:0.5rem;">
						{#if allMarSubmitted}
							Every player has chosen — complete the plan.
						{:else}
							Waiting for all players to choose ({marEntries.length}/{marRequired})…
						{/if}
					</p>
					<button class="action-btn primary"
						onclick={() => onComplete(plan)} disabled={resBusy || !allMarSubmitted}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				{/if}
			</div>
		{/if}
	</ResolvingCard>
{/if}
