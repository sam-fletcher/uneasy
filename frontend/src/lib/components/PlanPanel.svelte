<!-- PlanPanel.svelte
  Dispatcher for plan preparation and resolution. Plan-type knowledge lives
  entirely in plans/registry.ts; this file only orchestrates the dispatch
  sites and constructs the PlanContext that every panel consumes.

  Modes:
    'prep'    — focus player's preparation form (action step 2)
    'resolve' — currently resolving plan

  Delay-reveal panels (Make War, Clandestinely Liaise) are rendered
  directly from MainEventView for every player via the row_state kind
  'await_delay_reveal' — they don't dispatch through here.

  The parent is responsible for showing DiceRollPanel when activeRoll != null.
  This component signals "a roll was created" by calling onRollCreated so the
  parent can set activeRoll.
-->
<script lang="ts">
	import {
		getPlanEligibility,
		type Plan, type PlanType, type Asset, type Player, type Ranking,
		type EligiblePlan, type IneligiblePlan, type DiceRoll,
		type RankingCategory, type PreparePlanDraft,
	} from '$lib/api';

	import './plans/planPanel.css';
	import { PLAN_SHORT, PLAN_DESCRIPTION, PLAN_DELAY, TRACK_ORDER, playerName } from './plans/shared';
	import { REGISTRY } from './plans/registry';
	import RowPill from './plans/RowPill.svelte';
	import { highlightedRow } from '$lib/highlight';
	import type { PlanContext } from './plans/types';

	const TRACK_LABEL: Record<RankingCategory, string> = {
		power:     'Power',
		esteem:    'Esteem',
		knowledge: 'Knowledge',
	};
	const TRACKS: RankingCategory[] = ['power', 'knowledge', 'esteem'];

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
		 * Whether prep UI should be shown at all (post-scene action step).
		 * Set by the parent based on scene-end state — independent of who's
		 * looking. Authority is conveyed by `isFocusPlayer`.
		 */
		prepEnabled?: boolean;
		/**
		 * Hide the post-scene prep grid and "next pending plan" call-to-action,
		 * leaving only the resolving panel visible. Used when a Make War delay
		 * reveal needs the play area's full attention.
		 */
		suppressPrep?: boolean;
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
		/**
		 * Ephemeral mirror of the focus player's currently-highlighted plan
		 * card, broadcast over the WS. Drives the read-only highlight for
		 * non-focus viewers. Null until the focus player's first selection.
		 */
		preparePlanDraft?: PreparePlanDraft | null;
	}

	let {
		gameID, currentRow, plans, assets, players, rankings, currentPlayerID,
		isFocusPlayer, prepEnabled = false, suppressPrep = false,
		rollActive, rollOutcome, activeRoll = null,
		onRollCreated, onPlansChanged, onPlanPrepared,
		preparePlanDraft = null,
	}: Props = $props();

	// ── Shared context handed to every panel ─────────────────────────────────

	// ctx is built further down, after `selectedPlanType` and
	// `displaySelectedPlanType` are declared — it depends on both for
	// `prepDraft` scoping and `emitPrepDraft`'s plan_type stamping.

	// ── Derived plan state ────────────────────────────────────────────────────

	const resolvingPlan = $derived(plans.find(p => p.status === 'resolving') ?? null);

	// Pending plans on the current row are auto-kicked off server-side (see
	// broadcastRowState in handler/row_state.go), so the client should only
	// ever observe `resolving` here in normal play. The 'pending' status
	// surfaces only briefly during the server-side transition, or in the
	// rare case OnResolve errors and leaves the plan pending — neither
	// warrants a play-area panel.
	const needsResolution = $derived(resolvingPlan != null);

	// ── Eligibility loading (prep mode) ───────────────────────────────────────

	let eligiblePlans = $state<EligiblePlan[]>([]);
	let ineligiblePlans = $state<IneligiblePlan[]>([]);
	let eligibilityLoaded = $state(false);
	let eligibilityError = $state('');
	let selectedPlanType = $state<PlanType | null>(null);

	async function loadEligibility() {
		eligibilityError = '';
		try {
			const res = await getPlanEligibility(gameID);
			eligiblePlans = res.eligible ?? [];
			ineligiblePlans = res.ineligible ?? [];
			eligibilityLoaded = true;
		} catch (e) {
			eligibilityError = e instanceof Error ? e.message : 'Could not load eligibility.';
		}
	}

	// Build a per-plan-type lookup combining eligible + ineligible entries
	// so the 3-column grid can render all 12 plans (ineligible ones disabled,
	// with a hover tooltip explaining why).
	type PlanCell =
		| { type: PlanType; eligible: true; targetRow: number }
		| { type: PlanType; eligible: false; reason: string };
	const planCells = $derived.by<Map<PlanType, PlanCell>>(() => {
		const m = new Map<PlanType, PlanCell>();
		for (const p of eligiblePlans) m.set(p.plan_type, {
			type: p.plan_type, eligible: true, targetRow: p.target_row,
		});
		for (const p of ineligiblePlans) m.set(p.plan_type, {
			type: p.plan_type, eligible: false, reason: p.reason,
		});
		return m;
	});

	function onPlanClick(cell: PlanCell) {
		if (!cell.eligible) return;
		const next = selectedPlanType === cell.type ? null : cell.type;
		selectedPlanType = next;
		if (next) highlightedRow.set(cell.targetRow);
		else highlightedRow.set(null);
	}

	function onPlanHover(cell: PlanCell) {
		if (cell.eligible) highlightedRow.set(cell.targetRow);
	}
	function onPlanLeave() {
		// Restore selection's row, if any, else clear.
		if (selectedPlanType) {
			const c = planCells.get(selectedPlanType);
			if (c?.eligible) { highlightedRow.set(c.targetRow); return; }
		}
		highlightedRow.set(null);
	}

	$effect(() => {
		// Only the focus player's eligibility is meaningful here; the
		// endpoint resolves the calling player, so non-focus players would
		// fetch their own (incorrect) eligibility. Skip the fetch for them
		// — the grid still renders as a disabled skeleton.
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
			ineligiblePlans = [];
			highlightedRow.set(null);
		}
	});

	// What the grid should visually highlight as selected: the focus player
	// reads from local state; everyone else reads from the broadcast draft.
	const displaySelectedPlanType = $derived<PlanType | null>(
		isFocusPlayer
			? selectedPlanType
			: ((preparePlanDraft?.plan_type ?? '') as PlanType) || null
	);

	// ── Shared ctx (now that selectedPlanType + displaySelectedPlanType exist) ─
	// Slice of the draft addressed to the currently-mounted plan panel.
	// Guard on plan_type match so a stale draft from a previously-selected
	// card never leaks into the new one (focus player can switch cards
	// faster than the WS round-trip).
	const prepDraftForSelection = $derived<Record<string, unknown> | null>(
		preparePlanDraft &&
		displaySelectedPlanType &&
		preparePlanDraft.plan_type === displaySelectedPlanType
			? (preparePlanDraft.prep ?? null)
			: null,
	);

	function emitPrepDraft(prep: Record<string, unknown>) {
		// Only the focus player should ever call this; double-guard so a
		// rogue panel can't broadcast on behalf of someone else.
		if (!isFocusPlayer || !selectedPlanType) return;
		window.dispatchEvent(new CustomEvent('uneasy:prepare_plan_draft', {
			detail: { plan_type: selectedPlanType, prep },
		}));
	}

	const ctx = $derived<PlanContext>({
		gameID, currentRow, plans, assets, players, rankings,
		currentPlayerID, isFocusPlayer,
		rollActive, rollOutcome, activeRoll,
		onRollCreated, onPlansChanged, onPlanPrepared,
		readOnly: !isFocusPlayer,
		prepDraft: prepDraftForSelection,
		emitPrepDraft,
	});

	// ── Card-selection draft emission (focus player only) ────────────────────
	// Broadcast which card is currently selected so non-focus viewers can
	// mirror the highlight. Only emit while the focus player is actually in
	// the prep step — otherwise we'd spam stale "null" pings. Per-field prep
	// snapshots are sent separately by the individual panels via emitPrepDraft.
	let lastEmittedPlanType = $state<PlanType | null>(null);
	$effect(() => {
		if (!isFocusPlayer) return;
		if (!prepEnabled || needsResolution || suppressPrep) return;
		if (selectedPlanType === lastEmittedPlanType) return;
		lastEmittedPlanType = selectedPlanType;
		window.dispatchEvent(new CustomEvent('uneasy:prepare_plan_draft', {
			detail: { plan_type: selectedPlanType ?? '', prep: null },
		}));
	});

