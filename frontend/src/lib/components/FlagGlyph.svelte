<!--
  FlagGlyph.svelte — the banner that marks the tones item.

  Extracted for the same reason as HelpGlyph: two screens now offer the same
  item (the lobby's "Adjust the tone of the game" and the prologue closing
  stage's "Tones — last chance", adr/LOBBY_AND_CHECKLIST_PLAN.md D6), and a
  checklist glyph that drifts between them would make one item read as two.
  House icon idiom, same as AssetTypeIcon/CrownGlyph/HelpGlyph: 24×24 viewBox,
  stroke=currentColor, stroke-width 2, round caps/joins. Colour comes from the
  caller.
-->
<script lang="ts">
	let { size = 16 }: { /** Glyph edge length, any CSS length (number = px). */ size?: number | string } = $props();
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
	<path d="M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1z" />
	<line x1="4" y1="22" x2="4" y2="15" />
</svg>

<style>
	/* The component owns its box: callers are flex containers, and a bare svg
	   there shrinks below its width attribute (HelpGlyph's note). */
	svg {
		display: block;
		flex-shrink: 0;
	}
</style>
