<!-- Liaise/WhenWillISeeYouAgain.svelte
  Phase 4: When Will I See You Again. Each seat reveals a die face (0–6) for how many
  rows until the next liaison; revealing the last one resolves the plan.
-->
<script lang="ts">
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import SimultaneousRevealInput from '../SimultaneousRevealInput.svelte';
	import EndsNote from '../shared/EndsNote.svelte';
	import type { Plan } from '$lib/api';
	import type { PlanContext } from '../types';
	import { clStateOf, type Seats } from './shared';

	let { ctx, plan, seats }: {
		ctx: PlanContext;
		plan: Plan;
		seats: Seats;
	} = $props();

	const currentPlayerID = $derived(ctx.currentPlayerID);
	const players = $derived(ctx.players);
	const clState = $derived(clStateOf(plan));
</script>

<div class="choices-section">
	<p class="ft-prompt">
		Reveal a die face (1–6) to choose how many rows until the next liaison.
		If either of you picks 0, no follow-up is scheduled.
	</p>
	{#if seats.amLockedOut}
		<p class="choices-note"><PlayerName {players} id={seats.preparerSeatHolder} /> is revealing a face for you.</p>
	{:else if clState.redelayRevealID != null && currentPlayerID != null}
		<!-- actingAs: the reveal's entries are keyed by seat, so a stand-in
		     for the preparer (or one player holding both seats) submits
		     under the seat(s) they hold, not their own id. -->
		<SimultaneousRevealInput
			revealID={clState.redelayRevealID}
			{currentPlayerID}
			actingAs={seats.mySeats}
			participants={seats.participants}
			allowZero={true}
			prompt="Pick a row delay (or 0 to part ways):"
		/>
		<EndsNote text="This is the last step: Clandestinely Liaise resolves as soon as you have both revealed." />
	{/if}
</div>
