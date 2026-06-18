<!-- HostFestivityPanel.svelte
  Prep + resolve UI for Host Festivity (Tier 3, Esteem, delay 6).

  There are no phases or turns. While the plan is resolving, the event is one
  open stretch of socializing: guests roll/opt-out and pick a make/mar option,
  the host takes their extra makes (their spoils — one for hosting, one per
  guest who marred or opted out), and successful guests inflict mars on the
  host. The only ordering rule is that a single roll-and-choice must conclude
  before the next action starts. The host winds the event down (End the event)
  once everything is settled.

  Each guest creates their own dice roll via /guest-roll. The active roll is
  surfaced through the parent's <DiceRollPanel> using the standard
  activeRoll/rollOutcome props. Once the roll resolves, the guest submits a
  make/mar choice through /guest-choice (or /challenge-duel for duels).

  This panel is a dispatcher. Role-specific UI lives in `festivity/`: PrepForm,
  GuestList, ChallengeBanner, SocializingTurn, InsistFlow, HostChoosing. The
  parent owns the `fest` derivation, the active-roll-fallback fetch, and the WS
  subscription; children get slices + `onPlansChanged`.
-->
<script lang="ts">
	import './planPanel.css';
	import { onMount, onDestroy } from 'svelte';
	import {
		endFestivity,
		getRoll,
		type DiceRoll,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import TargetPlanDemandOverlay from './demand/TargetPlanDemandOverlay.svelte';
	import { assetName, parseResolutionData } from './shared';

	import PrepForm from './festivity/PrepForm.svelte';
	import Buffet from './festivity/Buffet.svelte';
	import GuestList from './festivity/GuestList.svelte';
	import ChallengeBanner from './festivity/ChallengeBanner.svelte';
	import SocializingTurn from './festivity/SocializingTurn.svelte';
	import InsistFlow from './festivity/InsistFlow.svelte';
	import HostChoosing from './festivity/HostChoosing.svelte';
	import { festivityEndable, earnedHostMakes, type FestRes } from './festivity/options';

	import type { PlanPanelProps } from './types';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const rankings = $derived(ctx.rankings);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);
	const rollOutcome = $derived(ctx.rollOutcome);
	const activeRoll = $derived(ctx.activeRoll);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	// ── Resolve: parse resolution_data ───────────────────────────────────────
	// Every player at the table is a guest (the roster is fixed once a game
	// starts), so the guest list is derived from `players`, not stored in
	// resolution_data.
	const fest = $derived.by<FestRes>(() => {
		const f = parseResolutionData(plan).festivity ?? {};
		return {
			guests: players.map(p => p.id),
			outcomes: f.outcomes ?? {},
			guestMakes: f.guest_makes ?? {},
			guestMars: f.guest_mars ?? {},
			hostMakesTaken: f.host_makes_taken ?? [],
			guestRollIDs: f.guest_roll_ids ?? {},
			guestIOUs: f.guest_ious ?? [],
			hostMarInsists: f.host_mar_insists ?? [],
			acceptDuels: f.accept_duels ?? [],
			pendingDuelPlanID: f.pending_duel_plan_id ?? null,
			pendingChallenge: f.pending_challenge ?? null,
			centeredAssetIDs: f.centered_asset_ids ?? [],
		};
	});

	// The event is open while the plan is resolving; it ends when the host winds
	// it down. No phases.
	const isResolving = $derived(plan?.status === 'resolving');

	const meKey = $derived(currentPlayerID == null ? '' : String(currentPlayerID));
	const amHost = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const iAmGuest = $derived(currentPlayerID != null && fest.guests.includes(currentPlayerID));
	const myOutcome = $derived(meKey ? fest.outcomes[meKey] ?? null : null);
	const myRollID = $derived(meKey ? fest.guestRollIDs[meKey] ?? null : null);
	const iHaveIOU = $derived(currentPlayerID != null && fest.guestIOUs.includes(currentPlayerID));

	function esteemRank(playerID: number | null): number {
		if (playerID == null) return 999;
		const r = rankings.find(x => x.category === 'esteem' && x.player_id === playerID);
		return r?.rank ?? 999;
	}

	const mustAccept = $derived(
		currentPlayerID != null && fest.acceptDuels.includes(currentPlayerID),
	);

	// Roll difficulty = the host's esteem status (6 − rank, min 1), the same for
	// every guest. Mirrors gamepkg.HostFestivityDifficulty so the button can show
	// it without a server round-trip.
	const difficulty = $derived(Math.max(6 - esteemRank(plan?.preparer_id ?? null), 1));

	// A guest who has rolled but not yet recorded an outcome holds a
	// roll-and-choice in progress; it must conclude before anyone else may act.
	const activeRollerID = $derived.by<number | null>(() => {
		for (const id of fest.guests) {
			const k = String(id);
			if (k in fest.guestRollIDs && !(k in fest.outcomes)) return id;
		}
		return null;
	});
	const blockedByOtherRoll = $derived(
		activeRollerID != null && activeRollerID !== currentPlayerID,
	);

	// ── Action gates ─────────────────────────────────────────────────────────
	// Extra makes the host has earned but not yet taken.
	const hostMakesLeft = $derived(
		plan == null ? 0 : earnedHostMakes(fest, plan.preparer_id) - fest.hostMakesTaken.length,
	);
	// Whether the host may now wind the event down.
	const endable = $derived(plan != null && festivityEndable(fest, plan.preparer_id));

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
		'uneasy:festivity.updated',
		'uneasy:festivity.challenge_issued',
		'uneasy:festivity.challenge_declined',
		'uneasy:festivity.duel_triggered',
	];
	onMount(() => { for (const ev of FEST_EVENTS) window.addEventListener(ev, onFestEvent); });
	onDestroy(() => { for (const ev of FEST_EVENTS) window.removeEventListener(ev, onFestEvent); });

	// ── End the event (host) ─────────────────────────────────────────────────
	let endBusy = $state(false);
	let endError = $state('');
	async function onEndEvent() {
		if (!plan || endBusy) return;
		endBusy = true; endError = '';
		try { await endFestivity(plan.id); onPlansChanged(); }
		catch (e) { endError = e instanceof Error ? e.message : 'Could not end the event.'; }
		finally { endBusy = false; }
	}
