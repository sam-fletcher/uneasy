<!-- Liaise/DelayReveal.svelte
  Delay reveal — the plan is pending at row 0 until both faces are in. Rendered by
  MainEventView for every player when the row_state kind is 'await_delay_reveal'.

  No panel header here — the page draws the open step's heading above this (a delay
  reveal has no plan card to draw it in).
-->
<script lang="ts">
	import SetPiece from '../shared/SetPiece.svelte';
	import QuotedText from '../shared/QuotedText.svelte';
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
	<div class="step-groups">
		<!-- The meeting is a set piece: the scene, then the two peers face to face. The
		     step heading above says whose move it is; the difficulty line is not here
		     because a liaison has no roll to be hard. -->
		{#if clState.partnerID != null && (preparerMeetingPeer || partnerMeetingPeerAsset)}
			<SetPiece
				{players}
				a={{ playerID: plan.preparer_id, peer: preparerMeetingPeer?.name ?? '?' }}
				b={{ playerID: clState.partnerID, peer: partnerMeetingPeerAsset?.name ?? '?' }}
				connector="meets"
				scene={plan.preparation_notes}
				label="The meeting"
			/>
		{:else if plan.preparation_notes}
			<QuotedText text={plan.preparation_notes}>
				{#snippet label()}The scene{/snippet}
			</QuotedText>
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
				prompt="Pick a die face to set the delay"
			/>
		{/if}
	</div>
</div>
