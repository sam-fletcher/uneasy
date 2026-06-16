<!-- HostFestivityPanel.svelte
  Prep + resolve UI for Host Festivity (Tier 3, Esteem, delay 6).

  Resolution phases (driven by resolution_data.festivity_phase):
    socializing   — guests join, then take turns (lower esteem first, host last).
    host_choosing — host picks a make option for each guest who marred / opted out.
    done          — host clicks Complete.

  Each guest creates their own dice roll via /guest-roll. The active roll is
  surfaced through the parent's <DiceRollPanel> using the standard
  activeRoll/rollOutcome props. Once the roll resolves, the guest submits a
  make/mar choice through /guest-choice (or /challenge-duel for duels).

  This panel is a dispatcher. Phase- and role-specific UI lives in
  `festivity/`: PrepForm, GuestList, ChallengeBanner, SocializingTurn,
  InsistFlow, HostChoosing. The parent owns the `fest` derivation, the
  active-roll-fallback fetch, and the WS subscription; children get
  slices + `onPlansChanged`.
-->
<script lang="ts">
	import './planPanel.css';
	import { onMount, onDestroy } from 'svelte';
	import {
		completePlan,
		getRoll,
		type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import { playerName, assetName, parseResolutionData } from './shared';

	import PrepForm from './festivity/PrepForm.svelte';
	import GuestList from './festivity/GuestList.svelte';
	import ChallengeBanner from './festivity/ChallengeBanner.svelte';
	import SocializingTurn from './festivity/SocializingTurn.svelte';
	import InsistFlow from './festivity/InsistFlow.svelte';
	import HostChoosing from './festivity/HostChoosing.svelte';
	import type { FestRes } from './festivity/options';

	import type { PlanPanelProps } from './types';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const rankings = $derived(ctx.rankings);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const rollOutcome = $derived(ctx.rollOutcome);
	const activeRoll = $derived(ctx.activeRoll);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	// ── Resolve: parse resolution_data ───────────────────────────────────────
	// Every player at the table is a guest (the roster is fixed once a game
	// starts), so the guest list is derived from `players`, not stored in
	// resolution_data.
	const fest = $derived.by<FestRes>(() => {
		const f = parseResolutionData(plan).festivity ?? {};
		return {
			phase: f.phase ?? '',
			guests: players.map(p => p.id),
			outcomes: f.outcomes ?? {},
			guestMakes: f.guest_makes ?? {},
			guestMars: f.guest_mars ?? {},
			hostChoices: f.host_choices ?? {},
			guestRollIDs: f.guest_roll_ids ?? {},
			guestIOUs: f.guest_ious ?? [],
			hostMarInsists: f.host_mar_insists ?? [],
			acceptDuels: f.accept_duels ?? [],
			pendingDuelPlanID: f.pending_duel_plan_id ?? null,
			pendingChallenge: f.pending_challenge ?? null,
			centeredAssetIDs: f.centered_asset_ids ?? [],
		};
	});

	const meKey = $derived(currentPlayerID == null ? '' : String(currentPlayerID));
	const amHost = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const iAmGuest = $derived(currentPlayerID != null && fest.guests.includes(currentPlayerID));
	const myOutcome = $derived(meKey ? fest.outcomes[meKey] ?? null : null);
	const myRollID = $derived(meKey ? fest.guestRollIDs[meKey] ?? null : null);
	const iHaveIOU = $derived(currentPlayerID != null && fest.guestIOUs.includes(currentPlayerID));

	// Esteem-ordered turn pointer — also computed inside GuestList for its
	// own display, but the dispatcher needs it so SocializingTurn can show
	// "X should go before you".
	function esteemRank(playerID: number | null): number {
		if (playerID == null) return 999;
		const r = rankings.find(x => x.category === 'esteem' && x.player_id === playerID);
		return r?.rank ?? 999;
	}
	const currentTurnID = $derived.by<number | null>(() => {
		if (plan == null) return null;
		const hostID = plan.preparer_id;
		const others = fest.guests.filter(id => id !== hostID);
		others.sort((a, b) => esteemRank(b) - esteemRank(a));
		const ordered = fest.guests.includes(hostID) ? [...others, hostID] : others;
		for (const id of ordered) {
			if (!(String(id) in fest.outcomes)) return id;
		}
		return null;
	});

	const mustAccept = $derived(
		currentPlayerID != null && fest.acceptDuels.includes(currentPlayerID),
	);

	// ── Active roll outcome (fallback fetch) ─────────────────────────────────
	// If our own roll has resolved but the parent's activeRoll is gone (e.g. it
	// was cleared), fetch the roll once to recover the outcome.
	let fetchedRollOutcome = $state<'make' | 'mar' | null>(null);
	let fetchedForRollID = $state<number | null>(null);
	$effect(() => {
		const haveOwnActive = activeRoll && myRollID != null && activeRoll.id === myRollID;
		if (haveOwnActive) {
			fetchedRollOutcome = rollOutcome ?? null;
			fetchedForRollID = myRollID;
			return;
		}
		if (myRollID != null && myOutcome == null && fetchedForRollID !== myRollID) {
			fetchedForRollID = myRollID;
			getRoll(myRollID)
				.then((r: { roll: DiceRoll }) => { fetchedRollOutcome = r.roll.outcome ?? null; })
				.catch(() => { fetchedRollOutcome = null; });
		}
	});
	const myEffectiveOutcome = $derived<'make' | 'mar' | null>(
		(activeRoll && myRollID != null && activeRoll.id === myRollID
			? rollOutcome
			: fetchedRollOutcome) ?? null,
	);

	// ── Live refresh ─────────────────────────────────────────────────────────
	function onFestEvent(e: Event) {
		const d = (e as CustomEvent<{ plan_id: number }>).detail;
		if (plan && d?.plan_id === plan.id) onPlansChanged();
	}
	const FEST_EVENTS = [
		'uneasy:festivity.guest_rolled',
		'uneasy:festivity.guest_chose',
		'uneasy:festivity.host_chose',
		'uneasy:festivity.insist_host_mar',
		'uneasy:festivity.phase_changed',
		'uneasy:festivity.challenge_issued',
		'uneasy:festivity.challenge_declined',
		'uneasy:festivity.duel_triggered',
	];
	onMount(() => { for (const ev of FEST_EVENTS) window.addEventListener(ev, onFestEvent); });
	onDestroy(() => { for (const ev of FEST_EVENTS) window.removeEventListener(ev, onFestEvent); });

	// ── Complete (host) ──────────────────────────────────────────────────────
	let completeBusy = $state(false);
	let completeError = $state('');
	async function onComplete() {
		if (!plan || completeBusy) return;
		completeBusy = true; completeError = '';
		try { await completePlan(plan.id); onPlansChanged(); }
		catch (e) { completeError = e instanceof Error ? e.message : 'Could not complete plan.'; }
		finally { completeBusy = false; }
	}
</script>

{#if mode === 'prep'}
	<PrepForm {ctx} />

{:else if plan}
	<ResolvingCard {plan} {players}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID} />

		<p class="choices-note">
			Phase: <strong>{fest.phase || '(starting)'}</strong>
			· host: <strong>{playerName(players, plan.preparer_id)}</strong>
		</p>

		{#if fest.phase === 'socializing'}
			<p class="choices-note muted">
				Turn order is flexible: if it matters, players with a 
				lower Esteem rank must roll (or opt out) first. Decide in the chat.
			</p>
		{/if}

		<GuestList {plan} {fest} {players} {rankings} />

		{#if fest.pendingChallenge}
			<ChallengeBanner
				planID={plan.id}
				challenge={fest.pendingChallenge}
				{players} {currentPlayerID} {mustAccept} {onPlansChanged}
			/>
		{/if}

		{#if fest.phase === 'socializing' && iAmGuest && !fest.pendingChallenge && myOutcome == null}
			<SocializingTurn
				{plan} {fest} {players} {assets} {currentPlayerID}
				{currentTurnID} {myRollID} {myEffectiveOutcome} {onPlansChanged}
			/>
		{/if}

		{#if iHaveIOU && fest.phase !== 'done' && !fest.pendingChallenge}
			<InsistFlow {plan} {fest} {players} {assets} {onPlansChanged} />
		{/if}

		{#if fest.phase === 'host_choosing'}
			<HostChoosing {plan} {fest} {players} {assets} {amHost} {onPlansChanged} />
		{/if}

		{#if fest.centeredAssetIDs.length > 0}
			<p class="choices-note muted">
				Center of the table:
				{fest.centeredAssetIDs.map(id => assetName(assets, id)).join(', ')}
			</p>
		{/if}

		{#if fest.phase === 'done'}
			<div class="complete-section">
				<p class="choices-applied">The festivity has wound down.</p>
				{#if completeError}<p class="res-error">{completeError}</p>{/if}
				{#if amHost}
					<button class="action-btn primary" onclick={onComplete} disabled={completeBusy}>
						{completeBusy ? '…' : 'Complete plan'}
					</button>
				{/if}
			</div>
		{/if}

	</ResolvingCard>
{/if}