</script>

{#if mode === 'prep'}
	<PrepForm {ctx} />

{:else if plan}
	<ResolvingCard {plan} {players}>
		<TargetPlanDemandOverlay {plan} {plans} {players} {assets} {currentPlayerID} />

		{#if isResolving}
			<p class="choices-note">
				{#if amHost}
					As host: Introduce the event in the chat. Where is it taking place, and
					what kind of event is it?
				{:else}
					Make your entrance in chat, socialize, and roll for a Make — but risk a Mar!
				{/if}
			</p>
			<Buffet />
		{/if}

		<GuestList {plan} {fest} {players} {currentPlayerID} />

		{#if fest.pendingChallenge}
			<ChallengeBanner
				planID={plan.id}
				challenge={fest.pendingChallenge}
				{players} {currentPlayerID} {mustAccept} {onPlansChanged}
			/>
		{/if}

		<!-- Your move(s): each control appears only for the player who can use it. -->
		{#if isResolving && iAmGuest && !amHost && !fest.pendingChallenge && myOutcome == null}
			<SocializingTurn
				{plan} {fest} {players} {assets} {currentPlayerID}
				{myRollID} {myEffectiveOutcome}
				{difficulty} {blockedByOtherRoll} {activeRollerID} {onPlansChanged}
			/>
		{/if}

		{#if isResolving && amHost && hostMakesLeft > 0}
			<HostChoosing {plan} {fest} {players} {assets} {onPlansChanged} />
		{/if}

		{#if isResolving && iHaveIOU && !fest.pendingChallenge && !blockedByOtherRoll}
			<div class="choices-section">
				<p class="choices-header">Your hold over the host</p>
				<InsistFlow {plan} {players} {assets} {onPlansChanged} />
			</div>
		{/if}

		{#if fest.centeredAssetIDs.length > 0}
			<p class="choices-note muted">
				Center of the table:
				{fest.centeredAssetIDs.map(id => assetName(assets, id)).join(', ')}
			</p>
		{/if}

		{#if isResolving && amHost}
			<div class="complete-section">
				{#if endError}<p class="res-error">{endError}</p>{/if}
				<button
					class="action-btn primary"
					onclick={onEndEvent}
					disabled={endBusy || !endable}
					title={endable ? '' : 'Everyone must settle first: all guests chosen, all your Makes taken, all Mars inflicted.'}
				>
					{endBusy ? '…' : 'End the event'}
				</button>
			</div>
		{:else if !isResolving}
			<p class="choices-applied">The festivity has wound down.</p>
		{/if}

	</ResolvingCard>
{/if}

