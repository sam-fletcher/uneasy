<!-- MakeDemandsPanel.svelte
  Prep + resolve UI for the Make Demands plan (Tier 3, Power, variable delay).

  Flow:
  - Prep: pick a target plan from the public record. The demand lands on
    the target's row, slotted in immediately before it (so it resolves
    first within the row); the picker shows that to the preparer.
  - Resolve: dice roll → make→draft picker (alternating between demander and
    target-plan preparer, higher power-rank picks first); mar→counter-demand
    picker visible to the target.

  Stage 4 effects (the four draft options) are *not* rendered here; they are
  rendered on the *target* plan via TargetPlanDemandOverlay. This panel only
  handles the demand plan's own resolution UI.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import './planPanel.css';
	import {
		preparePlan, completePlan,
		draftChoice, counterDemand,
		type Plan, type Asset, type Player, type Ranking, type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import {
		PLAN_SHORT, playerName, parseResolutionData,
		DEMAND_OPTIONS, DEMAND_OPTION_LABELS, type DemandOption,
	} from './shared';

	import type { PlanPanelProps } from './types';
	import { TEXT_LIMITS } from '$lib/textLimits';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const gameID = $derived(ctx.gameID);
	const players = $derived(ctx.players);
	const rankings = $derived(ctx.rankings);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const currentRow = $derived(ctx.currentRow);
	const plans = $derived(ctx.plans);
	const rollActive = $derived(ctx.rollActive);
	const rollOutcome = $derived(ctx.rollOutcome);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	const readOnly = $derived(ctx.readOnly);
	const prepDraft = $derived(ctx.prepDraft as {
		target_plan_id?: number | null;
		notes?: string;
	} | null);

	// ── Prep ─────────────────────────────────────────────────────────────────

	// A plan is targetable iff it is not own, not Make War, not already
	// resolved/cancelled, and not already targeted by an unresolved demand.
	const targetablePlans = $derived.by(() => {
		const targetedSet = new Set<number>();
		for (const p of plans) {
			if (p.plan_type !== 'make_demands') continue;
			if (p.targeted_plan_id == null) continue;
			if (p.status === 'resolved' || p.status === 'cancelled') continue;
			targetedSet.add(p.targeted_plan_id);
		}
		return plans.filter(p =>
			p.plan_type !== 'make_demands' &&  // demand-on-demand is allowed by backend, but not on Make War — included anyway
			p.plan_type !== 'make_war' &&
			p.preparer_id !== currentPlayerID &&
			// A demand slots in *before* its target within the same row, so a
			// target whose resolution has already started can't be demanded
			// against — there's no "before" left.
			p.status === 'pending' &&
			// Variable-delay plans awaiting their delay reveal have no row
			// yet; demand placement is derived from the target's row, so
			// they're not targetable until the reveal closes.
			p.row_number != null &&
			!targetedSet.has(p.id),
		);
	});

	let targetPlanID = $state<number | null>(null);
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	const selectedTarget = $derived(
		targetPlanID == null ? null : (targetablePlans.find(p => p.id === targetPlanID) ?? null),
	);

	const landingRow = $derived(selectedTarget?.row_number ?? null);

	async function submitPrep() {
		if (prepBusy) return;
		if (targetPlanID == null) { prepError = 'Pick a target plan.'; return; }
		const notes = prepNotes.trim();
		if (notes === '') { prepError = 'Frame the demand in fiction — preparation notes are required.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'make_demands',
				target_plan_id: targetPlanID,
				preparation_notes: notes,
			});
			targetPlanID = null;
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare demand.';
		} finally { prepBusy = false; }
	}

	$effect(() => {
		if (!readOnly) return;
		targetPlanID = prepDraft?.target_plan_id ?? null;
		prepNotes = prepDraft?.notes ?? '';
	});
	let emitTimer: ReturnType<typeof setTimeout> | null = null;
	$effect(() => {
		if (readOnly || mode !== 'prep') return;
		void targetPlanID; void prepNotes;
		if (emitTimer) clearTimeout(emitTimer);
		emitTimer = setTimeout(() => {
			emitTimer = null;
			ctx.emitPrepDraft({ target_plan_id: targetPlanID, notes: prepNotes });
		}, 150);
	});
	onDestroy(() => { if (emitTimer) clearTimeout(emitTimer); });

	// ── Resolve: derived state ────────────────────────────────────────────────

	const rd = $derived(parseResolutionData(plan).make_demands ?? {});
	const draftChoices = $derived(rd.draft_choices ?? []);
	const counterPlaced = $derived(rd.counter_demand_placed ?? false);
	const draftComplete = $derived(draftChoices.length === 4);

	const targetPlan = $derived(
		plan?.targeted_plan_id == null ? null
			: (plans.find(p => p.id === plan!.targeted_plan_id) ?? null),
	);

	function powerRank(playerID: number | null | undefined): number | null {
		if (playerID == null) return null;
		return rankings.find(r => r.category === 'power' && r.player_id === playerID)?.rank ?? null;
	}

	// Higher power rank (lower rank number) picks first.
	const draftOrder = $derived.by<number[]>(() => {
		if (!plan || !targetPlan) return [];
		const dID = plan.preparer_id;
		const tID = targetPlan.preparer_id;
		const dRank = powerRank(dID);
		const tRank = powerRank(tID);
		if (dRank == null || tRank == null) return [dID, tID];
		return dRank < tRank ? [dID, tID] : [tID, dID];
	});

	const nextPickerID = $derived(
		draftComplete ? null : (draftOrder[draftChoices.length % 2] ?? null),
	);

	const remainingOptions = $derived.by<DemandOption[]>(() => {
		const taken = new Set(draftChoices.map(c => c.option));
		return DEMAND_OPTIONS.filter(o => !taken.has(o));
	});

	const winners = $derived.by<Partial<Record<DemandOption, number>>>(() => {
		const w: Partial<Record<DemandOption, number>> = {};
		for (const c of draftChoices) {
			if ((DEMAND_OPTIONS as string[]).includes(c.option)) {
				w[c.option as DemandOption] = c.player_id;
			}
		}
		return w;
	});

	// ── Resolve: draft picker ─────────────────────────────────────────────────

	let pickedOption = $state<DemandOption | null>(null);
	let draftBusy = $state(false);
	let resError = $state('');

	const amNextPicker = $derived(currentPlayerID != null && currentPlayerID === nextPickerID);

	async function submitDraftPick() {
		if (!plan || pickedOption == null || draftBusy) return;
		draftBusy = true; resError = '';
		try {
			await draftChoice(plan.id, pickedOption);
			pickedOption = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not record draft pick.';
		} finally { draftBusy = false; }
	}

	// ── Resolve: counter-demand picker (mar) ──────────────────────────────────

	// Counter-demand may target only the original demander's plans (per spec)
	// — and only their currently unresolved, non-Make-War, non-self plans (the
	// usual demand-target rules, applied with the *target preparer* as the
	// nominal player). The "Defer" option attaches the counter to the next
	// plan the original demander prepares.
	const counterTargetablePlans = $derived.by(() => {
		if (!plan || !targetPlan) return [];
		const targetedSet = new Set<number>();
		for (const p of plans) {
			if (p.plan_type !== 'make_demands') continue;
			if (p.targeted_plan_id == null) continue;
			if (p.status === 'resolved' || p.status === 'cancelled') continue;
			targetedSet.add(p.targeted_plan_id);
		}
		return plans.filter(p =>
			p.preparer_id === plan!.preparer_id &&  // original demander's plans only
			p.plan_type !== 'make_war' &&
			p.plan_type !== 'make_demands' &&
			p.status !== 'resolved' && p.status !== 'cancelled' &&
			!targetedSet.has(p.id),
		);
	});

	const amCounterTarget = $derived(
		plan != null && targetPlan != null && currentPlayerID === targetPlan.preparer_id,
	);

	type CounterChoice = 'defer' | number;
	let counterChoice = $state<CounterChoice | null>(null);
	let counterBusy = $state(false);

	async function submitCounter() {
		if (!plan || counterChoice == null || counterBusy) return;
		counterBusy = true; resError = '';
		try {
			await counterDemand(plan.id, counterChoice === 'defer' ? null : counterChoice);
			counterChoice = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not place counter-demand.';
		} finally { counterBusy = false; }
	}

	// ── Complete ──────────────────────────────────────────────────────────────

	const canComplete = $derived.by(() => {
		if (!plan || plan.result == null) return false;
		if (plan.result === 'make') return draftComplete;
		if (plan.result === 'mar') return counterPlaced;
		return false;
	});

	let resBusy = $state(false);
	async function onComplete() {
		if (!plan || resBusy) return;
		resBusy = true; resError = '';
		try {
			await completePlan(plan.id);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not complete plan.';
		} finally { resBusy = false; }
	}
