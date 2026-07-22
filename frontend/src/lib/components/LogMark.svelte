<!--
	LogMark.svelte — the family marks for the chat log (adr/LOG_MARKS_PLAN.md).

	One mark per system-post family, replacing the Unicode `FAMILY_GLYPHS` map.
	Of that old set only `§` existed in Spectral, so `⚑ ⚄ ✎ ❧` each resolved to
	whatever system font happened to carry them — a different font per glyph,
	per platform, which is where the metric mismatches the old CSS fudges
	fought actually came from. These ship with the app.

	Line icons in the house style, same as AssetTypeIcon/CrownGlyph: 24×24
	viewBox, stroke=currentColor, stroke-width 2, round caps/joins. Die pips are
	the one exception (filled, unstroked). The mark inherits colour from its
	slot and fills whatever box the caller sizes — see `.log-mark` in
	ChatPanel.svelte, which owns the 16px.

	The set is closed and owner-approved; the plan's "Constraints these marks
	encode" section records what each shape is deliberately avoiding (the tear's
	stagger, the die's two pips, the podium-not-scales trade). Don't tidy them
	without reading it. An unknown family renders nothing rather than a bullet:
	every family is supposed to have a mark, so a blank is the louder bug.
-->
<script lang="ts">
	let { family }: { family: string | null } = $props();
</script>

{#if family}
	<svg
		class="mark"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		{#if family === 'plan'}
			<!-- flag -->
			<path d="M6 3v18" />
			<path d="M6 4.5h11l-2.5 4L17 12.5H6" />
		{:else if family === 'demand'}
			<!-- manicule: the index runs to the edge of the box, the remaining
			     fingers step down beneath it -->
			<path d="M2.5 9h3.2v6.6H2.5z" />
			<path d="M5.7 10.3c0-.7.6-1.3 1.3-1.3h1.3c.7 0 1.3.6 1.3 1.3v.5h9.3a1.3 1.3 0 0 1 0 2.6h-6.2a1.1 1.1 0 0 1 0 2.2h-1.6a1.1 1.1 0 0 1 0 2.2H7c-.7 0-1.3-.6-1.3-1.3z" />
		{:else if family === 'asset'}
			<!-- crest -->
			<path d="M12 3l8 3v6.5c0 5-4.6 7.8-8 9.2-3.4-1.4-8-4.2-8-9.2V6z" />
		{:else if family === 'marginalia'}
			<!-- pencil -->
			<path d="M4 20h4l10-10a2.83 2.83 0 0 0-4-4L4 16z" />
			<path d="M13.5 6.5l4 4" />
		{:else if family === 'tear'}
			<!-- torn sheet — the hostile counterpart to the pencil. Staggered on
			     purpose: squared up, its silhouette is the die's. -->
			<path d="M10 2.5H4.5v15H10" />
			<path d="M14 6.5h5.5v15H14" />
			<path d="M10 2.5l-1.4 3 1.4 3-1.4 3 1.4 3-1.4 3" />
			<path d="M14 6.5l-1.4 3 1.4 3-1.4 3 1.4 3-1.4 3" />
		{:else if family === 'seize'}
			<!-- crossed swords — the hostile counterpart to the crest -->
			<path d="M4 4l11.5 11.5" />
			<path d="M20 4L8.5 15.5" />
			<path d="M5.5 17.5l3-3 1.5 1.5-3 3z" />
			<path d="M18.5 17.5l-3-3-1.5 1.5 3 3z" />
		{:else if family === 'roll'}
			<!-- die: two r-2 pips stay crisp at 16px where three r-1.5 pips
			     soften into the face. The count is legibility, not a value. -->
			<rect x="3.5" y="3.5" width="17" height="17" rx="3.5" />
			<circle cx="9" cy="9" r="2" fill="currentColor" stroke="none" />
			<circle cx="15" cy="15" r="2" fill="currentColor" stroke="none" />
		{:else if family === 'law'}
			<!-- scales: freed for law by moving rankings to the podium -->
			<path d="M12 4v16" />
			<path d="M5 8h14" />
			<path d="M5 8l-3 6h6z" />
			<path d="M19 8l-3 6h6z" />
			<path d="M8 20.5h8" />
		{:else if family === 'scene'}
			<!-- quotation marks -->
			<path d="M4.5 17a4 4 0 0 0 4-4V7h-5v6h4" />
			<path d="M15 17a4 4 0 0 0 4-4V7h-5v6h4" />
		{:else if family === 'ranking'}
			<!-- podium, not scales -->
			<path d="M2.5 20.5h19" />
			<path d="M9 20.5V6h6v14.5" />
			<path d="M3 20.5v-8h6" />
			<path d="M21 20.5v-11h-6" />
		{:else if family === 'shake_up'}
			<!-- burst -->
			<path d="M12 3v18" />
			<path d="M4.2 7.5l15.6 9" />
			<path d="M19.8 7.5l-15.6 9" />
		{:else if family === 'prologue'}
			<!-- sun. Radial like the Shake-Up's burst, which is accepted: they
			     bookend the game and never appear near each other in the feed. -->
			<path d="M2.5 19.5h19" />
			<path d="M6 19.5a6 6 0 0 1 12 0" />
			<path d="M12 3.5v3.5" />
			<path d="M4 7l2.5 2.5" />
			<path d="M20 7l-2.5 2.5" />
		{:else if family === 'rumor'}
			<!-- speech bubble -->
			<path d="M4 5.5h16v10h-9l-5.5 4v-4H4z" />
		{:else if family === 'secret'}
			<!-- eye, echoing the open/struck-eye counters on asset cards -->
			<path d="M2.5 12s3.6-6.5 9.5-6.5S21.5 12 21.5 12s-3.6 6.5-9.5 6.5S2.5 12 2.5 12z" />
			<circle cx="12" cy="12" r="2.6" />
		{/if}
	</svg>
{/if}

<style>
	/* The caller owns the size; block display keeps the box exactly that tall
	   (an inline svg would add the parent's line-box leading on top). */
	.mark {
		display: block;
		width: 100%;
		height: 100%;
	}
</style>
