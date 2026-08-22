<!--
  BellGlyph.svelte — the reminder bell on the profile page's table cards, and
  its struck twin for a table that is sending nothing.

  House icon idiom, same as HelpGlyph/CrownGlyph: 24×24 viewBox, stroke=
  currentColor, stroke-width 2, round caps/joins, colour from the caller.

  The `struck` variant draws the same bell plus a diagonal, rather than a
  different shape: the two states are the same object answering "will this
  table ping me?", and a reader who learns one recognises the other. The
  diagonal is drawn with a background-coloured under-stroke so it reads as
  crossing *over* the bell at any size, instead of merging with the outline
  where the two meet.
-->
<script lang="ts">
	let {
		size = 24,
		struck = false,
	}: {
		/** Glyph edge length, any CSS length (number = px). */
		size?: number | string;
		/** Draw the "no reminders" diagonal across the bell. */
		struck?: boolean;
	} = $props();
</script>

<svg
	viewBox="0 0 24 24"
	width={size}
	height={size}
	fill="none"
	stroke="currentColor"
	stroke-width="2"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
>
	<path d="M18 8a6 6 0 0 0-12 0c0 6-3 7-3 7h18s-3-1-3-7" />
	<path d="M13.7 19a2 2 0 0 1-3.4 0" />
	{#if struck}
		<line class="strike-under" x1="4" y1="4" x2="20" y2="20" />
		<line x1="4" y1="4" x2="20" y2="20" />
	{/if}
</svg>

<style>
	/* The component owns its box: callers are flex/inline-flex containers, and
	   a bare svg there shrinks below its width attribute (scoped CSS in the
	   caller can't reach across the component boundary to stop it). */
	svg {
		display: block;
		flex-shrink: 0;
	}
	/* Widened and painted in the caller's own background so the diagonal carves
	   a gap through the bell's outline rather than blending into it. The caller
	   sets --bell-strike-bg to whatever it actually sits on — on the profile
	   card that is the warm your-move fill, not the plain surface, and getting
	   it wrong leaves a visible seam. */
	.strike-under {
		stroke: var(--bell-strike-bg, var(--color-surface));
		stroke-width: 5;
	}
</style>
