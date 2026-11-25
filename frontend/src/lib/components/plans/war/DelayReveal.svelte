<!-- MakeWar/DelayReveal.svelte
  Per-participant die-face reveal that sets the war's delay (ceil average).
  Non-participants see this block only as a "waiting…" note; the join
  buttons live in WarStatus.
-->
<script lang="ts">
	import SimultaneousRevealInput from '../SimultaneousRevealInput.svelte';
	import DelayThresholdNote from '../shared/DelayThresholdNote.svelte';

	let {
		delayRevealID, currentPlayerID, amParticipant, revealParticipants,
		currentRow, endingMode, slotSpent, preparerName,
	}: {
		delayRevealID: number;
		currentPlayerID: number | null;
		amParticipant: boolean;
		revealParticipants: { player_id: number; display_name: string }[];
		currentRow: number;
		endingMode: string | null;
		slotSpent: boolean;
		preparerName: string;
	} = $props();
</script>

<div class="choices-section">
	<p class="choices-note">
		The row delay is the average of each participant's choice, rounded up.
	</p>
	<!-- Everyone sees the line, participant or not: the faces are chosen rather
	     than rolled, so overflowing row 13 is a decision the table is making. -->
	<DelayThresholdNote {currentRow} {endingMode} {slotSpent} {preparerName} />
	{#if amParticipant && currentPlayerID != null}
		<SimultaneousRevealInput
			revealID={delayRevealID}
			{currentPlayerID}
			participants={revealParticipants}
			prompt=""
		/>
	{/if}
</div>
