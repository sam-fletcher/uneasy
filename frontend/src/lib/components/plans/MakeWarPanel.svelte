<!-- MakeWarPanel.svelte
  Prep + full lifecycle UI for Make War (Tier 3, Power, variable delay).

  A Make War plan creates a `war` row and a simultaneous delay reveal at prep
  time. The plan itself sits at row 0 (pending) until the reveal closes; then
  it advances to the rolled row, where "Begin resolution" auto-resolves it
  in one step (no extra prompts) and the follow scene takes over. The war
  persists across rows and ends only via peace or full surrender — so the
  cost-of-battle picker, peace flow, and surrender-claim UI must remain
  visible long after the plan itself has resolved.

  This panel is a dispatcher. The actual phase-specific UI lives in
  `war/`: PrepForm, DelayReveal, WarStatus, CostOfBattle, PeaceFlow,
  SurrenderClaims. The parent owns the war-state fetch and refresh
  lifecycle; children receive a slice + `onChanged` callback that re-fetches.
-->
<script lang="ts">
	import './planPanel.css';
	import { onMount } from 'svelte';
	import { useWindowEvents } from '$lib/useWindowEvents';
	import { WAR_EVENTS } from '$lib/ws';
	import {
		getWarState,
		type WarStateResponse, type WarParticipantInfo,
	} from '$lib/api';
	import { playerName, parseResolutionData } from './shared';

	import PrepForm from './war/PrepForm.svelte';
	import DelayReveal from './war/DelayReveal.svelte';
	import WarStatus from './war/WarStatus.svelte';
	import CostOfBattle from './war/CostOfBattle.svelte';
	import PeaceFlow from './war/PeaceFlow.svelte';
	import SurrenderClaims from './war/SurrenderClaims.svelte';

	import type { PlanPanelProps } from './types';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	// Make War uses one underlying view for both resolve and alwaysOn
	// dispatches — both fall through to "the war view".
	const isWarView = $derived(mode === 'resolve' || mode === 'alwaysOn');

	// ── War-mode state ───────────────────────────────────────────────────────
	let war = $state<WarStateResponse | null>(null);
	let warError = $state('');
	let actionError = $state('');

	const planMW = $derived(plan ? (parseResolutionData(plan).make_war ?? {}) : {});
	const delayRevealID = $derived(planMW.delay_reveal_id ?? null);

	async function refreshWar() {
		if (!plan) return;
		try {
			war = await getWarState(plan.id);
			warError = '';
		} catch (e) {
			// 404 just means the war row hasn't been created yet — fine.
			const msg = e instanceof Error ? e.message : '';
			if (!/no war/i.test(msg)) warError = msg || 'Could not load war state';
			war = null;
		}
	}

	useWindowEvents(WAR_EVENTS, () => { refreshWar(); });
	onMount(() => { if (isWarView) refreshWar(); });

	// Re-fetch when the plan changes.
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (isWarView && plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			refreshWar();
		}
	});

	// ── Derived participant views ────────────────────────────────────────────
	const myPart = $derived<WarParticipantInfo | null>(
		war && currentPlayerID != null
			? war.participants.find(p => p.player_id === currentPlayerID) ?? null
			: null,
	);
	const amParticipant = $derived(myPart != null);
	const amSurrendered = $derived(myPart?.surrendered_at_row != null);
	const amFullParticipant = $derived(
		myPart != null && myPart.entry_payment_complete && !amSurrendered,
	);

	const proposal = $derived(war?.open_proposal ?? null);
	const myClaims = $derived(
		war?.open_claims.filter(c => c.claimant_id === currentPlayerID) ?? [],
	);

	const revealParticipants = $derived(
		war?.participants.map(p => ({
			player_id: p.player_id,
			display_name: playerName(players, p.player_id),
		})) ?? [],
	);

	// Don't render anything if the plan is fully resolved AND the war ended.
	const shouldHide = $derived(
		isWarView
		&& plan != null
		&& plan.status === 'resolved'
		&& war != null
		&& war.status === 'ended',
	);

	const setActionError = (msg: string) => { actionError = msg; };
</script>

{#if mode === 'prep'}
	<PrepForm {gameID} {players} {currentPlayerID} {onPlanPrepared} />

{:else if plan && !shouldHide}
	<div class="plan-panel resolving">
		<div class="plan-header">
			<span class="plan-badge resolving-badge">
				{war?.status === 'ended' ? 'War (ended)'
					: war?.status === 'active' ? 'War — ongoing'
					: 'War — pending'}
			</span>
			<strong class="plan-title">Make War</strong>
			<span class="plan-preparer">by {playerName(players, plan.preparer_id)}</span>
		</div>
		{#if plan.preparation_notes}
			<p class="plan-notes">"{plan.preparation_notes}"</p>
		{/if}
		{#if warError}<p class="res-error">{warError}</p>{/if}
		{#if actionError}<p class="res-error">{actionError}</p>{/if}

		{#if war && delayRevealID != null && plan.row_number == null && war.status === 'active'}
			<DelayReveal
				{delayRevealID}
				{currentPlayerID}
				{amParticipant}
				{revealParticipants}
			/>
		{/if}

		{#if war}
			<WarStatus
				{war}
				planID={plan.id}
				{players}
				{amParticipant}
				onChanged={refreshWar}
				setError={setActionError}
			/>

			<CostOfBattle
				{war}
				planID={plan.id}
				{players}
				{assets}
				{currentPlayerID}
				{myPart}
				{amFullParticipant}
				onChanged={refreshWar}
				setError={setActionError}
			/>
		{/if}

		{#if proposal}
			<PeaceFlow
				{proposal}
				planID={plan.id}
				{players}
				{currentPlayerID}
				{amFullParticipant}
				onChanged={refreshWar}
				setError={setActionError}
			/>
		{/if}

		{#if myClaims.length > 0}
			<SurrenderClaims
				claims={myClaims}
				planID={plan.id}
				{players}
				{assets}
				onChanged={refreshWar}
				setError={setActionError}
			/>
		{/if}
	</div>
{/if}
