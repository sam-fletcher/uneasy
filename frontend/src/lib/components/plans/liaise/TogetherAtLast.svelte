<!-- Liaise/TogetherAtLast.svelte
  Phase 1: Together At Last. The pair set the scene of their meeting in the chat;
  whoever holds the preparer's seat moves the meeting on.
-->
<script lang="ts">
	import { advanceLiaise, type Plan } from '$lib/api';
	import EndsNote from '../shared/EndsNote.svelte';
	import Buffet from '../shared/Buffet.svelte';
	import { LIAISE_BUFFET } from '$lib/plans/buffets';
	import { playerName } from '../shared';
	import type { PlanContext } from '../types';
	import { clStateOf, type Seats } from './shared';

	let { ctx, plan, seats, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		seats: Seats;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const players = $derived(ctx.players);
	const clState = $derived(clStateOf(plan));
	const onPlansChanged = $derived(ctx.onPlansChanged);

	// ── Advance (whoever holds the preparer's seat) ──────────────────────────────────────────────
	let advanceBusy = $state(false);
	async function onAdvance(p: Plan) {
		if (advanceBusy) return;
		advanceBusy = true; error = '';
		try {
			await advanceLiaise(p.id);
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not advance phase.';
		} finally { advanceBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="ft-prompt">
		{playerName(players, plan.preparer_id)} and
		{playerName(players, clState.partnerID)} — set the scene of your
		meeting together in the chat. Where are you? What are you doing?
		Are you sharing a meal, meeting under a bridge under the cover
		of night, or perhaps something more intimate?
	</p>
	{#if seats.canAdvance}
		<p class="choices-note muted">
			{#if seats.standingIn}
				As the Make Demands winner, moving the meeting on is yours.
				When {playerName(players, plan.preparer_id)} and
				{playerName(players, clState.partnerID)} are ready, advance the liaison.
			{:else}
				When you're both ready, advance the liaison.
			{/if}
		</p>
		<button class="action-btn primary"
			onclick={() => onAdvance(plan)} disabled={advanceBusy}>
			{advanceBusy ? '…' : 'Advance to Secrets We Keep'}
		</button>
		<EndsNote text="Advancing ends the meeting scene. Secrets We Keep begins." />
	{:else}
		<p class="choices-note muted">
			Once you're both ready, {seats.holderName}{#if seats.preparerSeatHolder !== plan.preparer_id}, the Make Demands winner,{/if} will advance the liaison.
		</p>
	{/if}
</div>
<!-- What the two choices to come can bring. A liaison rolls nothing, so there is no
     make or mar to preview. Only the two players: a bystander is given nothing to
     read here, and once the options themselves are on screen this would repeat them. -->
<Buffet tabs={LIAISE_BUFFET} />
