<!-- PlanPanel.svelte
  Dispatcher for plan preparation, resolution, and out-of-band ("alwaysOn")
  views. Plan-type knowledge lives entirely in plans/registry.ts; this file
  only orchestrates the three dispatch sites and constructs the PlanContext
  that every panel consumes.

  Modes:
    'prep'      — focus player's preparation form (action step 2)
    'resolve'   — currently resolving plan
    'alwaysOn'  — out-of-band per-plan view (e.g. Make War status, Liaise
                  delay reveal); driven by REGISTRY[type].alwaysOn predicates.

  The parent is responsible for showing DiceRollPanel when activeRoll != null.
  This component signals "a roll was created" by calling onRollCreated so the
  parent can set activeRoll.
-->
<script lang="ts">
	import {
		getPlanEligibility, resolvePlan,
		type Plan, type PlanType, type Asset, type Player, type Ranking,
		type EligiblePlan, type DiceRoll,
	} from '$lib/api';

	import './plans/planPanel.css';
	import { PLAN_SHORT, playerName } from './plans/shared';
	import { REGISTRY } from './plans/registry';
	import type { PlanContext } from './plans/types';

	interface Props {
		gameID: number;
		currentRow: number;
		/** All plans for the game (used to find a resolving plan). */
		plans: Plan[];
		/** All assets in the game (used for fair-trade peer picker, etc). */
		assets: Asset[];
		players: Player[];
		rankings: Ranking[];
		currentPlayerID: number | null;
		isFocusPlayer: boolean;
		/**
		 * Whether the focus player is allowed to prepare a plan right now.
		 * Should be true only during action step 2.
		 */
		prepEnabled?: boolean;
		/** Whether any dice roll is currently active. */
		rollActive: boolean;
		/** Latest roll outcome — set by parent when roll.resolved arrives. */
		rollOutcome: 'make' | 'mar' | null;
		/** Full active roll object — used by Propose Duel to size the
		 *  winner-takes-N picker (result on make, difficulty on mar). */
		activeRoll?: DiceRoll | null;
		/** Called when a plan-linked dice roll is created. */
		onRollCreated: (roll: DiceRoll) => void;
		/** Called when any plan state changes. */
		onPlansChanged: () => void;
		/** Called when the focus player prepares a plan (their step-2 action). */
		onPlanPrepared: () => void;
	}

	let {
		gameID, currentRow, plans, assets, players, rankings, currentPlayerID,
		isFocusPlayer, prepEnabled = false,
		rollActive, rollOutcome, activeRoll = null,
		onRollCreated, onPlansChanged, onPlanPrepared,
	}: Props = $props();

	// ── Shared context handed to every panel ─────────────────────────────────

	const ctx = $derived<PlanContext>({
		gameID, currentRow, plans, assets, players, rankings,
		currentPlayerID, isFocusPlayer,
		rollActive, rollOutcome, activeRoll,
		onRollCreated, onPlansChanged, onPlanPrepared,
	});

	// ── Derived plan state ────────────────────────────────────────────────────

	const resolvingPlan = $derived(plans.find(p => p.status === 'resolving') ?? null);

	const pendingOnRow = $derived(
		plans.filter(p => p.status === 'pending' && p.row_number === currentRow)
	);

	const needsResolution = $derived(resolvingPlan != null || pendingOnRow.length > 0);

	// Plans that should render out-of-band (independent of resolving status).
	// Driven by per-plan alwaysOn predicates in the registry. Each panel may
	// still self-hide for state it can only learn by fetching (e.g. Make
	// War's war-ended check). The resolving plan is excluded to avoid
	// double-rendering.
	const alwaysOnPlans = $derived(
		plans.filter(p => {
			const entry = REGISTRY[p.plan_type];
			if (!entry?.alwaysOn) return false;
			if (resolvingPlan?.id === p.id) return false;
			return entry.alwaysOn(p, currentPlayerID);
		}),
	);

	// ── Eligibility loading (prep mode) ───────────────────────────────────────

	let eligiblePlans = $state<EligiblePlan[]>([]);
	let eligibilityLoaded = $state(false);
	let eligibilityError = $state('');
	let selectedPlanType = $state<PlanType | null>(null);

	async function loadEligibility() {
		eligibilityError = '';
		try {
			const res = await getPlanEligibility(gameID);
			eligiblePlans = res.eligible;
			eligibilityLoaded = true;
		} catch (e) {
			eligibilityError = e instanceof Error ? e.message : 'Could not load eligibility.';
		}
	}

	$effect(() => {
		if (prepEnabled && isFocusPlayer && !needsResolution && !eligibilityLoaded) {
			loadEligibility();
		}
	});

	// Reset selected plan type when the row changes.
	$effect(() => {
		if (currentRow) {
			selectedPlanType = null;
			eligibilityLoaded = false;
			eligiblePlans = [];
		}
	});

	// ── Pending-plan resolution kickoff ───────────────────────────────────────

	let resError = $state('');
	let resBusy = $state(false);

	async function onResolve(plan: Plan) {
		if (resBusy) return;
		resBusy = true;
		resError = '';
		try {
			const res = await resolvePlan(plan.id);
			if (res.roll) onRollCreated(res.roll);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not begin resolution.';
		} finally {
			resBusy = false;
		}
	}
