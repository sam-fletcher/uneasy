<!-- shared/WeightMeter.svelte
  A prologue card's value as a 1–4 "weight", drawn as the house segmented meter
  (adr/PROLOGUE_UX_ROUND2_PLAN.md, decision 2 — the `.seg` idiom from
  plans/shared/DifficultyMeter, shrunk to an ornament). Never stars; stars
  aren't in this app's visual language.

  Shared because Round 2 gives it two homes with the same object in each: the
  tile expansion's card rows (Session 2) and, once suits left the ranking too,
  every per-player card mark on the TrackBoard (Session 3a). A copy in each
  would let the two drift, and they are the *same* number — the one card
  property with no other expression on screen now that the suit is gone.

  Takes the raw card value rather than a pre-computed weight so `cardWeight`
  stays the single mapping: a caller that did its own arithmetic could disagree
  with the tie-break the number exists to explain (see choosing.ts).

  Metrics are custom properties, so a caller in a tight column can shrink the
  meter without forking it — `.mark :global(.weight) { --weight-seg-h: 7px; }`.
-->
<script lang="ts">
	import { cardWeight, MAX_CARD_WEIGHT } from '$lib/prologue/choosing';

	let {
		/** The card's face value as the API spells it ("J", "Q", "K", "A"). */
		value,
		/** Whether the meter narrates itself. False when the caller's own
		 *  wrapper already carries a label that includes the weight — a mark
		 *  reading "wild, wasted" then "weight 3 of 4" is two objects to a
		 *  screen reader and one on the page. */
		describe = true,
		/** Tighter segments for the TrackBoard's ~78px columns. */
		compact = false,
	}: { value: string; describe?: boolean; compact?: boolean } = $props();

	const weight = $derived(cardWeight(value));
	const SEGMENTS = Array.from({ length: MAX_CARD_WEIGHT }, (_, i) => i + 1);
</script>

<span
	class="weight"
	class:compact
	role={describe ? 'img' : undefined}
	aria-label={describe ? `weight ${weight} of ${MAX_CARD_WEIGHT}` : undefined}
	aria-hidden={describe ? undefined : 'true'}
>
	{#each SEGMENTS as s (s)}
		<span class="seg" class:on={s <= weight}></span>
	{/each}
</span>

<style>
	.weight {
		display: inline-flex;
		align-items: center;
		gap: var(--weight-seg-gap, 2px);
		flex: none;
	}
	.seg {
		width: var(--weight-seg-w, 3px);
		height: var(--weight-seg-h, 9px);
		border-radius: 1px;
		background: transparent;
		border: 1px solid var(--color-border-strong);
	}
	.seg.on {
		background: var(--color-text-muted);
		border-color: var(--color-text-muted);
	}
	/* The TrackBoard draws one of these per card held, three columns wide, in a
	   phase column that is a phone at every viewport — so the segments lose
	   their outlines and become bare bars. At this size a 1px border on a 3px
	   box is half the object, and "off" only has to be dimmer than "on", not
	   separately drawn. */
	.compact {
		--weight-seg-gap: 2px;
	}
	.compact .seg {
		width: 3px;
		height: 7px;
		border: none;
		background: var(--color-border-strong);
	}
	.compact .seg.on {
		background: var(--color-text-muted);
	}
</style>
