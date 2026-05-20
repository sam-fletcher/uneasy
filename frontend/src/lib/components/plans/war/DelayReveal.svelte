<!-- MakeWar/DelayReveal.svelte
  Per-participant die-face reveal that sets the war's delay (ceil average).
  Non-participants see this block only as a "waiting…" note; the join
  buttons live in WarStatus.
-->
<script lang="ts">
	import SimultaneousRevealInput from '../SimultaneousRevealInput.svelte';

	let { delayRevealID, currentPlayerID, amParticipant, revealParticipants }: {
		delayRevealID: number;
		currentPlayerID: number | null;
		amParticipant: boolean;
		revealParticipants: { player_id: number; display_name: string }[];
	} = $props();
</script>

<div class="choices-section">
	<p class="choices-header">Vote for the war's delay</p>
	<p class="choices-note">
		The row delay is the average of each particpant's choice, rounded up. 
		Other players may join either side below.
	</p>
	{#if amParticipant && currentPlayerID != null}
		<SimultaneousRevealInput
			revealID={delayRevealID}
			{currentPlayerID}
			participants={revealParticipants}
			prompt=""
		/>
	{:else}
		<p class="choices-note muted">
			Waiting for participants to reveal their delay dice…
		</p>
	{/if}
</div>
