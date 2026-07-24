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
		castSeekAnswersRoll, breakResource, revealSecret, forfeitSeekStep,
		declareTruth, askQuestion, vetoQuestion, answerQuestion,
		type Plan, type Asset, type Player, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import CardPicker from './CardPicker.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import ChoicesApplied from './ChoicesApplied.svelte';
	import { parseResolutionData, playerName, breakableAssets } from './shared';
	import { destructionWarning } from '$lib/assetRisk';
	import { hiddenCount } from '$lib/secretCounts';
	import { useSecretCounts } from '$lib/secretCountsContext';

	import type { PlanPanelProps } from './types';
	import { TEXT_LIMITS } from '$lib/textLimits';

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
		{ key: 'declare_truth',  label: 'Declare something true about the world' },
		{ key: 'ask_question',   label: 'Ask a player a question (they must be truthful)' },
		{ key: 'reveal_secret',  label: "Learn the secrets of an asset" },
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
		// Can't pick an option with no valid target, or past its live target count.
		if (delta > 0 && (!optionAvailable[key] || atCap(key))) return;
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
	// Pre-roll is done once this plan's dice exist. pre_roll_done is the
	// server-authoritative flag for plans created under the pre-roll flow; the
	// roll-existence and committed-choices fallbacks free legacy in-flight plans
	// — resolved before the pre-roll step shipped, so they have no flag — from
	// being trapped on the pre-roll screen.
	const rollForThisPlan = $derived(
		plan != null && activeRoll != null && activeRoll.plan_id === plan.id,
	);
	const hasCommittedChoices = $derived(
		plan != null && (parseResolutionData(plan).make_mar_choices ?? []).length > 0,
	);
	const preRollDone = $derived(
		(saData.pre_roll_done ?? false) || rollForThisPlan || hasCommittedChoices,
	);
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

	// ── Tier-1 committed state: the specifics of each completed step, recorded in
	// resolution_data and shown read-only to every viewer (see ADR-006) so the
	// choice that was made persists in the panel instead of vanishing when its
	// picker commits. flawed_resource_ids splits by owner: on a mar the preparer's
	// own flaws are the penalty, everything else is a make-list break.
	const assetName = (id: number): string =>
		assets.find(a => a.id === id)?.name ?? 'an asset';
	const revealedAssetIDs = $derived(saData.revealed_asset_ids ?? []);
	const declaredTruths = $derived(saData.declared_truths ?? []);
	const answeredQuestions = $derived(saData.answered_questions ?? []);
	const flawedList = $derived(saData.flawed_resource_ids ?? []);
	const penaltyBrokenIDs = $derived(
		flawedList.filter(id => isMar && assets.find(a => a.id === id)?.owner_id === preparerID),
	);
	const makeBrokenIDs = $derived(
		flawedList.filter(id => !(isMar && assets.find(a => a.id === id)?.owner_id === preparerID)),
	);
	const anyResolved = $derived(
		makeBrokenIDs.length > 0 || revealedAssetIDs.length > 0 || declaredTruths.length > 0
			|| answeredQuestions.length > 0 || penaltyBrokenIDs.length > 0,
	);

	// Make-list "break a resource" picker: any breakable resource not yet flawed
	// this plan — one with intact marginalia, or a blank one, whose break
	// destroys it outright. On a mar, exclude the preparer's own resources —
	// those go through the penalty flow (the server attributes a break by owner).
	const resourcesWithMarginalia = $derived(
		breakableAssets(assets.filter(a => a.asset_type === 'resource')),
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
	// The preparer (the only player who ever picks reveal targets) sees assets
	// that still hold a secret THEY can't read, since revealing one they already
	// know fully is a no-op. That filter is viewer-scoped via the shared lookup,
	// so a read-only observer can't reproduce it — which secrets the preparer
	// knows is confidential. For the greyed mirror we fall back to public
	// existence (any secret at all), identical for every viewer; the lists differ
	// only for an asset the preparer already knows fully, which is rare and
	// harmless to show greyed.
	const secretCounts = useSecretCounts();
	const hasUnknownSecret = (a: Asset): boolean =>
		hiddenCount(a, secretCounts?.known(a.id) ?? 0) > 0;
	const revealableAssets = $derived(
		assets.filter(a =>
			!a.is_destroyed && a.owner_id !== preparerID
			&& (isPreparer ? hasUnknownSecret(a) : a.secret_count > 0),
		),
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

	// Discharge a depletable step that has remaining picks but no live target —
	// e.g. every breakable resource is gone. The server re-checks that no target
	// exists, so this only ever resolves a genuine dead-end (it can't skip a step
	// the preparer could still perform). Without it the plan wedges.
	let forfeitBusy = $state(false);
	async function forfeitStep(p: Plan, step: 'break_resource' | 'reveal_secret' | 'mar_penalty') {
		if (forfeitBusy) return;
		forfeitBusy = true; resError = '';
		try {
			await forfeitSeekStep(p.id, step);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not skip step.';
		} finally { forfeitBusy = false; }
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

	// Per-option availability for the counts picker: an option with no valid
	// target can't be picked, so we disable it. declare_truth is always possible;
	// the others need a live target (a breakable resource, a questionable player,
	// or an asset whose secrets you don't already hold).
	const optionAvailable = $derived<Record<string, boolean>>({
		break_resource: brResourcesWithMarginalia.length > 0,
		declare_truth: true,
		ask_question: questionTargets.length > 0,
		reveal_secret: revealableAssets.length > 0,
	});
	// Per-option cap (null = uncapped): the depletable options (break a resource,
	// reveal a secret) can't be picked more times than they have live targets —
	// otherwise the surplus picks commit and then wedge with no target to spend
	// them on. The non-depletable options (declare a truth; ask a question — you
	// can re-ask the same player) are uncapped, so the dice quota can always be
	// reached through them.
	const optionCap = $derived<Record<string, number | null>>({
		break_resource: brResourcesWithMarginalia.length,
		declare_truth: null,
		ask_question: null,
		reveal_secret: revealableAssets.length,
	});
	const atCap = (key: string): boolean => {
		const cap = optionCap[key];
		return cap != null && (counts[key] ?? 0) >= cap;
	};
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
				<textarea rows={3} bind:value={prepNotes} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
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
			     The narration is the preparer's own; only they can write it.
			     Everyone else sees the same area as a greyed-out, read-only mirror. -->
			{@const canNarrate = isPreparer}
			<fieldset class="resolve-mirror-wrap" class:resolve-mirror={!canNarrate} disabled={!canNarrate}>
				<p class="choices-header">What have you learned?</p>
				<label class="form-label">
					<textarea rows={4} bind:value={preRollText} class="form-textarea" maxlength={TEXT_LIMITS.LONG_TEXT}
						placeholder="Restate your research methods, and describe one thing you've learned while researching."
					></textarea>
				</label>
				{#if canNarrate}
					<button class="action-btn primary"
						onclick={() => submitPreRoll(plan)}
						disabled={preRollBusy || !preRollText.trim()}>
						{preRollBusy ? '…' : 'Submit'}
					</button>
				{/if}
			</fieldset>
			{#if !canNarrate}
				<p class="ft-prompt muted">
					{playerName(players, plan.preparer_id)} is researching…
				</p>
			{/if}

		{:else if rollActive && !choicesDone}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome != null && !choicesDone}
			<!-- Option-picking. The choice actor (preparer, or a perform_steps
			     demand winner acting in their place) picks; everyone else sees the
			     same option list greyed out. -->
			{@const canPick = amChoiceActor}
			{@const choiceActorName = playerName(players, performStepsWinnerID ?? plan.preparer_id)}
			<fieldset class="resolve-mirror-wrap" class:resolve-mirror={!canPick} disabled={!canPick}>
				<div class="choices-section">
					<p class="choices-header">
						Result: <span class="outcome-{rollOutcome}">
							{rollOutcome === 'make' ? '✓ Make' : '✗ Mar'}
						</span>
					</p>
					<p class="choices-note">
						Pick options equal to your dice result (repeatable){#if requiredPicks != null}: choose exactly {requiredPicks}{/if}.
						On a mar you must also describe a flaw in (break) your own resource assets, after this step.
					</p>
					{#each OPTIONS as opt}
						{@const available = optionAvailable[opt.key]}
						<div class="stepper-row" class:unavailable={!available}>
							<button class="action-btn" onclick={() => bump(opt.key, -1)}
								disabled={(counts[opt.key] ?? 0) === 0}>−</button>
							<strong style="min-width:1.5rem;text-align:center;">{counts[opt.key] ?? 0}</strong>
							<button class="action-btn" onclick={() => bump(opt.key, 1)}
								disabled={!available || atCap(opt.key) || (requiredPicks != null && totalPicked >= requiredPicks)}>+</button>
							<span>{opt.label}{#if !available}<span class="choices-note muted"> — no valid targets</span>{:else if atCap(opt.key)}<span class="choices-note muted"> — all targets picked</span>{/if}</span>
						</div>
					{/each}
					<p class="choices-note">
						Total picks: <strong>{totalPicked}</strong>{#if requiredPicks != null} / {requiredPicks}{/if}
					</p>
					{#if canPick}
						<button class="action-btn primary"
							onclick={() => onApplyChoices(plan, rollOutcome!)}
							disabled={choicesBusy || totalPicked === 0 || (requiredPicks != null && totalPicked !== requiredPicks)}>
							{choicesBusy ? '…' : 'Apply choices'}
						</button>
					{/if}
				</div>
			</fieldset>
			{#if !canPick}
				<p class="ft-prompt muted">
					{choiceActorName} is choosing make/mar options…
				</p>
			{/if}

		{:else if pendingQuestion != null && pendingQuestion.target_id === currentPlayerID}
			<!-- The questioned player answers truthfully — or vetoes, if they
			     outrank the preparer on knowledge. -->
			<div class="plan-form">
				<p class="choices-header">
					{playerName(players, plan.preparer_id)} asks you a question:
				</p>
				<blockquote class="q-quote">{pendingQuestion.question}</blockquote>
				<label class="form-label">
					Your truthful answer:
					<textarea rows={3} bind:value={answerText} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
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

		{:else if choicesDone}
			<!-- Sub-flow resolution. Only the preparer works the committed make/mar
			     list; everyone else sees the same area greyed out and read-only.
			     The pickers and buttons are blocked by the disabled fieldset, and
			     the cards take readOnly (they're role=checkbox, not form controls). -->
			{@const canSubflow = isPreparer}
			<div class="complete-section">
				<!-- Committed state (Tier-1, ADR-006): the picked plan and the
				     specifics of every step done so far, shown at full opacity to
				     EVERY viewer so the made choices persist after a picker commits.
				     Sits outside the greyed picker fieldset, like Propose Decree's
				     live law text. -->
				<ChoicesApplied choices={existingChoices} options={OPTIONS} />
				{#if anyResolved}
					<div class="sa-resolved">
						<p class="sa-resolved-label">Resolved so far:</p>
						<ul class="sa-resolved-list">
							{#each makeBrokenIDs as id}
								<li>Broke <em>{assetName(id)}</em></li>
							{/each}
							{#each revealedAssetIDs as id}
								<li>Learned the secrets of <em>{assetName(id)}</em></li>
							{/each}
							{#each declaredTruths as t}
								<li>Declared true: “{t}”</li>
							{/each}
							{#each answeredQuestions as q}
								<li>
									Asked {playerName(players, q.target_id)}: “{q.question}” —
									answered “{q.answer}”
								</li>
							{/each}
							{#each penaltyBrokenIDs as id}
								<li>Broke own resource, <em>{assetName(id)}</em></li>
							{/each}
						</ul>
					</div>
				{/if}

				<fieldset class="resolve-mirror-wrap" class:resolve-mirror={!canSubflow} disabled={!canSubflow}>

					{#if brRemaining > 0}
						<div class="plan-form">
							<p class="choices-header">
								Break a resource ({brRemaining} remaining)
							</p>
							<CardPicker
								label="Marginalium to tear"
								items={brResourcesWithMarginalia}
								{players}
								readOnly={!canSubflow}
								emptyMessage="No resource can be broken."
								ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
								marginaliaMode
								selectedMarginaliaID={brMargID}
								selectedAssetID={brAssetID}
								onSelectMarginalia={(mID, parent) => {
									brMargID = mID;
									brAssetID = parent?.id ?? null;
								}}
							/>
							{#if brWarn}<p class="res-warning">{brWarn}</p>{/if}
							{#if canSubflow}
								{#if brResourcesWithMarginalia.length === 0}
									<p class="choices-note muted">No resource can be broken — this pick has no valid target.</p>
									<button class="action-btn primary"
										onclick={() => forfeitStep(plan, 'break_resource')} disabled={forfeitBusy}>
										{forfeitBusy ? '…' : 'Skip — no valid targets'}
									</button>
								{:else}
									<button class="action-btn primary"
										onclick={() => submitBreakResource(plan)}
										disabled={brBusy || brAssetID == null || brMargID == null}>
										{brBusy ? '…' : 'Break resource'}
									</button>
								{/if}
							{/if}
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
								readOnly={!canSubflow}
								emptyMessage="No assets available."
								ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
								selected={rsAssetID}
								onSelect={(id) => (rsAssetID = id)}
							/>
							{#if canSubflow}
								{#if revealableAssets.length === 0}
									<p class="choices-note muted">No asset's secrets remain to learn — this pick has no valid target.</p>
									<button class="action-btn primary"
										onclick={() => forfeitStep(plan, 'reveal_secret')} disabled={forfeitBusy}>
										{forfeitBusy ? '…' : 'Skip — no valid targets'}
									</button>
								{:else}
									<button class="action-btn primary"
										onclick={() => submitRevealSecret(plan)}
										disabled={rsBusy || rsAssetID == null}>
										{rsBusy ? '…' : 'Reveal secrets'}
									</button>
								{/if}
							{/if}
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
								<textarea rows={2} bind:value={dtText} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
									placeholder="Declare something true about the world…"></textarea>
							</label>
							{#if canSubflow}
								<button class="action-btn primary"
									onclick={() => submitDeclareTruth(plan)}
									disabled={dtBusy || !dtText.trim()}>
									{dtBusy ? '…' : 'Declare truth'}
								</button>
							{/if}
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
									{#if pendingQuestion.vetoable}or veto {/if}{canSubflow ? 'your' : 'the'} question:
								</p>
								<blockquote class="q-quote">{pendingQuestion.question}</blockquote>
							{:else}
								{#if askVetoedHint && canSubflow}
									<p class="choices-note muted">
										Your last question was vetoed — ask another (it can't be vetoed).
									</p>
								{/if}
								<PlayerChips
									players={questionTargets}
									isActive={(p) => p.id === aqTargetID}
									onSelect={(p) => (aqTargetID = p.id)}
									readOnly={!canSubflow}
								/>
								<label class="form-label">
									Your question:
									<textarea rows={2} bind:value={aqQuestion} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
										placeholder="Ask a question they must answer truthfully…"></textarea>
								</label>
								{#if canSubflow}
									<button class="action-btn primary"
										onclick={() => submitAskQuestion(plan)}
										disabled={aqBusy || aqTargetID == null || !aqQuestion.trim()}>
										{aqBusy ? '…' : 'Ask question'}
									</button>
								{/if}
							{/if}
						</div>
					{/if}

					{#if marRemaining > 0}
						<!-- The mar penalty (flaw your own resources) is listed last so the
						     make-list tasks are settled before this consequence. -->
						<div class="plan-form">
							<p class="choices-header">
								Describe a flaw in your own resource assets
							</p>
							<p class="choices-note">
								The plan marred by {marRequired} — {canSubflow ? 'you' : 'the preparer'} must break {marRequired} of {canSubflow ? 'your' : 'their'} own resources.
							</p>
							<CardPicker
								label="Choose a marginallia to tear"
								items={penaltyResources}
								{players}
								readOnly={!canSubflow}
								emptyMessage="No eligible resources of your own remain."
								marginaliaMode
								selectedMarginaliaID={maMargID}
								selectedAssetID={maAssetID}
								onSelectMarginalia={(mID, parent) => {
									maMargID = mID;
									maAssetID = parent?.id ?? null;
								}}
							/>
							{#if maWarn}<p class="res-warning">{maWarn}</p>{/if}
							{#if canSubflow}
								{#if penaltyResources.length === 0}
									<p class="choices-note muted">None of your resources can be broken — this penalty can't be applied.</p>
									<button class="action-btn primary"
										onclick={() => forfeitStep(plan, 'mar_penalty')} disabled={forfeitBusy}>
										{forfeitBusy ? '…' : 'Skip — no valid targets'}
									</button>
								{:else}
									<button class="action-btn primary"
										onclick={() => submitPenaltyFlaw(plan)}
										disabled={maBusy || maAssetID == null || maMargID == null}>
										{maBusy ? '…' : 'Break your resource'}
									</button>
								{/if}
							{/if}
						</div>
					{/if}

					{#if subflowsDone && canSubflow}
						<p class="complete-note">
							Bring the scene to a close, then complete the plan.
						</p>
						<button class="action-btn primary"
							onclick={() => onComplete(plan)} disabled={resBusy}>
							{resBusy ? '…' : 'Complete plan'}
						</button>
					{/if}
				</fieldset>
				{#if !canSubflow}
					<p class="ft-prompt muted">
						{#if pendingQuestion != null}
							Waiting for {playerName(players, pendingQuestion.target_id)} to answer
							{playerName(players, plan.preparer_id)}'s question…
						{:else}
							{playerName(players, plan.preparer_id)} is resolving Seek Answers…
						{/if}
					</p>
				{/if}
			</div>

		{:else}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is resolving Seek Answers…
			</p>
		{/if}
	</ResolvingCard>
{/if}

<style>
	.stepper-row.unavailable { opacity: 0.55; }

	/* Transparent fieldset wrapping a resolve phase. disabled blocks every
	   descendant control for free; CardPicker / PlayerChips take readOnly
	   separately (their cards are role=checkbox, not form controls). */
	.resolve-mirror-wrap {
		border: none;
		padding: 0;
		margin: 0;
		min-width: 0;
	}
	/* Greyed, non-interactive mirror for players waiting on the actor. */
	.resolve-mirror-wrap.resolve-mirror {
		opacity: 0.55;
		pointer-events: none;
	}

	/* Committed-state summary (Tier-1): the choices already made, shown plainly
	   to every viewer — kept at full opacity, outside the greyed picker area. */
	.sa-resolved { margin: 0; }
	.sa-resolved-label {
		margin: 0;
		font-size: 0.82rem;
		color: var(--color-text-muted);
	}
	.sa-resolved-list {
		margin: 0.25rem 0 0 1.1rem;
		padding: 0;
		list-style: disc;
		font-size: 0.85rem;
	}
	.sa-resolved-list li { margin: 0.2rem 0; }
	.sa-resolved-list em { font-style: italic; }
</style>
