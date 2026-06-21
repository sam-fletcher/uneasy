<!-- SeekAnswersPanel.svelte
  Prep + resolve UI for Seek Answers (Tier 1, Knowledge).

  Flow:
  - Prep: notes textarea (required).
  - Resolve: dice roll → pick option counts (repeatable, total = dice result)
    → submit via makeChoice → fill a sub-form per pick (break_resource,
    reveal_secret, declare_truth, ask_question) → complete. ask_question is a
    two-sided flow: the target answers (or, if they outrank the preparer on
    knowledge, vetoes the first question once).

  The option list is the same for make and mar (per spec). Each resource may
  be flawed at most once ("overlooked until now"), so flawed resources drop
  out of the pickers. On a mar the preparer must additionally describe a flaw
  in (difficulty − result) of their OWN resources; that penalty sub-flow blocks
  completion until satisfied.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		castSeekAnswersRoll, breakResource, revealSecret,
		declareTruth, askQuestion, vetoQuestion, answerQuestion,
		type Plan, type Asset, type Player, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import CardPicker from './CardPicker.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import ChoicesApplied from './ChoicesApplied.svelte';
	import { parseResolutionData, playerName, assetsWithIntactMarginalia } from './shared';
	import { destructionWarning } from '$lib/assetRisk';

	import type { PlanPanelProps } from './types';

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
	const onRollCreated = $derived(ctx.onRollCreated);

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
		{ key: 'break_resource', label: 'Break a resource (describe a flaw)' },
		{ key: 'declare_truth',  label: 'Declare something true' },
		{ key: 'ask_question',   label: 'Ask a player a truthful question' },
		{ key: 'reveal_secret',  label: "Reveal an asset's secrets to you" },
	] as const;

	// ── Prep ─────────────────────────────────────────────────────────────────
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	async function submitPrep() {
		if (prepBusy) return;
		if (!prepNotes.trim()) { prepError = 'Describe your research methods.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'seek_answers',
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

	// ── Pre-roll: restate methods + one thing learned, then cast ─────────────
	// OnResolve opens the plan with no roll; the preparer writes their pre-roll
	// narration and casts via cast-roll, which posts it and creates the dice.
	let preRollText = $state('');
	let preRollBusy = $state(false);
	async function submitPreRoll(p: Plan) {
		if (preRollBusy || !preRollText.trim()) return;
		preRollBusy = true; resError = '';
		try {
			const res = await castSeekAnswersRoll(p.id, preRollText.trim());
			if (res.roll) onRollCreated(res.roll);
			preRollText = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not cast your dice.';
		} finally { preRollBusy = false; }
	}

	// ── Resolve: option counts picker ────────────────────────────────────────
	let counts = $state<Record<string, number>>({
		break_resource: 0, declare_truth: 0, ask_question: 0, reveal_secret: 0,
	});
	let choicesBusy = $state(false);
	let resError = $state('');

	const totalPicked = $derived(Object.values(counts).reduce((a, b) => a + b, 0));
	// "Choose a number of these options equal to your result" — exactly result,
	// on both a make and a mar (MaxChoices = result).
	const requiredPicks = $derived(activeRoll?.result ?? null);

	function bump(key: string, delta: number) {
		// Don't let the running total exceed the dice quota.
		if (delta > 0 && requiredPicks != null && totalPicked >= requiredPicks) return;
		const next = Math.max(0, (counts[key] ?? 0) + delta);
		counts = { ...counts, [key]: next };
	}

	async function onApplyChoices(p: Plan, outcome: 'make' | 'mar') {
		if (choicesBusy || totalPicked === 0) return;
		if (requiredPicks != null && totalPicked !== requiredPicks) return;
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

	// ── Sub-flows: break_resource, reveal_secret ─────────────────────────────
	// Completion is server-authoritative (resolution_data counters, derived from
	// saData below) so a refresh/remount can't re-prompt — and re-run — a step
	// that already happened; the handlers also reject any step beyond the pick.

	// Sub-form selection state
	let brAssetID = $state<number | null>(null);
	let brMargID = $state<number | null>(null);
	let brBusy = $state(false);

	// Mar penalty selection (own resources).
	let maAssetID = $state<number | null>(null);
	let maMargID = $state<number | null>(null);
	let maBusy = $state(false);

	// Destruction warnings for the currently-selected break targets.
	const brWarn = $derived(destructionWarning(assets.find(a => a.id === brAssetID)));
	const maWarn = $derived(destructionWarning(assets.find(a => a.id === maAssetID)));

	let rsAssetID = $state<number | null>(null);
	let rsBusy = $state(false);

	// Seek Answers resolution state: which resources have been flawed (each may
	// be flawed at most once) and the mar self-flaw penalty progress.
	const saData = $derived(plan ? (parseResolutionData(plan).seek_answers ?? {}) : {});
	const preRollDone = $derived(saData.pre_roll_done ?? false);
	const flawedIDs = $derived(new Set(saData.flawed_resource_ids ?? []));
	const marRequired = $derived(saData.mar_self_flaws_required ?? 0);
	const marApplied = $derived(saData.mar_self_flaws_applied ?? 0);
	const marRemaining = $derived(Math.max(0, marRequired - marApplied));
	// Server-recorded make-list sub-flow progress (see comment above).
	const breakDone = $derived(saData.break_resource_done ?? 0);
	const revealDone = $derived(saData.reveal_secret_done ?? 0);
	const declareTruthDone = $derived(saData.declare_truth_done ?? 0);
	const askQuestionDone = $derived(saData.ask_question_done ?? 0);
	const pendingQuestion = $derived(saData.pending_question ?? null);
	const askVetoedHint = $derived(saData.current_ask_vetoed ?? false);

	const preparerID = $derived(plan?.preparer_id ?? null);
	const isMar = $derived(rollOutcome === 'mar');

	// Make-list "break a resource" picker: any resource with intact marginalia
	// not yet flawed this plan. On a mar, exclude the preparer's own resources —
	// those go through the penalty flow (the server attributes a break by owner).
	const resourcesWithMarginalia = $derived(
		assetsWithIntactMarginalia(assets.filter(a => a.asset_type === 'resource')),
	);
	const brResourcesWithMarginalia = $derived(
		resourcesWithMarginalia
			.filter(a => !flawedIDs.has(a.id))
			.filter(a => !isMar || a.owner_id !== preparerID),
	);
	// Mar penalty picker: the preparer's own resources, not yet flawed.
	const penaltyResources = $derived(
		resourcesWithMarginalia
			.filter(a => a.owner_id === preparerID && !flawedIDs.has(a.id)),
	);
	// Reveal-secret targets another player's asset ("ask a player to show you the
	// underside of a specific one of their assets") — never the preparer's own.
	const revealableAssets = $derived(
		assets.filter(a => !a.is_destroyed && a.owner_id !== preparerID),
	);


	async function submitBreakResource(p: Plan) {
		if (brBusy || brAssetID == null || brMargID == null) return;
		brBusy = true; resError = '';
		try {
			await breakResource(p.id, brAssetID, brMargID);
			brAssetID = null; brMargID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not break resource.';
		} finally { brBusy = false; }
	}

	// Penalty self-flaw: like submitBreakResource but progress is tracked by the
	// server (mar_self_flaws_applied), so we just refetch after each break.
	async function submitPenaltyFlaw(p: Plan) {
		if (maBusy || maAssetID == null || maMargID == null) return;
		maBusy = true; resError = '';
		try {
			await breakResource(p.id, maAssetID, maMargID);
			maAssetID = null; maMargID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not flaw your resource.';
		} finally { maBusy = false; }
	}

	async function submitRevealSecret(p: Plan) {
		if (rsBusy || rsAssetID == null) return;
		rsBusy = true; resError = '';
		try {
			await revealSecret(p.id, rsAssetID);
			rsAssetID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not reveal secret.';
		} finally { rsBusy = false; }
	}

	// ── declare_truth / ask_question sub-flows ───────────────────────────────
	let dtText = $state('');
	let dtBusy = $state(false);
	async function submitDeclareTruth(p: Plan) {
		if (dtBusy || !dtText.trim()) return;
		dtBusy = true; resError = '';
		try {
			await declareTruth(p.id, dtText.trim());
			dtText = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not declare truth.';
		} finally { dtBusy = false; }
	}

	// Other players the preparer can question.
	const questionTargets = $derived(players.filter(p => p.id !== plan?.preparer_id));
	let aqTargetID = $state<number | null>(null);
	let aqQuestion = $state('');
	let aqBusy = $state(false);
	async function submitAskQuestion(p: Plan) {
		if (aqBusy || aqTargetID == null || !aqQuestion.trim()) return;
		aqBusy = true; resError = '';
		try {
			await askQuestion(p.id, aqTargetID, aqQuestion.trim());
			aqTargetID = null; aqQuestion = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not ask question.';
		} finally { aqBusy = false; }
	}

	// Target's answer/veto of the open question.
	let answerText = $state('');
	let answerBusy = $state(false);
	async function submitAnswer(p: Plan) {
		if (answerBusy || !answerText.trim()) return;
		answerBusy = true; resError = '';
		try {
			await answerQuestion(p.id, answerText.trim());
			answerText = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not answer.';
		} finally { answerBusy = false; }
	}
	async function submitVeto(p: Plan) {
		if (answerBusy) return;
		answerBusy = true; resError = '';
		try {
			await vetoQuestion(p.id);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not veto.';
		} finally { answerBusy = false; }
	}

	// ── Complete ────────────────────────────────────────────────────────────
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
			<label class="form-label">
				Research methods and topics:
				<textarea rows={3} bind:value={prepNotes} class="form-textarea"
					placeholder="What are you learning, and how?" required></textarea>
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
	{@const existingChoices = (parseResolutionData(plan).make_mar_choices ?? []).map(c => c.option)}
	{@const choicesDone = existingChoices.length > 0}
	{@const brNeeded = countIn(existingChoices, 'break_resource')}
	{@const rsNeeded = countIn(existingChoices, 'reveal_secret')}
	{@const dtNeeded = countIn(existingChoices, 'declare_truth')}
	{@const aqNeeded = countIn(existingChoices, 'ask_question')}
	{@const brRemaining = Math.max(0, brNeeded - breakDone)}
	{@const rsRemaining = Math.max(0, rsNeeded - revealDone)}
	{@const dtRemaining = Math.max(0, dtNeeded - declareTruthDone)}
	{@const aqRemaining = Math.max(0, aqNeeded - askQuestionDone)}
	{@const subflowsDone = brRemaining === 0 && rsRemaining === 0 && marRemaining === 0
		&& dtRemaining === 0 && aqRemaining === 0 && pendingQuestion == null}

	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		{#if !preRollDone}
			<!-- Pre-roll: restate methods + describe one thing learned, then cast.
			     The narration is the preparer's own; only they can write it. -->
			<!-- <div class="plan-form"> -->
				<p class="choices-header">What have you learned?</p>
				{#if isPreparer}
					<p class="choices-note">
						<!-- Restate your research methods, and describe one thing you've learned while researching. -->
					</p>
					<label class="form-label">
						<!-- Your pre-roll narration: -->
						<textarea rows={4} bind:value={preRollText} class="form-textarea"
							placeholder="Restate your research methods, and describe one thing you've learned while researching."
						></textarea>
					</label>
					<button class="action-btn primary"
						onclick={() => submitPreRoll(plan)}
						disabled={preRollBusy || !preRollText.trim()}>
						{preRollBusy ? '…' : 'Submit'}
					</button>
				{:else}
					<p class="ft-prompt muted">
						{playerName(players, plan.preparer_id)} is researching…
					</p>
				{/if}
			<!-- </div> -->

		{:else if rollActive && !choicesDone}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome != null && !choicesDone && amChoiceActor}
			<div class="choices-section">
				<p class="choices-header">
					Result: <strong class="outcome-{rollOutcome}">
						{rollOutcome === 'make' ? '✓ Make' : '✗ Mar'}
					</strong>
				</p>
				<p class="choices-note">
					Pick options equal to your dice result (repeatable){#if requiredPicks != null}: choose exactly <strong>{requiredPicks}</strong>{/if}. On a mar
					you must also describe a flaw in your own resources — that penalty
					appears below once you apply your choices.
				</p>
				{#each OPTIONS as opt}
					<div class="choice-item" style="display:flex;align-items:center;gap:0.5rem;">
						<button class="action-btn" onclick={() => bump(opt.key, -1)}
							disabled={(counts[opt.key] ?? 0) === 0}>−</button>
						<strong style="min-width:1.5rem;text-align:center;">{counts[opt.key] ?? 0}</strong>
						<button class="action-btn" onclick={() => bump(opt.key, 1)}
							disabled={requiredPicks != null && totalPicked >= requiredPicks}>+</button>
						<span>{opt.label}</span>
					</div>
				{/each}
				<p class="choices-note">
					Total picks: <strong>{totalPicked}</strong>{#if requiredPicks != null} / {requiredPicks}{/if}
				</p>
				<button class="action-btn primary"
					onclick={() => onApplyChoices(plan, rollOutcome!)}
					disabled={choicesBusy || totalPicked === 0 || (requiredPicks != null && totalPicked !== requiredPicks)}>
					{choicesBusy ? '…' : 'Apply choices'}
				</button>
			</div>

		{:else if pendingQuestion != null && pendingQuestion.target_id === currentPlayerID}
			<!-- The questioned player answers truthfully — or vetoes, if they
			     outrank the preparer on knowledge. -->
			<div class="plan-form">
				<p class="choices-header">
					{playerName(players, plan.preparer_id)} asks you a question
				</p>
				<blockquote class="q-quote">{pendingQuestion.question}</blockquote>
				<label class="form-label">
					Your truthful answer:
					<textarea rows={3} bind:value={answerText} class="form-textarea"
						placeholder="Answer the question…"></textarea>
				</label>
				<div class="form-actions start">
					<button class="action-btn primary"
						onclick={() => submitAnswer(plan)}
						disabled={answerBusy || !answerText.trim()}>
						{answerBusy ? '…' : 'Answer'}
					</button>
					{#if pendingQuestion.vetoable}
						<button class="action-btn secondary"
							onclick={() => submitVeto(plan)} disabled={answerBusy}>
							Veto (ask another)
						</button>
					{/if}
				</div>
				{#if pendingQuestion.vetoable}
					<p class="choices-note muted">
						You outrank {playerName(players, plan.preparer_id)} on knowledge, so you
						may veto this first question — they'll have to ask another.
					</p>
				{/if}
			</div>

		{:else if choicesDone && isPreparer}
			<div class="complete-section">
				<ChoicesApplied choices={existingChoices} options={OPTIONS} />

				{#if brRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							Break a resource ({brRemaining} remaining)
						</p>
						<CardPicker
							label="Marginalium to tear"
							items={brResourcesWithMarginalia}
							{players}
							emptyMessage="No intact marginalia on any resource."
							ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
							marginaliaMode
							selectedMarginaliaID={brMargID}
							onSelectMarginalia={(mID, parent) => {
								brMargID = mID;
								brAssetID = parent?.id ?? null;
							}}
						/>
						{#if brWarn}<p class="res-warning">{brWarn}</p>{/if}
						<button class="action-btn primary"
							onclick={() => submitBreakResource(plan)}
							disabled={brBusy || brAssetID == null || brMargID == null}>
							{brBusy ? '…' : 'Break resource'}
						</button>
					</div>
				{/if}

				{#if marRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							Describe a flaw in your own resources ({marRemaining} remaining)
						</p>
						<p class="choices-note">
							The plan marred — you must flaw {marRequired} of your own
							resources before completing.
						</p>
						<CardPicker
							label="Your resource to flaw"
							items={penaltyResources}
							{players}
							emptyMessage="No eligible resources of your own remain."
							ownerLabel={() => 'Your resource'}
							marginaliaMode
							selectedMarginaliaID={maMargID}
							onSelectMarginalia={(mID, parent) => {
								maMargID = mID;
								maAssetID = parent?.id ?? null;
							}}
						/>
						{#if maWarn}<p class="res-warning">{maWarn}</p>{/if}
						<button class="action-btn primary"
							onclick={() => submitPenaltyFlaw(plan)}
							disabled={maBusy || maAssetID == null || maMargID == null}>
							{maBusy ? '…' : 'Flaw your resource'}
						</button>
					</div>
				{/if}

				{#if rsRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							Reveal an asset's secrets ({rsRemaining} remaining)
						</p>
						<CardPicker
							label="Asset"
							items={revealableAssets}
							{players}
							emptyMessage="No assets available."
							ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
							selected={rsAssetID}
							onSelect={(id) => (rsAssetID = id)}
						/>
						<button class="action-btn primary"
							onclick={() => submitRevealSecret(plan)}
							disabled={rsBusy || rsAssetID == null}>
							{rsBusy ? '…' : 'Reveal secrets'}
						</button>
					</div>
				{/if}

				{#if dtRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							Declare something true ({dtRemaining} remaining)
						</p>
						<p class="choices-note">
							It can't contradict any truth already known to the table.
						</p>
						<label class="form-label">
							The truth you declare:
							<textarea rows={2} bind:value={dtText} class="form-textarea"
								placeholder="Declare something true about the world…"></textarea>
						</label>
						<button class="action-btn primary"
							onclick={() => submitDeclareTruth(plan)}
							disabled={dtBusy || !dtText.trim()}>
							{dtBusy ? '…' : 'Declare truth'}
						</button>
					</div>
				{/if}

				{#if aqRemaining > 0}
					<div class="plan-form">
						<p class="choices-header">
							Ask a player a question ({aqRemaining} remaining)
						</p>
						{#if pendingQuestion != null}
							<p class="choices-note">
								Waiting for {playerName(players, pendingQuestion.target_id)} to answer
								{#if pendingQuestion.vetoable}or veto {/if}your question:
							</p>
							<blockquote class="q-quote">{pendingQuestion.question}</blockquote>
						{:else}
							{#if askVetoedHint}
								<p class="choices-note muted">
									Your last question was vetoed — ask another (it can't be vetoed).
								</p>
							{/if}
							<PlayerChips
								players={questionTargets}
								isActive={(p) => p.id === aqTargetID}
								onSelect={(p) => (aqTargetID = p.id)}
							/>
							<label class="form-label">
								Your question:
								<textarea rows={2} bind:value={aqQuestion} class="form-textarea"
									placeholder="Ask a question they must answer truthfully…"></textarea>
							</label>
							<button class="action-btn primary"
								onclick={() => submitAskQuestion(plan)}
								disabled={aqBusy || aqTargetID == null || !aqQuestion.trim()}>
								{aqBusy ? '…' : 'Ask question'}
							</button>
						{/if}
					</div>
				{/if}

				{#if subflowsDone}
					<p class="complete-note">
						Post any questions, truths, or follow-scene narration in the scene
						thread, then complete the plan.
					</p>
					<button class="action-btn primary"
						onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				{/if}
			</div>

		{:else if !amChoiceActor && !choicesDone}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is resolving Seek Answers…
			</p>
		{/if}
	</ResolvingCard>
{/if}

<style>
	:global(.form-select) {
		width: 100%;
		padding: 0.4rem;
		border: 1px solid var(--border, #ccc);
		border-radius: 4px;
	}
</style>
