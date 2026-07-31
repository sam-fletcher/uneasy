<!-- plans/shared/DelayThresholdNote.svelte
  The line every Make War / Clandestinely Liaise delay reveal is played
  against: how high the average may go before the plan lands past row 13, and
  what happens if it does.

  The faces are CHOSEN, not rolled (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md, the
  2026-07-27 ruling), which is what makes overflow avoidable rather than
  arbitrary — so the threshold has to be stated outright. Submission is
  simultaneous, so no one participant can guarantee the average; everyone can
  still know the line.

  Renders nothing while nothing can overflow: the delay is ceil(average of d6
  faces), which caps at 6, so a row with six or more rows left behind it has no
  reachable overflow to warn about.
-->
<script lang="ts">
	import { FINAL_ROW } from '../shared';

	let { currentRow, endingMode, slotSpent, preparerName }: {
		currentRow: number;
		/** game.ending_mode — null until the table's row 7 → 8 vote settles it. */
		endingMode: string | null;
		/** Whether the PREPARER's one Explosive Finale plan is already spent.
		 *  It is their slot the collapse would spend, not the submitter's. */
		slotSpent: boolean;
		preparerName: string;
	} = $props();

	/** The highest delay that still fits on the record. */
	const maxDelay = $derived(FINAL_ROW - currentRow);

	/** ceil(avg) caps at 6, so below this there is nothing to warn about. */
	const canOverflow = $derived(maxDelay < 6);

	const outcome = $derived(
		endingMode === 'explosive_finale' && !slotSpent
			? `under an Explosive Finale it collapses onto row ${FINAL_ROW} instead, and that ` +
				`spends ${preparerName}'s one Explosive Finale plan.`
			: endingMode === 'explosive_finale'
				? `${preparerName}'s Explosive Finale plan is already spent, so there is nowhere ` +
					'for it to go — the plan falls through and you need to choose something else.'
				: 'nothing may be placed there, so the plan falls through and you need to choose something else.',
	);
</script>

{#if canOverflow}
	<p class="delay-threshold" role="note">
		{#if maxDelay < 1}
			<span class="delay-threshold-head">There is no room left on the record.</span>
			Any delay at all lands past row {FINAL_ROW}: {outcome}
		{:else}
			<span class="delay-threshold-head">Keep the average at {maxDelay} or below.</span>
			The delay is everyone's face averaged and rounded up. Anything higher lands past
			row {FINAL_ROW}: {outcome}
		{/if}
	</p>
{/if}

<style>
	/* The warning family — orange is the one warning trio (app.css), and an
	   overflow here costs either the bonus plan or the whole declaration. */
	.delay-threshold {
		margin: 0.35rem 0 0;
		padding: 0.45rem 0.55rem;
		font-size: 0.78rem;
		line-height: 1.45;
		background: var(--color-warning-bg);
		border: 1px solid var(--color-warning-border);
		border-radius: 5px;
		color: var(--color-text-secondary);
	}

	.delay-threshold-head {
		display: block;
		color: var(--color-warning);
	}
</style>
