<!-- SpreadPropagandaPanel.svelte
  Prep + resolve UI for the Spread Propaganda plan.
  Resolve flow: pre-roll narration → dice roll → make/mar choices → complete.
  The make's artifact step also carries a short narration of the changes society
  undergoes (book p.58) — one step, no extra gate.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan, getRoll, castSpreadPropagandaRoll,
		type Plan, type DiceRollDie,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import MakeMarPicker from './MakeMarPicker.svelte';
	import ChoicesApplied from './ChoicesApplied.svelte';
	import CardPicker from './CardPicker.svelte';
	import AssetCreationForm from '../AssetCreationForm.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import PreRollStep from './shared/PreRollStep.svelte';
	import {
		MAKE_OPTIONS, MAR_OPTIONS, parseResolutionData, playerName,
		breakableAssets, breaksAvailable, playersExcept,
	} from './shared';
	import { spGivePeer, spBreakSelf, createArtifact, forfeitSpreadPropagandaStep } from '$lib/api';
	import { destructionWarning } from '$lib/assetRisk';

	import type { PlanPanelProps } from './types';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	// make and mar option keys don't overlap, so a completed choice list can
	// be labelled without knowing which outcome produced it.
	const SP_ALL_OPTIONS = [
		...(MAKE_OPTIONS.spread_propaganda ?? []),
		...(MAR_OPTIONS.spread_propaganda ?? []),
	];

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
	const prepDraft = $derived(ctx.prepDraft as { notes?: string } | null);

	let performStepsWinnerID = $state<number | null>(null);
	// The preparer resolves their own plan; the perform_steps demand winner may
	// drive the choice in their place.
	const isPreparer = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const amChoiceActor = $derived(
		isPreparer || (currentPlayerID != null && currentPlayerID === performStepsWinnerID),
	);

	// Prep state
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	async function submitPrep() {
		if (prepBusy) return;
		if (!prepNotes.trim()) { prepError = 'Preparation notes are required.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'spread_propaganda',
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

	const spData = $derived(plan ? (parseResolutionData(plan).spread_propaganda ?? {}) : {});

	// ── Pre-roll: summarise the message, say how it is spreading ─────────────
	// OnResolve parks the plan with no roll; the preparer's narration is posted for
	// the table (who read it to decide whether to help or interfere) and only then
	// does the roll open. Plans that rolled before the step existed carry no flag, so
	// "a roll exists for this plan" counts as done too — as does a counter-propaganda
	// plan, which is born rolling.
	const rollForThisPlan = $derived(plan != null && activeRoll != null && activeRoll.plan_id === plan.id);
	const hasCommittedChoices = $derived(
		plan != null && (parseResolutionData(plan).make_mar_choices ?? []).length > 0,
	);
	const preRollDone = $derived(
		(spData.pre_roll_done ?? false) || rollForThisPlan || hasCommittedChoices,
	);
	let preRollBusy = $state(false);
	async function submitPreRoll(p: Plan, values: Record<string, string>) {
		if (preRollBusy) return;
		preRollBusy = true; resError = '';
		try {
			const res = await castSpreadPropagandaRoll(p.id, (values.narration ?? '').trim());
			if (res.roll) ctx.onRollCreated(res.roll);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not open the roll.';
		} finally { preRollBusy = false; }
	}

	// Resolve state — make offers a single required option (create_artifact),
	// so it keeps the plain toggle picker. Mar is repeatable ("Choose options
	// equal to (difficulty − result) (repeatable)"), so it gets its own
	// counts/stepper picker below.
	let resError = $state('');
	let resBusy = $state(false);
	let selectedChoices = $state<string[]>([]); // make only
	let choicesBusy = $state(false);

	function toggleChoice(key: string) {
		selectedChoices = selectedChoices.includes(key)
			? selectedChoices.filter(k => k !== key)
			: [...selectedChoices, key];
	}

	// give_peer / break_self are genuinely repeatable — each pick owes a
	// separate sub-flow step (a peer handed over, an asset broken) — but only up
	// to what there is to spend it on. lay_low / counter_prop fire a single
	// immediate effect (an esteem lockout flag; a guarded one-shot recursive
	// plan), so a second pick would just spend budget for nothing.
	//
	// Every option is therefore capped at its live target count (marCap below),
	// and greyed with a reason when it has none — the same shape as Seek Answers'
	// picker. An uncapped picker let three give_peer picks commit against one
	// peer, after which /complete refused forever ("2 of 3 give-peer pick(s)
	// still to resolve") with nothing at the table able to satisfy them.
	let marCounts = $state<Record<string, number>>({
		give_peer: 0, lay_low: 0, break_self: 0, counter_prop: 0,
	});
	const marTotalPicked = $derived(Object.values(marCounts).reduce((a, b) => a + b, 0));
	const marRequiredPicks = $derived.by(() => {
		const result = activeRoll?.result;
		const difficulty = activeRoll?.adjusted_difficulty ?? activeRoll?.difficulty ?? null;
		if (result == null || difficulty == null) return null;
		return Math.max(0, difficulty - result);
	});

	// The preparer's retinue and breakables — what give_peer / break_self spend
	// picks on. Mirrors spEligibleGivePeers / spEligibleSelfBreaks on the server.
	// A blank asset counts as one break (it is destroyed outright); see
	// breakableAssets / adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md D3.
	const preparerPeers = $derived(
		plan ? assets.filter(a => a.owner_id === plan.preparer_id && a.asset_type === 'peer' && !a.is_destroyed) : []
	);
	const preparerBreakable = $derived(plan ? breakableAssets(assets, plan.preparer_id) : []);
	const breakSelfCap = $derived(preparerBreakable.reduce((n, a) => n + breaksAvailable(a), 0));

	// counter_prop needs someone to have interfered with THIS roll: the top
	// interferer is who spreads propaganda of their own. The dice aren't on the
	// plan context, so fetch them once the mar picker is up. Fail OPEN on a fetch
	// error — the server re-checks and stays the authority; failing closed would
	// strand a legitimate pick behind a flaky request.
	let rollDice = $state<{ rollID: number; dice: DiceRollDie[] | null } | null>(null);
	$effect(() => {
		const rollID = activeRoll != null && activeRoll.plan_id === plan?.id ? activeRoll.id : null;
		if (rollID == null || rollOutcome !== 'mar' || !amChoiceActor || choicesDone) return;
		let stale = false;
		getRoll(rollID)
			.then(d => { if (!stale) rollDice = { rollID, dice: d.dice }; })
			.catch(() => { if (!stale) rollDice = { rollID, dice: null }; });
		return () => { stale = true; };
	});
	/** Why counter_prop can't be picked right now, or null when it can. */
	const coOptBlocker = $derived.by((): string | null => {
		if (spData.original_plan_id != null) return 'not available on counter-propaganda';
		if (rollDice == null || rollDice.rollID !== activeRoll?.id) return 'checking who interfered…';
		if (rollDice.dice == null) return null;
		return rollDice.dice.some(d => d.is_interference) ? null : 'nobody interfered with this roll';
	});

	const marCap = $derived<Record<string, number>>({
		lay_low: 1,
		counter_prop: coOptBlocker == null ? 1 : 0,
		give_peer: preparerPeers.length,
		break_self: breakSelfCap,
	});
	const marCapacity = $derived(Object.values(marCap).reduce((a, b) => a + b, 0));
	// "Choose options equal to (difficulty − result)" — but if fewer picks have a
	// valid target than that, the preparer takes every one they can. Requiring the
	// full count would leave Apply disabled for good once the caps bind.
	const marPicksNeeded = $derived(
		marRequiredPicks == null ? null : Math.min(marRequiredPicks, marCapacity),
	);
	function marNote(key: string): string | null {
		const cap = marCap[key] ?? 0;
		if (cap === 0) {
			if (key === 'give_peer') return 'no peers to give';
			if (key === 'break_self') return 'nothing of yours left to break';
			if (key === 'counter_prop') return coOptBlocker;
			return null;
		}
		if ((marCounts[key] ?? 0) >= cap) {
			return key === 'lay_low' || key === 'counter_prop' ? 'already chosen' : 'all targets picked';
		}
		return null;
	}
	function bumpMar(key: string, delta: number) {
		if (delta > 0 && (marCounts[key] ?? 0) >= (marCap[key] ?? 0)) return;
		if (delta > 0 && marPicksNeeded != null && marTotalPicked >= marPicksNeeded) return;
		const next = Math.max(0, (marCounts[key] ?? 0) + delta);
		marCounts = { ...marCounts, [key]: next };
	}
	function marFlatChoices(): string[] {
		const flat: string[] = [];
		for (const opt of MAR_OPTIONS.spread_propaganda ?? []) {
			for (let i = 0; i < (marCounts[opt.key] ?? 0); i++) flat.push(opt.key);
		}
		return flat;
	}

	async function onApplyChoices(p: Plan, outcome: 'make' | 'mar') {
		if (choicesBusy) return;
		choicesBusy = true; resError = '';
		try {
			await makeChoice(p.id, outcome, outcome === 'make' ? selectedChoices : marFlatChoices());
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

	const existingChoices = $derived((parseResolutionData(plan).make_mar_choices ?? []).map(c => c.option));
	const choicesDone = $derived(existingChoices.length > 0);

	// ── Mar asset-picker effects (driven by the preparer) ─────────────────────
	// give_peer (a) and break_self (c) are repeatable: the preparer picks a
	// peer/marginalia to hand over or break once per pick. "Owed" comes from
	// how many times the option was picked (server-authoritative, mirrors
	// pickedChoiceCount on the backend) versus how many have been performed.
	const givePeerPicked = $derived(existingChoices.filter(c => c === 'give_peer').length);
	const givePeerDone = $derived(spData.give_peer_done ?? 0);
	const needsGivePeer = $derived(givePeerDone < givePeerPicked);
	const breakSelfPicked = $derived(existingChoices.filter(c => c === 'break_self').length);
	const breakSelfDone = $derived(spData.break_self_done ?? 0);
	const needsBreakSelf = $derived(breakSelfDone < breakSelfPicked);
	const marEffectsPending = $derived(needsGivePeer || needsBreakSelf);
	// A made plan requires the preparer to author the artifact before completion.
	const artifactPending = $derived(!!spData.artifact_required && spData.artifact_id == null);
	// Everything the preparer must still do before the plan can be completed.
	const preparerActionPending = $derived(marEffectsPending || artifactPending);
	// Who performs those steps: the preparer, or — after a Make Demands
	// perform_steps win — the winner, in their place (the server refuses the
	// preparer every one of them). The retinue and assets are still the
	// PREPARER's, so copy that says "your" has to say whose when they aren't.
	const stepActorName = $derived(plan ? playerName(players, performStepsWinnerID ?? plan.preparer_id) : '');
	const whose = $derived(isPreparer || !plan ? 'your' : `${playerName(players, plan.preparer_id)}'s`);
	const whoHas = $derived(isPreparer || !plan ? 'You have' : `${playerName(players, plan.preparer_id)} has`);

	// give_peer: pick one of the preparer's peers and a recipient player.
	let givePeerID = $state<number | null>(null);
	let giveToPlayerID = $state<number | null>(null);
	let giveBusy = $state(false);
	async function submitGivePeer(p: Plan) {
		if (giveBusy || givePeerID == null || giveToPlayerID == null) return;
		giveBusy = true; resError = '';
		try {
			await spGivePeer(p.id, givePeerID, giveToPlayerID);
			givePeerID = null; giveToPlayerID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not give up peer.';
		} finally { giveBusy = false; }
	}

	// break_self: break one of the preparer's own assets — tear a marginalia, or
	// (blank asset, picked with the sentinel id 0) destroy it outright.
	let breakMargID = $state<number | null>(null);
	let breakAssetID = $state<number | null>(null);
	let breakBusy = $state(false);
	const breakWarn = $derived(destructionWarning(assets.find(a => a.id === breakAssetID)));
	async function submitBreakSelf(p: Plan) {
		if (breakBusy || breakMargID == null || breakAssetID == null) return;
		breakBusy = true; resError = '';
		try {
			await spBreakSelf(p.id, breakAssetID, breakMargID);
			breakMargID = null; breakAssetID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not break asset.';
		} finally { breakBusy = false; }
	}

	// Discharge a depletable step that has picks owed but no live target — no peer
	// left to give, nothing left to break. The server re-checks that no target
	// exists, so this only resolves a genuine dead end. Without it the row wedges:
	// completion is gated on every committed pick being performed or forfeited.
	let forfeitBusy = $state(false);
	async function forfeitStep(p: Plan, step: 'give_peer' | 'break_self') {
		if (forfeitBusy) return;
		forfeitBusy = true; resError = '';
		try {
			await forfeitSpreadPropagandaStep(p.id, step);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not skip step.';
		} finally { forfeitBusy = false; }
	}

	// ── Author the societal-shift artifact (preparer) ─────────────────────────
	// A made plan requires the preparer to author the artifact: create-artifact
	// creates it under the chosen name in one transaction (no placeholder). It is
	// required — it gates completion until the artifact exists.
	const needsArtifactCreation = $derived(amChoiceActor && artifactPending);
	// The make also asks the player to "describe the changes that come from this":
	// that narration rides in the same request as the artifact, so there is no
	// second gate for it (principle 3).
	let changesText = $state('');
	let artifactName = $state('');
	let artifactMarginalia = $state('');
	let nameBusy = $state(false);
	async function submitArtifactName(p: Plan) {
		if (nameBusy || !changesText.trim() || !artifactName.trim() || !artifactMarginalia.trim()) return;
		nameBusy = true; resError = '';
		try {
			await createArtifact(p.id, changesText.trim(), artifactName.trim(), [artifactMarginalia.trim()]);
			changesText = '';
			artifactName = '';
			artifactMarginalia = '';
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not author the artifact.';
		} finally { nameBusy = false; }
	}
</script>

{#if mode === 'prep'}
	<fieldset class="plan-form-fieldset" disabled={readOnly}>
		<div class="plan-form">
			{#if prepError}<ErrorText message={prepError} variant="panel" />{/if}
			<label class="form-label">
				Message and Methods:
				<textarea rows={2} bind:value={prepNotes} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
					placeholder="What are you spreading through the realm, and how? Distributing pamphlets? Giving sermons? Feeding talking points to town criers?" required></textarea>
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
	<ResolvingCard {plan} {players} error={resError} showNotes={false}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		<!-- The message. Before the pre-roll the prep note is only a draft (principle
		     2) and is labelled so; after it, the stated version replaces it. -->
		{#if preRollDone && spData.pre_roll_text}
			<p class="choices-note muted">The message, as stated:</p>
			<p class="plan-notes">"{spData.pre_roll_text}"</p>
		{:else if plan.preparation_notes && (preRollDone || !isPreparer)}
			<p class="choices-note muted">{preRollDone ? 'As prepared:' : 'As prepared — a draft, restated before the roll:'}</p>
			<p class="plan-notes">"{plan.preparation_notes}"</p>
		{/if}

		{#if !preRollDone}
			<PreRollStep
				title="Spread your message"
				prompt="Summarize the message you want the public to take from your propaganda and describe again how you've been spreading it."
				fields={[{
					key: 'narration',
					label: 'The message and how it spreads',
					placeholder: 'What should the public take from this, and how have you been getting it out?',
					draft: plan.preparation_notes,
					rows: 4,
					max: TEXT_LIMITS.LONG_TEXT,
				}]}
				canAct={isPreparer}
				busy={preRollBusy}
				actorName={playerName(players, plan.preparer_id)}
				waitingVerb="is about to say how they're spreading their message"
				onSubmit={(v) => submitPreRoll(plan, v)}
			/>

		{:else if rollActive && !choicesDone}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome === 'make' && !choicesDone && amChoiceActor}
			<MakeMarPicker
				outcome="make"
				options={MAKE_OPTIONS.spread_propaganda ?? []}
				selected={selectedChoices}
				busy={choicesBusy}
				onToggle={toggleChoice}
				onSubmit={() => onApplyChoices(plan, 'make')}
			/>

		{:else if rollOutcome === 'mar' && !choicesDone && amChoiceActor}
			<div class="choices-section">
				<p class="choices-header">
					Result: <span class="outcome-mar">✗ Mar</span>
				</p>
				<p class="choices-note">
					Pick options equal to (difficulty − result) (repeatable){#if marPicksNeeded != null}: choose exactly {marPicksNeeded}{/if}.
				</p>
				{#if marRequiredPicks != null && marPicksNeeded != null && marPicksNeeded < marRequiredPicks}
					<p class="choices-note muted">
						Only {marPicksNeeded} of the {marRequiredPicks} picks have a valid target, so take every one you can.
					</p>
				{/if}
				{#each MAR_OPTIONS.spread_propaganda ?? [] as opt}
					{@const note = marNote(opt.key)}
					<div class="stepper-row" class:unavailable={(marCap[opt.key] ?? 0) === 0}>
						<button class="action-btn" onclick={() => bumpMar(opt.key, -1)}
							disabled={(marCounts[opt.key] ?? 0) === 0}>−</button>
						<strong style="min-width:1.5rem;text-align:center;">{marCounts[opt.key] ?? 0}</strong>
						<button class="action-btn" onclick={() => bumpMar(opt.key, 1)}
							disabled={(marCounts[opt.key] ?? 0) >= (marCap[opt.key] ?? 0) || (marPicksNeeded != null && marTotalPicked >= marPicksNeeded)}>+</button>
						<span>{opt.label}{#if note}<span class="choices-note muted"> — {note}</span>{/if}</span>
					</div>
				{/each}
				<p class="choices-note">
					Total picks: <strong>{marTotalPicked}</strong>{#if marPicksNeeded != null} / {marPicksNeeded}{/if}
				</p>
				<button class="action-btn primary"
					onclick={() => onApplyChoices(plan, 'mar')}
					disabled={choicesBusy || marTotalPicked === 0 || (marPicksNeeded != null && marTotalPicked !== marPicksNeeded)}>
					{choicesBusy ? '…' : 'Apply choices'}
				</button>
			</div>

		{:else if choicesDone}
			<div class="complete-section">
				<ChoicesApplied choices={existingChoices} options={SP_ALL_OPTIONS} />

				{#if needsArtifactCreation}
					<div class="plan-form">
						<p class="choices-header">Your message sets society ablaze</p>
						<p class="choices-note">
							Describe the changes that come from this, and create an artifact that
							embodies the shift in society.
						</p>
						<label class="form-label">
							<span class="form-label-text">The changes society undergoes</span>
							<textarea rows={3} bind:value={changesText} class="form-textarea"
								maxlength={TEXT_LIMITS.LONG_TEXT} disabled={nameBusy}
								placeholder="How is the realm different now?"></textarea>
						</label>
						<AssetCreationForm
							{gameID}
							assetType="artifact"
							bind:name={artifactName}
							bind:marginalia={artifactMarginalia}
							disabled={nameBusy}
						/>
						<button class="action-btn primary" onclick={() => submitArtifactName(plan)}
							disabled={nameBusy || !changesText.trim() || !artifactName.trim() || !artifactMarginalia.trim()}>
							{nameBusy ? '…' : 'Create artifact'}
						</button>
					</div>
				{/if}

				{#if amChoiceActor && needsGivePeer}
					<div class="plan-form">
						<p class="choices-header">
							A peer leaves {whose} retinue — hand it to another player
							{#if givePeerPicked > 1}({givePeerPicked - givePeerDone} of {givePeerPicked} remaining){/if}
						</p>
						<CardPicker
							label="Peer to give up"
							items={preparerPeers}
							{players}
							emptyMessage="{whoHas} no peers to give."
							selected={givePeerID}
							onSelect={(id) => (givePeerID = id)}
						/>
						{#if preparerPeers.length === 0}
							<p class="choices-note muted">Nothing left to give — this pick has no valid target.</p>
							<button class="action-btn primary"
								onclick={() => forfeitStep(plan, 'give_peer')} disabled={forfeitBusy}>
								{forfeitBusy ? '…' : 'Skip — no valid targets'}
							</button>
						{:else}
							<PlayerChips
								players={playersExcept(players, plan.preparer_id)}
								isActive={(pl) => giveToPlayerID === pl.id}
								onSelect={(pl) => (giveToPlayerID = pl.id)}
							/>
							<button class="action-btn primary" onclick={() => submitGivePeer(plan)}
								disabled={giveBusy || givePeerID == null || giveToPlayerID == null}>
								{giveBusy ? '…' : 'Give up peer'}
							</button>
						{/if}
					</div>
				{/if}

				{#if amChoiceActor && needsBreakSelf}
					<div class="plan-form">
						<p class="choices-header">
							Break yourself — break one of {whose} assets
							{#if breakSelfPicked > 1}({breakSelfPicked - breakSelfDone} of {breakSelfPicked} remaining){/if}
						</p>
						<CardPicker
							label="Marginalium to tear"
							items={preparerBreakable}
							{players}
							emptyMessage="{whoHas} nothing left to break."
							marginaliaMode
							selectedMarginaliaID={breakMargID}
							selectedAssetID={breakAssetID}
							onSelectMarginalia={(mID, parent) => {
								breakMargID = mID;
								breakAssetID = parent?.id ?? null;
							}}
						/>
						{#if breakWarn}<p class="res-warning">{breakWarn}</p>{/if}
						{#if preparerBreakable.length === 0}
							<p class="choices-note muted">Nothing left to break — this pick has no valid target.</p>
							<button class="action-btn primary"
								onclick={() => forfeitStep(plan, 'break_self')} disabled={forfeitBusy}>
								{forfeitBusy ? '…' : 'Skip — no valid targets'}
							</button>
						{:else}
							<button class="action-btn primary" onclick={() => submitBreakSelf(plan)}
								disabled={breakBusy || breakMargID == null}>
								{breakBusy ? '…' : 'Break asset'}
							</button>
						{/if}
					</div>
				{/if}

				{#if isPreparer && !preparerActionPending}
					<p class="complete-note">
						Write any follow-scene narration in the chat, then complete the plan.
					</p>
					<button class="action-btn primary" onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				{:else if preparerActionPending && !amChoiceActor}
					<p class="ft-prompt muted">
						Waiting for {stepActorName} to {artifactPending && !marEffectsPending ? 'author the artifact' : 'face the fallout'}…
					</p>
				{:else if !preparerActionPending && !isPreparer}
					<p class="ft-prompt muted">
						Waiting for {playerName(players, plan.preparer_id)} to complete the plan…
					</p>
				{/if}
			</div>

		{:else if !amChoiceActor && !choicesDone}
			<p class="ft-prompt muted">
				{playerName(players, plan.preparer_id)} is resolving Spread Propaganda…
			</p>
		{/if}
	</ResolvingCard>
{/if}
