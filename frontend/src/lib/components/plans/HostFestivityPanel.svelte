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
	import Buffet from './festivity/Buffet.svelte';
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

	// Roll difficulty = the host's esteem status (6 − rank, min 1), the same for
	// every guest. Mirrors gamepkg.HostFestivityDifficulty so the button can show
	// it without a server round-trip.
	const difficulty = $derived(Math.max(6 - esteemRank(plan?.preparer_id ?? null), 1));

	// A guest who has rolled but not yet recorded an outcome is mid-turn. Their
	// turn serializes play: everyone else's roll / opt-out is held until they
	// finish (best-effort — the backend doesn't hard-lock concurrent rolls).
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

	// ── Favour trackers ──────────────────────────────────────────────────────
	// Guests who currently hold an IOU (rolled a make → may force a host mar).
	const iouHolders = $derived(fest.guestIOUs);
	// Guests who grant the host a free make (rolled mar or opted out).
	const hostMakeOwed = $derived(
		fest.guests.filter(id => {
			const oc = fest.outcomes[String(id)];
			return oc === 'mar' || oc === 'opt_out';
		}),
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

		<div class="fest-phasebar">
			<ol class="fest-steps" aria-label="Festivity progress">
				{#each [['socializing', 'Socializing'], ['host_choosing', 'Host choosing'], ['done', 'Done']] as [key, label] (key)}
					<li class:current={fest.phase === key}>{label}</li>
				{/each}
			</ol>
			<span class="fest-host">Host: <strong>{playerName(players, plan.preparer_id)}</strong></span>
		</div>

		{#if fest.phase === 'socializing'}
			{#if amHost}
				<p class="choices-note">
					As host: Introduce the event in the chat. Where is it taking place, and
					what kind of event is it?
				</p>
			{/if}
			<p class="choices-note choices-emph">
				Roleplay your characters making their entrances and socializing amongst each other. 
				At any point each attendee may make a dice roll, and choose one option from the Make or Mar list.
			</p>
			<p class="choices-note muted">
				Turn order is flexible: if it matters, players with a
				lower Esteem rank must roll (or opt out) first. Resolve in the chat.
			</p>
		{/if}

		{#if fest.phase === 'socializing' || fest.phase === 'host_choosing'}
			<Buffet />
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
				{currentTurnID} {myRollID} {myEffectiveOutcome}
				{difficulty} {blockedByOtherRoll} {activeRollerID} {onPlansChanged}
			/>
		{/if}

		<!-- IOU tracker: who can force the host into a mar, plus the holder's action. -->
		{#if fest.phase !== 'done'}
			<div class="choices-section">
				<p class="choices-header">The host's Mars</p>
				{#if iouHolders.length === 0}
					<p class="choices-note muted">
						A guest who rolls a Make can insist the host to choose a Mar at any point during the festivity. No one has that power yet.
					</p>
				{:else}
					<ul class="plan-notes fest-ledger">
						{#each iouHolders as holderID (holderID)}
							<li>
								<strong>{playerName(players, holderID)}</strong>{#if holderID === currentPlayerID}<em> (you)</em>{/if}
								— may insist the host take a Mar
							</li>
						{/each}
					</ul>
				{/if}
				{#if fest.hostMarInsists.length > 0}
					<p class="choices-note muted">Insisted upon the host so far: {fest.hostMarInsists.join(', ')}</p>
				{/if}
				{#if iHaveIOU && !fest.pendingChallenge}
					<InsistFlow {plan} {players} {assets} {onPlansChanged} />
				{/if}
			</div>
		{/if}

		<!-- Host's free makes: owed by mar/opt-out guests, taken by the host. -->
		{#if fest.phase === 'host_choosing'}
			<HostChoosing {plan} {fest} {players} {assets} {amHost} {onPlansChanged} />
		{:else if fest.phase === 'socializing' && hostMakeOwed.length > 0}
			<div class="choices-section">
				<p class="choices-header">The host's free Makes</p>
				<p class="choices-note muted">
					Before the event winds down, the host gets one Make for each:
				</p>
				<ul class="plan-notes fest-ledger">
					{#each hostMakeOwed as owedID (owedID)}
						<li>
							<strong>{playerName(players, owedID)}</strong>
							— {fest.outcomes[String(owedID)] === 'opt_out' ? 'opted out' : 'rolled a mar'}
						</li>
					{/each}
				</ul>
			</div>
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

<style>
	.fest-phasebar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-bottom: 0.75rem;
		padding-bottom: 0.6rem;
		border-bottom: 1px solid var(--color-border-warm);
	}
	.fest-steps {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.4rem;
		list-style: none;
		margin: 0;
		padding: 0;
		font-size: 0.85rem;
		color: var(--color-text-faint);
	}
	.fest-steps li + li::before {
		content: '›';
		margin-right: 0.4rem;
		color: var(--color-text-faint);
	}
	.fest-steps li.current {
		color: var(--color-accent);
		font-weight: 500;
	}
	.fest-host {
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}
	.choices-emph {
		border-left: 2px solid var(--color-accent);
		padding-left: 0.6rem;
		color: var(--color-accent-hover);
	}
	.fest-ledger {
		margin: 0.25rem 0;
		padding-left: 1.25rem;
	}
	.fest-ledger li {
		margin: 0.15rem 0;
	}
</style>