</script>

<!-- ── Always-on plan views (registry-driven) ──────────────────────────── -->
{#each alwaysOnPlans as p (p.id)}
	{@const Comp = REGISTRY[p.plan_type].component}
	<Comp {ctx} plan={p} mode="alwaysOn" />
{/each}

<!-- ── Resolution dispatch ───────────────────────────────────────────────── -->
{#if resolvingPlan}
	{@const entry = REGISTRY[resolvingPlan.plan_type]}
	{#if entry}
		{@const Comp = entry.component}
		<Comp {ctx} plan={resolvingPlan} mode="resolve" />
	{:else}
		<!-- Fallback for plan types whose resolution UI is not yet implemented. -->
		<div class="plan-panel resolving">
			<div class="plan-header">
				<span class="plan-badge resolving-badge">Resolving</span>
				<strong class="plan-title">{PLAN_SHORT[resolvingPlan.plan_type] ?? resolvingPlan.plan_type}</strong>
				<span class="plan-preparer">by {playerName(players, resolvingPlan.preparer_id)}</span>
			</div>
			{#if resolvingPlan.preparation_notes}
				<p class="plan-notes">"{resolvingPlan.preparation_notes}"</p>
			{/if}
			<p class="ft-prompt muted">
				Resolution UI for this plan is not yet implemented.
			</p>
		</div>
	{/if}

<!-- ── Pending plans on current row ─────────────────────────────────────── -->
{:else if pendingOnRow.length > 0 && isFocusPlayer}
	{@const nextPlan = pendingOnRow[0]}
	<div class="plan-panel pending">
		<div class="plan-header">
			<span class="plan-badge pending-badge">Resolve first</span>
			<strong class="plan-title">{PLAN_SHORT[nextPlan.plan_type] ?? nextPlan.plan_type}</strong>
			<span class="plan-preparer">by {playerName(players, nextPlan.preparer_id)}</span>
		</div>
		{#if nextPlan.preparation_notes}
			<p class="plan-notes">"{nextPlan.preparation_notes}"</p>
		{/if}
		{#if resError}
			<p class="res-error">{resError}</p>
		{/if}
		<p class="resolve-note">This plan must be resolved before the regular scene.</p>
		<button class="action-btn primary" onclick={() => onResolve(nextPlan)} disabled={resBusy}>
			{resBusy ? '…' : 'Begin resolution'}
		</button>
	</div>
{/if}

<!-- ── Preparation dispatch ─────────────────────────────────────────────── -->
{#if prepEnabled && !needsResolution && isFocusPlayer}
	<div class="prep-section">
		{#if !eligibilityLoaded}
			<p class="muted">Checking eligibility…</p>
		{:else if eligibilityError}
			<p class="res-error">{eligibilityError}</p>
		{:else if eligiblePlans.length === 0}
			<p class="muted">No plans available to prepare this turn.</p>
		{:else}
			<div class="plan-picker">
				<span class="picker-label">Prepare a plan:</span>
				{#each eligiblePlans as ep}
					<button
						class="plan-option-btn"
						class:selected={selectedPlanType === ep.plan_type}
						onclick={() => {
							selectedPlanType = selectedPlanType === ep.plan_type ? null : ep.plan_type;
						}}
					>
						{PLAN_SHORT[ep.plan_type] ?? ep.plan_type}
						<span class="plan-row-hint">→ row {ep.target_row}</span>
					</button>
				{/each}
			</div>
		{/if}

		{#if selectedPlanType}
			{@const entry = REGISTRY[selectedPlanType]}
			{#if entry}
				{@const Comp = entry.component}
				<Comp {ctx} mode="prep" />
			{:else}
				<div class="plan-form">
					<p class="form-hint">
						Preparation form for {PLAN_SHORT[selectedPlanType] ?? selectedPlanType}
						is not yet implemented.
					</p>
				</div>
			{/if}
		{/if}
	</div>
{/if}