</script>

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

{/if}

<!-- ── Preparation dispatch ─────────────────────────────────────────────── -->
{#if prepEnabled && !needsResolution && !suppressPrep}
	<div class="prep-section">
		{#if isFocusPlayer && !eligibilityLoaded}
			<p class="muted">Checking eligibility…</p>
		{:else if isFocusPlayer && eligibilityError}
			<p class="res-error">{eligibilityError}</p>
		{:else if isFocusPlayer && eligiblePlans.length === 0 && ineligiblePlans.length === 0}
			<p class="muted">No plans available to prepare this turn.</p>
		{:else}
			<!-- Flat 3-column grid: 3 track headings on the first row, then
			     4 rows of 3 cards. Auto-flow (row-major) handles placement;
			     equal min-height on every card guarantees uniform sizing
			     across all 12 plans. For non-focus players the grid renders
			     as a disabled skeleton — eligibility is per-player on the
			     server, so we don't fetch it for them; the focus player's
			     selection isn't synced yet either. -->
			<div class="plan-grid">
				{#each TRACKS as track}
					<h4 class="track-heading">{TRACK_LABEL[track]}</h4>
				{/each}
				{#each [0, 1, 2, 3] as rowIdx}
					{#each TRACKS as track}
						{@const pt = TRACK_ORDER[track][rowIdx]}
						{@const cell = isFocusPlayer ? planCells.get(pt) : undefined}
						<button
							type="button"
							class="plan-card"
							class:selected={displaySelectedPlanType === pt}
							disabled={!isFocusPlayer || !cell || !cell.eligible}
							title={cell && !cell.eligible ? cell.reason : undefined}
							onclick={() => cell && onPlanClick(cell)}
							onmouseenter={() => cell && onPlanHover(cell)}
							onmouseleave={onPlanLeave}
							onfocus={() => cell && onPlanHover(cell)}
							onblur={onPlanLeave}
						>
							<div class="card-head">
								<span class="card-title">{PLAN_SHORT[pt]}</span>
								{#if cell?.eligible}
									{#if pt === 'make_demands'}
										<span class="plan-icon" aria-label="Targets another plan" title="Targets another plan">
											<svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true">
												<circle cx="10" cy="10" r="8.5" fill="none" stroke="currentColor" stroke-width="1.2" />
												<circle cx="10" cy="10" r="5"   fill="none" stroke="currentColor" stroke-width="1.2" />
												<circle cx="10" cy="10" r="1.8" fill="currentColor" />
											</svg>
										</span>
									{:else if pt === 'make_war' || pt === 'clandestinely_liaise'}
										<span class="plan-icon" aria-label="Delay determined by dice roll" title="Delay determined by dice roll">
											<svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true">
												<rect x="2.5" y="2.5" width="15" height="15" rx="2.5" ry="2.5"
													fill="none" stroke="currentColor" stroke-width="1.2" />
												<circle cx="6.5"  cy="6.5"  r="1.2" fill="currentColor" />
												<circle cx="13.5" cy="6.5"  r="1.2" fill="currentColor" />
												<circle cx="10"   cy="10"   r="1.2" fill="currentColor" />
												<circle cx="6.5"  cy="13.5" r="1.2" fill="currentColor" />
												<circle cx="13.5" cy="13.5" r="1.2" fill="currentColor" />
											</svg>
										</span>
									{:else}
										<RowPill row={PLAN_DELAY[pt]} kind="delay" />
									{/if}
								{/if}
							</div>
							<p class="card-desc">{PLAN_DESCRIPTION[pt]}</p>
						</button>
					{/each}
				{/each}
			</div>
		{/if}

		{#if displaySelectedPlanType}
			{@const entry = REGISTRY[displaySelectedPlanType]}
			{#if entry}
				{@const Comp = entry.component}
				<Comp {ctx} mode="prep" />
			{:else}
				<div class="plan-form">
					<p class="form-hint">
						Preparation form for {PLAN_SHORT[displaySelectedPlanType] ?? displaySelectedPlanType}
						is not yet implemented.
					</p>
				</div>
			{/if}
		{/if}
	</div>
{/if}
