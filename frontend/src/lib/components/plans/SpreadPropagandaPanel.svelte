<!-- SpreadPropagandaPanel.svelte
  Prep + resolve UI for the Spread Propaganda plan.
  Resolve flow: dice roll → make/mar choices → complete.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import './planPanel.css';
	import {
		preparePlan, makeChoice, completePlan,
		type Plan, type Asset, type Player, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import MakeMarPicker from './MakeMarPicker.svelte';
	import ChoicesApplied from './ChoicesApplied.svelte';
	import CardPicker from './CardPicker.svelte';
	import AssetCreationForm from '../AssetCreationForm.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import {
		MAKE_OPTIONS, MAR_OPTIONS, parseResolutionData, playerName,
		assetsWithIntactMarginalia, playersExcept,
	} from './shared';
	import { spGivePeer, spBreakSelf, createArtifact } from '$lib/api';

	import type { PlanPanelProps } from './types';

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
	// separate sub-flow step (a peer handed over, a marginalia broken).
	// lay_low / counter_prop fire a single immediate effect (an esteem
	// lockout flag; a guarded one-shot recursive plan), so a second pick
	// would just spend budget for nothing — capped at 1 in the picker.
	const MAR_SINGLE_FIRE = new Set(['lay_low', 'counter_prop']);
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
	function bumpMar(key: string, delta: number) {
		if (delta > 0 && MAR_SINGLE_FIRE.has(key) && (marCounts[key] ?? 0) >= 1) return;
		if (delta > 0 && marRequiredPicks != null && marTotalPicked >= marRequiredPicks) return;
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
	const spData = $derived(plan ? (parseResolutionData(plan).spread_propaganda ?? {}) : {});
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

	// give_peer: pick one of the preparer's peers and a recipient player.
	const preparerPeers = $derived(
		plan ? assets.filter(a => a.owner_id === plan.preparer_id && a.asset_type === 'peer' && !a.is_destroyed) : []
	);
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

	// break_self: tear one marginalia from one of the preparer's own assets.
	const breakSelfAssets = $derived(
		plan ? assetsWithIntactMarginalia(assets, plan.preparer_id) : []
	);
	let breakMargID = $state<number | null>(null);
	let breakBusy = $state(false);
	async function submitBreakSelf(p: Plan) {
		if (breakBusy || breakMargID == null) return;
		breakBusy = true; resError = '';
		try {
			await spBreakSelf(p.id, breakMargID);
			breakMargID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not break asset.';
		} finally { breakBusy = false; }
	}

	// ── Author the societal-shift artifact (preparer) ─────────────────────────
	// A made plan requires the preparer to author the artifact: create-artifact
	// creates it under the chosen name in one transaction (no placeholder). It is
	// required — it gates completion until the artifact exists.
	const needsArtifactCreation = $derived(isPreparer && artifactPending);
	let artifactName = $state('');
	let artifactMarginalia = $state('');
	let nameBusy = $state(false);
	async function submitArtifactName(p: Plan) {
		if (nameBusy || !artifactName.trim() || !artifactMarginalia.trim()) return;
		nameBusy = true; resError = '';
		try {
			await createArtifact(p.id, artifactName.trim(), [artifactMarginalia.trim()]);
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
			{#if prepError}<p class="res-error">{prepError}</p>{/if}
			<label class="form-label">
				Message and Methods:
				<textarea rows={2} bind:value={prepNotes} class="form-textarea" maxlength={1000}
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
	<ResolvingCard {plan} {players} error={resError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID}
			bind:performStepsWinnerID />
		{#if rollActive && !choicesDone}
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
					Pick options equal to (difficulty − result) (repeatable){#if marRequiredPicks != null}: choose exactly {marRequiredPicks}{/if}.
				</p>
				{#each MAR_OPTIONS.spread_propaganda ?? [] as opt}
					{@const atCap = MAR_SINGLE_FIRE.has(opt.key) && (marCounts[opt.key] ?? 0) >= 1}
					<div class="stepper-row">
						<button class="action-btn" onclick={() => bumpMar(opt.key, -1)}
							disabled={(marCounts[opt.key] ?? 0) === 0}>−</button>
						<strong style="min-width:1.5rem;text-align:center;">{marCounts[opt.key] ?? 0}</strong>
						<button class="action-btn" onclick={() => bumpMar(opt.key, 1)}
							disabled={atCap || (marRequiredPicks != null && marTotalPicked >= marRequiredPicks)}>+</button>
						<span>{opt.label}</span>
					</div>
				{/each}
				<p class="choices-note">
					Total picks: <strong>{marTotalPicked}</strong>{#if marRequiredPicks != null} / {marRequiredPicks}{/if}
				</p>
				<button class="action-btn primary"
					onclick={() => onApplyChoices(plan, 'mar')}
					disabled={choicesBusy || marTotalPicked === 0 || (marRequiredPicks != null && marTotalPicked !== marRequiredPicks)}>
					{choicesBusy ? '…' : 'Apply choices'}
				</button>
			</div>

		{:else if choicesDone}
			<div class="complete-section">
				<ChoicesApplied choices={existingChoices} options={SP_ALL_OPTIONS} />

				{#if needsArtifactCreation}
					<div class="plan-form">
						<p class="choices-header">Author the artifact your propaganda created</p>
						<AssetCreationForm
							{gameID}
							assetType="artifact"
							bind:name={artifactName}
							bind:marginalia={artifactMarginalia}
							disabled={nameBusy}
						/>
						<button class="action-btn primary" onclick={() => submitArtifactName(plan)}
							disabled={nameBusy || !artifactName.trim() || !artifactMarginalia.trim()}>
							{nameBusy ? '…' : 'Create artifact'}
						</button>
					</div>
				{/if}

				{#if isPreparer && needsGivePeer}
					<div class="plan-form">
						<p class="choices-header">
							A peer leaves your retinue — hand it to another player
							{#if givePeerPicked > 1}({givePeerPicked - givePeerDone} of {givePeerPicked} remaining){/if}
						</p>
						<CardPicker
							label="Peer to give up"
							items={preparerPeers}
							{players}
							emptyMessage="You have no peers to give."
							selected={givePeerID}
							onSelect={(id) => (givePeerID = id)}
						/>
						<PlayerChips
							players={playersExcept(players, plan.preparer_id)}
							isActive={(pl) => giveToPlayerID === pl.id}
							onSelect={(pl) => (giveToPlayerID = pl.id)}
						/>
						<button class="action-btn primary" onclick={() => submitGivePeer(plan)}
							disabled={giveBusy || givePeerID == null || giveToPlayerID == null}>
							{giveBusy ? '…' : 'Give up peer'}
						</button>
					</div>
				{/if}

				{#if isPreparer && needsBreakSelf}
					<div class="plan-form">
						<p class="choices-header">
							Break yourself — tear a marginalia from one of your assets
							{#if breakSelfPicked > 1}({breakSelfPicked - breakSelfDone} of {breakSelfPicked} remaining){/if}
						</p>
						<CardPicker
							label="Marginalium to tear"
							items={breakSelfAssets}
							{players}
							emptyMessage="You have no intact marginalia."
							marginaliaMode
							selectedMarginaliaID={breakMargID}
							onSelectMarginalia={(mID) => (breakMargID = mID)}
						/>
						<button class="action-btn primary" onclick={() => submitBreakSelf(plan)}
							disabled={breakBusy || breakMargID == null}>
							{breakBusy ? '…' : 'Tear marginalia'}
						</button>
					</div>
				{/if}

				{#if isPreparer && !preparerActionPending}
					<p class="complete-note">
						Write any follow-scene narration in the chat, then complete the plan.
					</p>
					<button class="action-btn primary" onclick={() => onComplete(plan)} disabled={resBusy}>
						{resBusy ? '…' : 'Complete plan'}
					</button>
				{:else if preparerActionPending && !isPreparer}
					<p class="ft-prompt muted">
						Waiting for {playerName(players, plan.preparer_id)} to {artifactPending && !marEffectsPending ? 'author the artifact' : 'face the fallout'}…
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
