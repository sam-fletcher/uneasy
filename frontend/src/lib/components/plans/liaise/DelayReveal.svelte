<!-- Liaise/DelayReveal.svelte
  Delay reveal — the plan is pending at row 0 until both faces are in. Rendered by
  MainEventView for every player when the row_state kind is 'await_delay_reveal'.

  No panel header here — the page draws the open step's heading above this (a delay
  reveal has no plan card to draw it in).
-->
<script lang="ts">
	import Quote from '$lib/components/shared/Quote.svelte';
	import AssetName from '$lib/components/shared/AssetName.svelte';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import SimultaneousRevealInput from '../SimultaneousRevealInput.svelte';
	import DelayThresholdNote from '../shared/DelayThresholdNote.svelte';
	import { playerName, finaleSlotSpent } from '../shared';
	import type { Plan } from '$lib/api';
	import type { PlanContext } from '../types';
	import { clStateOf, liaiseSeats } from './shared';

	let { ctx, plan }: { ctx: PlanContext; plan: Plan } = $props();

	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const plans = $derived(ctx.plans);

	const clState = $derived(clStateOf(plan));
	// No Make Demands winner can hold a seat yet: perform_steps is post-roll only.
	const seats = $derived(liaiseSeats(ctx, plan, clState, null));

	// The two specific peers brought to the meeting (by stored id, seat-agnostic),
	// for the delay-reveal play-area summary. Looked up from the shared asset list
	// so every viewer — participant or not — can see who is meeting whom.
	const preparerMeetingPeer = $derived(
		assets.find(a => a.id === clState.preparerPeerID) ?? null,
	);
	const partnerMeetingPeerAsset = $derived(
		assets.find(a => a.id === clState.partnerPeerID) ?? null,
	);
</script>

<div class="plan-panel pending">
	{#if plan.preparation_notes}
		<p class="plan-notes"><Quote text={plan.preparation_notes} /></p>
	{/if}
	{#if preparerMeetingPeer || partnerMeetingPeerAsset}
		<p class="ft-prompt cl-meeting-peers">
			<PlayerName {players} id={plan.preparer_id} />'s
			<AssetName name={preparerMeetingPeer?.name ?? '?'} />
			meets
			<PlayerName {players} id={clState.partnerID} />'s
			<AssetName name={partnerMeetingPeerAsset?.name ?? '?'} />
		</p>
	{/if}
	<!-- Everyone sees the line, participant or not: the faces are chosen rather
	     than rolled, so overflowing row 13 is a decision the pair is making.
	     The slot at stake is the PREPARER's, not the submitter's. -->
	<DelayThresholdNote
		currentRow={ctx.currentRow}
		endingMode={ctx.endingMode}
		slotSpent={finaleSlotSpent(plans, plan.preparer_id)}
		preparerName={playerName(players, plan.preparer_id)}
	/>
	{#if seats.amParticipant && clState.delayRevealID != null && currentPlayerID != null}
		<SimultaneousRevealInput
			revealID={clState.delayRevealID}
			{currentPlayerID}
			participants={seats.participants}
			prompt="Pick a die face to set the delay:"
		/>
	{:else}
		<p class="choices-note muted">
			<PlayerName {players} id={plan.preparer_id} /> and
			<PlayerName {players} id={clState.partnerID} /> are settling on a time…
		</p>
	{/if}
</div>