</script>

{#if mode === 'prep'}
	<fieldset class="plan-form-fieldset" disabled={readOnly}>
		<div class="plan-form">
			{#if prepError}<p class="res-error">{prepError}</p>{/if}

			{#if targetablePlans.length === 0}
				<p class="muted">No plans on the public record can be demanded against
					right now (own plans, Make War, already-resolved, and already-demanded
					plans are excluded).</p>
			{:else}
				<label class="form-label">
					Target plan:
					<select bind:value={targetPlanID} class="form-textarea" style="height:auto;">
						<option value={null}>— pick a plan to demand against —</option>
						{#each targetablePlans as p}
							<option value={p.id}>
								{PLAN_SHORT[p.plan_type] ?? p.plan_type}
								by {playerName(players, p.preparer_id)}
								(row {p.row_number})
							</option>
						{/each}
					</select>
				</label>

				<label class="form-label">
					Motivation:
					<textarea rows={3} bind:value={prepNotes} class="form-textarea" maxlength={TEXT_LIMITS.NARRATIVE}
						placeholder="Why are you getting involved?" required></textarea>
				</label>

				{#if !readOnly}
					<div class="form-actions">
						<button class="action-btn primary" onclick={submitPrep}
							disabled={prepBusy || targetPlanID == null || !prepNotes.trim()}>
							{prepBusy ? '…' : 'Prepare Plan'}
						</button>
					</div>
				{/if}
			{/if}
		</div>
	</fieldset>

{:else if plan}
	<ResolvingCard {plan} {players} error={resError}>
		{@const targetSummary = targetPlan
			? `${PLAN_SHORT[targetPlan.plan_type] ?? targetPlan.plan_type} by ${playerName(players, targetPlan.preparer_id)} (row ${targetPlan.row_number})`
			: 'unknown target'}
		<p class="choices-note">Targeting: {targetSummary}</p>

		{#if rollActive}
			<p class="ft-prompt muted">Dice roll in progress…</p>

		{:else if rollOutcome === 'make'}
			<!-- ── Draft picker ───────────────────────────────────────────── -->
			<div class="choices-section">
				<p class="choices-header">
					Demand draft ({draftChoices.length} of 4 picked)
				</p>

				{#if draftChoices.length > 0}
					<ul class="plan-notes" style="padding-left:1rem;">
						{#each draftChoices as c}
							<li>
								{playerName(players, c.player_id)}
								picked <em>{DEMAND_OPTION_LABELS[c.option as DemandOption] ?? c.option}</em>
							</li>
						{/each}
					</ul>
				{/if}

				{#if !draftComplete}
					{#if amNextPicker}
						<p class="choices-note">Your turn to pick.</p>
						<div class="choice-list">
							{#each remainingOptions as opt}
								<button type="button" class="choice-card"
									class:active={pickedOption === opt}
									aria-pressed={pickedOption === opt}
									onclick={() => { pickedOption = opt; }}>
									<span class="choice-text">
										<span class="choice-title">{DEMAND_OPTION_LABELS[opt]}{#if pickedOption === opt}<span class="choice-tick"> ✓</span>{/if}</span>
									</span>
								</button>
							{/each}
						</div>
						<button class="action-btn primary" onclick={submitDraftPick}
							disabled={draftBusy || pickedOption == null}>
							{draftBusy ? '…' : 'Pick option'}
						</button>
					{:else}
						<p class="ft-prompt muted">
							Waiting for {playerName(players, nextPickerID)} to pick…
						</p>
					{/if}
				{:else}
					<p class="choices-note">
						Draft complete. The cross-cutting effects (leverage,
						retarget, perform-steps, keep-assets) now apply to the target plan.
					</p>
					<ul class="plan-notes" style="padding-left:1rem;">
						{#each DEMAND_OPTIONS as opt}
							<li>
								<em>{opt.replace(/_/g, ' ')}</em>:
								{playerName(players, winners[opt] ?? null)}
							</li>
						{/each}
					</ul>
				{/if}
			</div>

		{:else if rollOutcome === 'mar'}
			<!-- ── Counter-demand picker ──────────────────────────────────── -->
			<div class="choices-section">
				<p class="choices-header">Counter-demand</p>
				{#if counterPlaced}
					<p class="choices-note">
						Counter-demand recorded. Awaiting completion of this demand plan.
					</p>
				{:else if amCounterTarget}
					<p class="choices-note">
						The demand was marred. You may place a free counter-demand against
						one of {playerName(players, plan.preparer_id)}'s plans, or defer it
						to whichever plan they prepare next.
					</p>
					<div class="choice-list">
						{#each counterTargetablePlans as p}
							<button type="button" class="choice-card"
								class:active={counterChoice === p.id}
								aria-pressed={counterChoice === p.id}
								onclick={() => { counterChoice = p.id; }}>
								<span class="choice-text">
									<span class="choice-title">{PLAN_SHORT[p.plan_type] ?? p.plan_type} (row {p.row_number}){#if counterChoice === p.id}<span class="choice-tick"> ✓</span>{/if}</span>
								</span>
							</button>
						{/each}
						<button type="button" class="choice-card"
							class:active={counterChoice === 'defer'}
							aria-pressed={counterChoice === 'defer'}
							onclick={() => { counterChoice = 'defer'; }}>
							<span class="choice-text">
								<span class="choice-title">Defer — counter the next plan
									{playerName(players, plan.preparer_id)} prepares{#if counterTargetablePlans.length === 0} (no eligible plans yet){/if}{#if counterChoice === 'defer'}<span class="choice-tick"> ✓</span>{/if}</span>
							</span>
						</button>
					</div>
					<button class="action-btn primary" onclick={submitCounter}
						disabled={counterBusy || counterChoice == null}>
						{counterBusy ? '…' : 'Place counter-demand'}
					</button>
				{:else}
					<p class="ft-prompt muted">
						Waiting for {targetPlan ? playerName(players, targetPlan.preparer_id) : 'the target'}
						to place a counter-demand…
					</p>
				{/if}
			</div>
		{/if}

		{#if canComplete && plan != null && currentPlayerID === plan.preparer_id}
			<button class="action-btn primary" onclick={onComplete} disabled={resBusy}>
				{resBusy ? '…' : 'Complete plan'}
			</button>
		{/if}
	</ResolvingCard>
{/if}
