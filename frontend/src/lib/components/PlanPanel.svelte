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

  The parent owns the DiceRollPanel. While a plan is resolving, the roll is one
  of the plan's steps, so the parent hands it down as the `roll` snippet and
  this component passes it (with the open step's heading) to the plan's card by
  context (plans/shared/stepFrame), which draws it between the finished steps
  and the plan's own body. `rollClaimed` tells the parent a card did, so the
  parent draws the roll itself only when no plan is on screen. This component
  signals "a roll was created" by calling onRollCreated so the parent can set
  activeRoll.
-->
<script lang="ts">
	import {
		getPlanEligibility,
		type Plan, type PlanToken, type PlanType, type Asset, type Player, type Ranking,
		type EligiblePlan, type IneligiblePlan, type DiceRoll,
		type RankingCategory, type PreparePlanDraft,
	} from '$lib/api';

	import type { Snippet } from 'svelte';
	import './plans/planPanel.css';
	import { PLAN_SHORT, PLAN_DESCRIPTION, PLAN_DELAY, TRACK_ORDER } from './plans/shared';
	import { REGISTRY } from './plans/registry';
	import { activeResolvingPlan } from '$lib/plans/resolutionData/host_festivity';
	import LazyPlanPanel from './plans/LazyPlanPanel.svelte';
	import RowPill from './plans/RowPill.svelte';
	import ResolvingCard from './plans/ResolvingCard.svelte';
	import { provideStepFrame } from './plans/shared/stepFrame';
	import type { StepHeading as StepHeadingData } from '$lib/stepHeading';
	import StepHeading from '$lib/components/shared/StepHeading.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';
	import ToneMark from '$lib/components/shared/ToneMark.svelte';
	import Notice from '$lib/components/shared/Notice.svelte';
	import DieMark from '$lib/components/shared/DieMark.svelte';
	import LogMark from './LogMark.svelte';
	import { highlightedRow } from '$lib/highlight';
	import type { PlanContext } from './plans/types';
	import { provideActiveRoll } from './plans/demand/activeRollContext';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	const TRACK_LABEL: Record<RankingCategory, string> = {
		power:     'Power',
		esteem:    'Esteem',
		knowledge: 'Knowledge',
	};
	const TRACKS: RankingCategory[] = ['power', 'knowledge', 'esteem'];

	interface Props {
		gameID: number;
		currentRow: number;
		/** game.ending_mode; passed straight through to the shared PlanContext. */
		endingMode: string | null;
		/** All plans for the game (used to find a resolving plan). */
		plans: Plan[];
		/**
		 * Plan tokens (one per plan_type/player). Rendered as coloured pips on
		 * each grid cell so every viewer — including read-only ones — can see
		 * which players hold a token on each plan's shield.
		 */
		planTokens: PlanToken[];
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
		/** The open step's heading ("Your move — …"): the current ask. */
		heading?: StepHeadingData | null;
		/** The dice roll, as a step. Drawn by the plan's card, in order. */
		roll?: Snippet | null;
		/** The roll is still open (so it carries the heading itself). */
		rollOpen?: boolean;
		/** Set while a plan's card is drawing the roll, so the parent doesn't. */
		rollClaimed?: boolean;
	}

	let {
		gameID, currentRow, endingMode, plans, planTokens, assets, players, rankings, currentPlayerID,
		isFocusPlayer, prepEnabled = false, suppressPrep = false,
		rollActive, rollOutcome, activeRoll = null,
		onRollCreated, onPlansChanged, onPlanPrepared,
		preparePlanDraft = null,
		heading = null, roll = null, rollOpen = false,
		rollClaimed = $bindable(false),
	}: Props = $props();

	// ── Shared context handed to every panel ─────────────────────────────────

	// ctx is built further down, after `selectedPlanType` and
	// `displaySelectedPlanType` are declared — it depends on both for
	// `prepDraft` scoping and `emitPrepDraft`'s plan_type stamping.

	// ── Derived plan state ────────────────────────────────────────────────────

	// Normally the one resolving plan; a festivity paused for its duel yields to it.
	const resolvingPlan = $derived(activeResolvingPlan(plans, currentPlayerID));

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
		| { type: PlanType; eligible: true; targetRow: number; finaleBonus: boolean }
		| { type: PlanType; eligible: false; reason: string };
	const planCells = $derived.by<Map<PlanType, PlanCell>>(() => {
		const m = new Map<PlanType, PlanCell>();
		for (const p of eligiblePlans) m.set(p.plan_type, {
			type: p.plan_type, eligible: true, targetRow: p.target_row,
			finaleBonus: p.finale_bonus,
		});
		for (const p of ineligiblePlans) m.set(p.plan_type, {
			type: p.plan_type, eligible: false, reason: p.reason,
		});
		return m;
	});

	// Token holders per plan type, resolved to Player objects and sorted by
	// seat for a stable pip order. Built from props every viewer has, so the
	// pips render identically in the read-only (non-focus) grid. The schema's
	// UNIQUE (game, plan_type, player) guarantees at most one pip per player,
	// capping each cell at the table size (≤5).
	const tokenHoldersByType = $derived.by<Map<PlanType, Player[]>>(() => {
		const byId = new Map(players.map(p => [p.id, p]));
		const m = new Map<PlanType, Player[]>();
		for (const t of planTokens) {
			const player = byId.get(t.player_id);
			if (!player) continue;
			const arr = m.get(t.plan_type);
			if (arr) arr.push(player);
			else m.set(t.plan_type, [player]);
		}
		for (const arr of m.values()) {
			arr.sort((a, b) => (a.seat_order ?? 0) - (b.seat_order ?? 0));
		}
		return m;
	});

	// Ineligibility reason surfaced by tapping a greyed-out card. The card's
	// title attribute covers desktop hover, but title tooltips never fire on
	// touch devices, so the reason also renders in a status line below the
	// grid (mobile-first).
	let ineligibleNotice = $state<{ type: PlanType; reason: string } | null>(null);

	function onPlanClick(cell: PlanCell) {
		if (!cell.eligible) {
			ineligibleNotice = { type: cell.type, reason: cell.reason };
			return;
		}
		ineligibleNotice = null;
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
			ineligibleNotice = null;
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

	// ── Explosive Finale bonus warning ───────────────────────────────────────
	// Selecting a tile commits nothing — the prep form below has its own submit
	// — so selection is itself the pre-decision moment, which is why a warning
	// here is enough and no confirmation dialog is wanted
	// (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §7). Non-focus viewers never have
	// eligibility loaded, so this stays false for them: the warning belongs to
	// whoever is making the choice.
	const selectedBonusCell = $derived.by(() => {
		if (!displaySelectedPlanType) return null;
		const cell = planCells.get(displaySelectedPlanType);
		return cell?.eligible && cell.finaleBonus ? cell : null;
	});
	// The row the plan would have landed on if it fitted — the thing the clamp
	// is replacing. Always a real number here: the two variable-delay plans are
	// never marked as bonus tiles.
	const bonusNaturalRow = $derived(
		displaySelectedPlanType ? currentRow + PLAN_DELAY[displaySelectedPlanType] : 0,
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

	provideActiveRoll(() => activeRoll);

	provideStepFrame(() => ({
		roll,
		rollOpen,
		heading,
		claimRoll: () => {
			rollClaimed = true;
			return () => { rollClaimed = false; };
		},
	}));

	const ctx = $derived<PlanContext>({
		gameID, currentRow, endingMode, plans, assets, players, rankings,
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
		<LazyPlanPanel planType={resolvingPlan.plan_type} {ctx} plan={resolvingPlan} mode="resolve" />
	{:else}
		<!-- Fallback for plan types whose resolution UI is not yet implemented. -->
		<ResolvingCard plan={resolvingPlan} {players}>
			<p class="ft-prompt muted">
				Resolution UI for this plan is not yet implemented.
			</p>
		</ResolvingCard>
	{/if}

{/if}

<!-- ── Preparation dispatch ─────────────────────────────────────────────── -->
{#if prepEnabled && !needsResolution && !suppressPrep}
	<div class="prep-section">
		<!-- Preparing is the open step: its heading is the current ask, for the
		     focus player ("Your move — prepare a plan, or refresh 2 assets") and for
		     everyone watching them ("Waiting on otto — …"). -->
		<StepHeading {heading} />
		{#if isFocusPlayer && !eligibilityLoaded}
			<p class="muted">Checking eligibility…</p>
		{:else if isFocusPlayer && eligibilityError}
			<ErrorText message={eligibilityError} variant="panel" />
		{:else if isFocusPlayer && eligiblePlans.length === 0 && ineligiblePlans.length === 0}
			<p class="muted">No plans available to prepare this turn.</p>
		{:else}
			<!-- Flat 3-column grid: 3 track headings on the first row, then
			     4 rows of 3 cards. Auto-flow (row-major) handles placement;
			     equal min-height on every card guarantees uniform sizing
			     across all 12 plans. For non-focus players the grid renders
			     as a disabled skeleton — eligibility is per-player on the
			     server, so we don't fetch it for them — mirroring the focus
			     player's highlighted card from the broadcast draft. -->
			<div class="plan-grid">
				{#each TRACKS as track}
					<h3 class="track-heading">{TRACK_LABEL[track]}</h3>
				{/each}
				{#each [0, 1, 2, 3] as rowIdx}
					{#each TRACKS as track}
						{@const pt = TRACK_ORDER[track][rowIdx]}
						{@const cell = isFocusPlayer ? planCells.get(pt) : undefined}
						{@const holders = tokenHoldersByType.get(pt) ?? []}
						{@const picked = displaySelectedPlanType === pt}
						{@const bonus = !!cell && cell.eligible && cell.finaleBonus}
						<!-- Ineligible cards stay enabled (aria-disabled + .ineligible
						     styling) so a tap can surface the reason below the grid;
						     native disabled would swallow the click. The non-focus
						     skeleton stays truly disabled. A card is a game piece, so
						     a pick is the bright border and a tick, never a fill, and
						     a card that can't be picked goes faint, never translucent. -->
						<button
							type="button"
							class="plan-card"
							class:selected={picked}
							class:mirrored={picked && !isFocusPlayer}
							class:finale-bonus={bonus}
							class:ineligible={!!cell && !cell.eligible}
							disabled={!isFocusPlayer || !cell}
							aria-pressed={picked}
							aria-disabled={cell && !cell.eligible ? true : undefined}
							title={cell && !cell.eligible ? cell.reason : undefined}
							onclick={() => cell && onPlanClick(cell)}
							onmouseenter={() => cell && onPlanHover(cell)}
							onmouseleave={onPlanLeave}
							onfocus={() => cell && onPlanHover(cell)}
							onblur={onPlanLeave}
						>
							<span class="card-head">
								<span class="card-title">{PLAN_SHORT[pt]}</span>
								{#if picked}
									<span class="card-tick" aria-hidden="true"><ToneMark tone="success" /></span>
								{/if}
							</span>
							<span class="card-desc">{PLAN_DESCRIPTION[pt]}</span>
							<span class="card-foot">
								{#if pt === 'make_demands'}
									<span class="card-delay">
										<span class="card-delay-mark"><LogMark family="demand" /></span>
										Targets a plan
									</span>
								{:else if pt === 'make_war' || pt === 'clandestinely_liaise'}
									<span class="card-delay">
										<DieMark size={16} />
										Delay by dice
									</span>
								{:else}
									<RowPill row={PLAN_DELAY[pt]} kind="delay" />
								{/if}
								{#if bonus}
									<!-- Spending the one Explosive Finale plan: a mark and words,
									     so the colour is never the only signal. -->
									<span class="card-bonus">
										<ToneMark tone="warning" size={16} />
										Finale
									</span>
								{/if}
							</span>
							{#if holders.length > 0}
								<!-- Who holds a token on this plan's shield: each holder's shape
								     in their colour. A sibling of the text, not inside a faded
								     body, so it stays legible on every card. -->
								<span class="prep-pips" role="img" aria-label="Prepared by {holders.map(h => h.display_name).join(', ')}">
									{#each holders as h (h.id)}
										<PlayerShape player={h} size={12} />
									{/each}
								</span>
							{/if}
						</button>
					{/each}
				{/each}
			</div>
			{#if ineligibleNotice}
				<p class="ineligible-reason" role="status">
					<em>{PLAN_SHORT[ineligibleNotice.type]}</em> — {ineligibleNotice.reason}.
				</p>
			{/if}
		{/if}

		{#if displaySelectedPlanType}
			{#if selectedBonusCell}
				<!-- One place covers all 12 plans: it names the clamp and the cost,
				     before anything is committed. -->
				<Notice tone="warning" live>
					<span class="finale-warning-head">This will be your one Explosive Finale plan.</span>
					<em>{PLAN_SHORT[displaySelectedPlanType]}</em> would normally land on row {bonusNaturalRow},
					past the end of the record, but it will be placed on row 13 instead.
					Each player only gets this benefit once.
				</Notice>
			{/if}
			{@const entry = REGISTRY[displaySelectedPlanType]}
			{#if entry}
				<LazyPlanPanel planType={displaySelectedPlanType} {ctx} mode="prep" />
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
