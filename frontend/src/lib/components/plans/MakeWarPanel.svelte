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

  It draws no card and no title of its own. The war is named where it is
  reached: by its band at the top of Play, by the title of the sheet the band
  opens, or by `war/WarName` where neither is on screen. What the declarer wrote
  is a finished-step line, as in every other plan, and the rest is the war's own
  steps in the order a player has to act on them. The open step's heading is the
  page's (MainEventView), so there is exactly one.
-->
<script lang="ts">
	import './planPanel.css';
	import { onMount } from 'svelte';
	import { useWindowEvents } from '$lib/useWindowEvents';
	import { WAR_EVENTS } from '$lib/ws';
	import {
		ApiError,
		getWarState,
		type WarStateResponse, type WarParticipantInfo,
	} from '$lib/api';
	import { warPanelHidden } from '$lib/warGate';
	import { useSettledInPlay } from './war/sheetContext';
	import { playerName, parseResolutionData, finaleSlotSpent } from './shared';
	import FinishedStep from './shared/FinishedStep.svelte';
	import LogMark from '../LogMark.svelte';

	import PrepForm from './war/PrepForm.svelte';
	import DelayReveal from './war/DelayReveal.svelte';
	import WarStatus from './war/WarStatus.svelte';
	import CostOfBattle from './war/CostOfBattle.svelte';
	import PeaceFlow from './war/PeaceFlow.svelte';
	import SurrenderClaims from './war/SurrenderClaims.svelte';

	import type { PlanPanelProps } from './types';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	// Make War uses a single "war view" for the resolving plan, the
	// pre-row delay reveal, and the war drawer — anything that isn't
	// the prep form. The branches differ only in which sub-component
	// (DelayReveal vs WarStatus etc.) self-shows based on war state.
	const isWarView = $derived(mode !== 'prep');

	// ── War-mode state ───────────────────────────────────────────────────────
	let war = $state<WarStateResponse | null>(null);
	let warError = $state('');
	let actionError = $state('');

	const planMW = $derived(plan ? (parseResolutionData(plan).make_war ?? {}) : {});
	const delayRevealID = $derived(planMW.delay_reveal_id ?? null);

	// Delay-reveal threshold inputs. The slot at stake is the DECLARER's — a
	// collapse onto row 13 spends theirs, whoever else submitted a high face.
	const preparerName = $derived(plan ? playerName(players, plan.preparer_id) : '');
	const preparerSlotSpent = $derived(plan ? finaleSlotSpent(ctx.plans, plan.preparer_id) : false);

	async function refreshWar() {
		if (!plan) return;
		try {
			war = await getWarState(plan.id);
			warError = '';
		} catch (e) {
			// 404 just means the war row hasn't been created yet — fine.
			// Checked on the status, not the message: this used to test
			// /no war/i against the server's prose, and the server already has
			// two wordings for it (plan_make_war.go, wars.go). A third that
			// didn't happen to contain "no war" would have started showing a
			// spurious error here.
			if (!(e instanceof ApiError && e.status === 404)) {
				warError = e instanceof Error ? e.message : 'Could not load war state';
			}
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

	// Nothing to show once the plan has resolved and the war has ended, unless
	// a surrender claim is still open (see warPanelHidden).
	const shouldHide = $derived(isWarView && warPanelHidden(plan, war));

	const setActionError = (msg: string) => { actionError = msg; };

	// In a war's sheet while the play area is settling this war (a cost, a vote or a
	// claim is holding up the row): those forms stay in the play area, and the sheet
	// says so. See war/sheetContext.ts.
	const settledInPlay = useSettledInPlay();
	const holdsRow = $derived(
		war != null && (war.status === 'ended'
			? war.open_claims.length > 0
			: war.outstanding_costs.length > 0 || war.open_proposal != null),
	);
	const pointToPlay = $derived(settledInPlay() && holdsRow);

	// What the declarer wrote, as a finished-step line. A long one opens in place.
	const LONG_NOTE = 40;
	const notes = $derived((plan?.preparation_notes ?? '').trim());
</script>

{#if mode === 'prep'}
	<PrepForm {ctx} />

{:else if plan && !shouldHide}
	<div class="plan-panel resolving">
		{#if notes}
			<div class="done-steps">
				{#if notes.length > LONG_NOTE}
					<FinishedStep>
						{#snippet mark()}<LogMark family="marginalia" />{/snippet}
						{preparerName}: “{notes}”
						{#snippet detail()}<p class="step-quote">“{notes}”</p>{/snippet}
					</FinishedStep>
				{:else}
					<FinishedStep>
						{#snippet mark()}<LogMark family="marginalia" />{/snippet}
						{preparerName}: “{notes}”
					</FinishedStep>
				{/if}
			</div>
		{/if}
		{#if warError}<ErrorText message={warError} variant="panel" />{/if}
		{#if actionError}<ErrorText message={actionError} variant="panel" />{/if}

		{#if war && delayRevealID != null && plan.row_number == null && war.status === 'active'}
			<DelayReveal
				{delayRevealID}
				{currentPlayerID}
				{amParticipant}
				{revealParticipants}
				currentRow={ctx.currentRow}
				endingMode={ctx.endingMode}
				slotSpent={preparerSlotSpent}
				{preparerName}
			/>
		{/if}

		<!-- Order is by what the viewer has to DO, not by what the war is. The
		     panel is the whole play area while a cost of battle or surrender
		     claim blocks the row, and on a phone the Sides block alone is most of a
		     screen — Accept / Reject and the payment form used to sit below it, out
		     of sight. Sides (review, joining) goes last. -->
		{#if pointToPlay}
			<p class="holds-row">
				This war is holding up the row. Close this sheet to settle it in the play area.
			</p>
		{/if}

		{#if proposal && !pointToPlay}
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

		{#if myClaims.length > 0 && !pointToPlay}
			<SurrenderClaims
				claims={myClaims}
				planID={plan.id}
				{players}
				{assets}
				onChanged={refreshWar}
				setError={setActionError}
			/>
		{/if}

		{#if war}
			<CostOfBattle
				{war}
				planID={plan.id}
				{players}
				{assets}
				{currentPlayerID}
				{myPart}
				{amFullParticipant}
				settledInPlay={pointToPlay}
				onChanged={refreshWar}
				setError={setActionError}
			/>

			<WarStatus
				{war}
				{plan}
				{players}
				{currentPlayerID}
				{amParticipant}
				begun={plan.row_number != null}
				onChanged={refreshWar}
				setError={setActionError}
			/>
		{/if}
	</div>
{/if}

<style>
	.holds-row {
		margin: 0;
		font-size: var(--text-small);
		line-height: 1.5;
		color: var(--color-text-secondary);
	}
</style>
