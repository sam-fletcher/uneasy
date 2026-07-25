<!--
  HelpGlyph.svelte — the circled "?" that marks the help affordance.

  Extracted so the header button (HelpButton) and any prose pointing at it
  ("reopen this from the ? in the top-right corner") draw the same shape: a
  bare typed "?" reads as punctuation, not as the control. House icon idiom,
  same as AssetTypeIcon/CrownGlyph: 24×24 viewBox, stroke=currentColor,
  stroke-width 2, round caps/joins. Colour comes from the caller.
-->
<script lang="ts">
	let { size = 24 }: { /** Glyph edge length, any CSS length (number = px). */ size?: number | string } = $props();
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
	<circle cx="12" cy="12" r="10" />
	<path d="M9.5 9a2.5 2.5 0 0 1 4.5 1.5c0 1.5-2 2-2 3" />
	<line x1="12" y1="17" x2="12" y2="17" />
</svg>

<style>
	/* The component owns its box: callers are flex/inline-flex containers, and
	   a bare svg there shrinks below its width attribute (scoped CSS in the
	   caller can't reach across the component boundary to stop it). */
	svg {
		display: block;
		flex-shrink: 0;
	}
</style>
