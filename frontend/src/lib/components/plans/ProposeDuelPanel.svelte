<!-- ProposeDuelPanel.svelte
  Prep + resolve UI for Propose Duel (Tier 3, Esteem, delay 5).

  Resolution is driven by resolution_data.duel_phase:
    setup    — who duels (a peer; main character by default) + a single
               combined stake commit (which assets, count derived). Both stay
               hidden from the opponent until both commit, then → bouts.
    bouts    — declarer/responder bout loop with hidden dice.
    roll     — standard dice roll with accumulated bout dice pre-loaded.
    done     — winner has claimed stakes; preparer completes.

  This panel is a dispatcher. The per-phase UI lives in `duel/`: PrepForm,
  SetupPhase, BoutsPhase, RollPhase. The parent owns the duel-state fetch, WS
  subscription, and shared identity/derivation work; children get slices +
  onPlansChanged + (where needed) an onRefresh callback that re-runs
  `getDuelState`.
-->
<script lang="ts">
	import './planPanel.css';
	import { onMount, onDestroy } from 'svelte';
	import {
		completePlan,
		getDuelState,
		type DuelStake, type DuelBout, type DuelStateResponse,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import { playerName, parseResolutionData } from './shared';

	import PrepForm from './duel/PrepForm.svelte';
	import SetupPhase from './duel/SetupPhase.svelte';
	import BoutsPhase from './duel/BoutsPhase.svelte';
	import RollPhase from './duel/RollPhase.svelte';
	import type { DuelRes } from './duel/shared';

	import type { PlanPanelProps } from './types';

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
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	// ── Resolve: parse resolution_data ───────────────────────────────────────
	const duelRes = $derived.by<DuelRes>(() => {
		const d = parseResolutionData(plan).duel ?? {};
		return {
			duelType: d.duel_type ?? '',
			phase: d.phase ?? '',
			initiativeID: d.initiative_player_id ?? null,
			prepChampID: d.preparer_champion_id ?? null,
			targChampID: d.target_champion_id ?? null,
			prepChampDeclared: d.preparer_champion_declared ?? false,
			targChampDeclared: d.target_champion_declared ?? false,
			currentBout: d.current_bout ?? 0,
		};
	});

	// ── Participant identity helpers ─────────────────────────────────────────
	const amPreparer = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const amTarget   = $derived(
		plan != null && plan.target_player_id != null && currentPlayerID === plan.target_player_id,
	);
	const amParticipant = $derived(amPreparer || amTarget);

	function esteemRank(playerID: number | null): number | null {
		if (playerID == null) return null;
		const r = rankings.find(x => x.category === 'esteem' && x.player_id === playerID);
		return r?.rank ?? null;
	}
	function statusOf(playerID: number | null): number {
		const r = esteemRank(playerID);
		if (r == null) return 0;
		return Math.max(6 - r, 0);
	}
	const myMaxStakes = $derived(1 + statusOf(currentPlayerID));

	// Difficulty = the challenged player's esteem status (mirrors
	// game.ProposeDuelDifficulty). It's how many of the loser's stakes change
	// hands on a mar, so it's worth surfacing up front.
	const difficulty = $derived.by(() => {
		const r = esteemRank(plan?.target_player_id ?? null);
		return r == null ? null : Math.max(6 - r, 1);
	});

	// ── Duel-state fetch + live refresh ──────────────────────────────────────
	let duelState = $state<DuelStateResponse | null>(null);
	let duelStateError = $state('');
	let lastFetchedPlanID = $state<number | null>(null);

	async function refreshDuelState() {
		if (!plan) return;
		try {
			duelState = await getDuelState(plan.id);
			duelStateError = '';
		} catch (e) {
			duelStateError = e instanceof Error ? e.message : 'Could not load duel state.';
		}
	}

	function onDuelEvent(e: Event) {
		const detail = (e as CustomEvent<{ plan_id: number }>).detail;
		if (plan && detail?.plan_id === plan.id) refreshDuelState();
	}

	onMount(() => {
		if (mode === 'resolve' && plan) {
			lastFetchedPlanID = plan.id;
			refreshDuelState();
		}
		window.addEventListener('uneasy:duel.champion_elected', onDuelEvent);
		window.addEventListener('uneasy:duel.stakes_selected', onDuelEvent);
		window.addEventListener('uneasy:duel.bout_declared', onDuelEvent);
		window.addEventListener('uneasy:duel.bout_resolved', onDuelEvent);
		window.addEventListener('uneasy:duel.bouts_complete', onDuelEvent);
	});
	onDestroy(() => {
		window.removeEventListener('uneasy:duel.champion_elected', onDuelEvent);
		window.removeEventListener('uneasy:duel.stakes_selected', onDuelEvent);
		window.removeEventListener('uneasy:duel.bout_declared', onDuelEvent);
		window.removeEventListener('uneasy:duel.bout_resolved', onDuelEvent);
		window.removeEventListener('uneasy:duel.bouts_complete', onDuelEvent);
	});

	$effect(() => {
		if (mode === 'resolve' && plan && plan.id !== lastFetchedPlanID) {
			lastFetchedPlanID = plan.id;
			refreshDuelState();
		}
	});
	$effect(() => {
		void duelRes.phase;
		if (mode === 'resolve' && plan) refreshDuelState();
	});

	const stakes = $derived<DuelStake[]>(duelState?.stakes ?? []);
	const bouts  = $derived<DuelBout[]>(duelState?.bouts ?? []);

	const preparerStakes = $derived(
		plan == null ? [] : stakes.filter(s => s.player_id === plan.preparer_id),
	);
	const targetStakes = $derived(
		plan == null ? [] : stakes.filter(s => s.player_id === plan.target_player_id),
	);
	const myStakes = $derived(stakes.filter(s => s.player_id === currentPlayerID));
	const myUnresolvedStakes = $derived(myStakes.filter(s => !s.is_resolved));

	// Complete (focus player, phase=done).
	let completeBusy = $state(false);
	let completeError = $state('');
	async function onComplete() {
		if (!plan || completeBusy) return;
		completeBusy = true; completeError = '';
		try {
			await completePlan(plan.id);
			onPlansChanged();
		} catch (e) {
			completeError = e instanceof Error ? e.message : 'Could not complete plan.';
		} finally { completeBusy = false; }
	}
</script>

{#if mode === 'prep'}
	<PrepForm {ctx} />

{:else if plan}
	<ResolvingCard {plan} {players} error={duelStateError}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID} />

		<p class="choices-note" style="margin:0;">
			{playerName(players, plan.preparer_id)}
			vs {playerName(players, plan.target_player_id)}
			{#if difficulty != null}· difficulty <strong>{difficulty}</strong>{/if}
		</p>

		{#if duelRes.phase === 'setup' || duelRes.phase === ''}
			<SetupPhase
				{plan} {duelRes} {players} {assets} {currentPlayerID}
				{amParticipant} {amPreparer} {amTarget} {myMaxStakes}
				{myStakes} {onPlansChanged} onRefresh={refreshDuelState}
			/>

		{:else if duelRes.phase === 'bouts'}
			<BoutsPhase
				{plan} {duelRes} {players} {assets} {currentPlayerID}
				{amParticipant}
				{preparerStakes} {targetStakes} {bouts} {myUnresolvedStakes}
				{onPlansChanged} onRefresh={refreshDuelState}
			/>

		{:else if duelRes.phase === 'roll'}
			<RollPhase
				{plan} {duelRes} {players} {assets} {currentPlayerID}
				{stakes} {bouts} {activeRoll} {rollActive} {rollOutcome}
				{onPlansChanged}
			/>

		{:else if duelRes.phase === 'done'}
			<div class="complete-section">
				<p class="choices-applied">
					Duel complete. All staked assets are leveraged.
				</p>
				{#if completeError}<p class="res-error">{completeError}</p>{/if}
				{#if amPreparer}
					<button class="action-btn primary"
						onclick={onComplete} disabled={completeBusy}>
						{completeBusy ? '…' : 'Complete plan'}
					</button>
				{/if}
			</div>

		{:else}
			<p class="ft-prompt muted">Phase: {duelRes.phase || '(unknown)'}</p>
		{/if}

	</ResolvingCard>
{/if}
