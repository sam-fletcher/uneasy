<!--
	SuitGlyph.svelte — the four playing-card suits as drawn paths rather than
	the Unicode pips, so their weight and proportions survive the sizes we
	actually use: ~11.5px bare in the Prologue sheet-header track tags (the
	smallest, and the only place a suit stands alone with no value beside it),
	~12px inside a card chip, 18px in the choosing legend.

	Redrawn 2026-08-01. The first set came from generic UI icon shapes and lost
	its distinctions as it shrank — the club's lobes overlapped so far they
	fused into one blob, the spade's stem was a stub swallowed by its body, and
	the diamond was a square stood on its point. Below ~12px that left spade
	and club as the same dark lump, and they are exactly the pair a reader
	needs to separate: colour already splits red from black, and heart from
	diamond is never in doubt, so spade-vs-club is the whole problem.

	The two blacks are therefore drawn to diverge at the silhouette, which is
	all that survives at 10px — the spade converges to a point at the top, the
	club spreads into three lobes over notches cut deep enough to hold. Both
	stand on the same wide flat foot, which is what separates them from the
	heart's point below. The diamond is card-proportioned (narrower than it is
	tall) rather than square, so it carries less ink than the heart at every
	size — that is how a real card reads too, so don't "fix" it by fattening it
	back towards a rhombus.

	Colour is `currentColor`, always: the same suit takes different ink on
	different grounds (a black suit is --color-bg on a parchment .card-glyph
	face but --color-text on the dark page), so the ground owns the colour and
	this only owns the shape. Callers set it on the wrapper, conventionally via
	`[data-color='red'|'black']`.

	Size defaults to 1em of the caller's font-size, which is the right pip for
	a chip with a card value beside it. Override per slot with
	`.wrapper :global(.suit) { width: …; height: …; }`.
-->
<script lang="ts">
	/** Card suit letter, as the API spells it: H, D, S or C. An unrecognised
	 *  suit renders nothing — a blank is a louder bug than a wrong pip. */
	let { suit }: { suit: string } = $props();
</script>

{#if suit === 'H'}
	<svg class="suit" viewBox="0 0 24 24" aria-hidden="true">
		<path fill="currentColor" d="M12 22.1C9.5 19.7 2.1 14.4 2.1 8.8C2.1 5.3 4.8 2.9 7.8 2.9C10.1 2.9 11.4 4.5 12 6.1C12.6 4.5 13.9 2.9 16.2 2.9C19.2 2.9 21.9 5.3 21.9 8.8C21.9 14.4 14.5 19.7 12 22.1Z"/>
	</svg>
{:else if suit === 'D'}
	<svg class="suit" viewBox="0 0 24 24" aria-hidden="true">
		<path fill="currentColor" d="M12 1.7 L20.2 12 L12 22.3 L3.8 12 Z"/>
	</svg>
{:else if suit === 'S'}
	<svg class="suit" viewBox="0 0 24 24" aria-hidden="true">
		<path fill="currentColor" d="M12 1.7C12.9 4.2 21.8 10.1 21.8 15.3C21.8 18.4 19.6 20.4 17 20.4C15.2 20.4 13.7 19.6 12.85 18.4C12.9 20.4 13.9 21.7 16.3 22.4L7.7 22.4C10.1 21.7 11.1 20.4 11.15 18.4C10.3 19.6 8.8 20.4 7 20.4C4.4 20.4 2.2 18.4 2.2 15.3C2.2 10.1 11.1 4.2 12 1.7Z"/>
	</svg>
{:else if suit === 'C'}
	<svg class="suit" viewBox="0 0 24 24" aria-hidden="true">
		<circle cx="12" cy="7.2" r="5.2" fill="currentColor"/>
		<circle cx="7.1" cy="15.1" r="5.2" fill="currentColor"/>
		<circle cx="16.9" cy="15.1" r="5.2" fill="currentColor"/>
		<path fill="currentColor" d="M10.8 15.7C11.25 19.2 10.4 21.4 7.9 22.4L16.1 22.4C13.6 21.4 12.75 19.2 13.2 15.7Z"/>
	</svg>
{/if}

<style>
	/* inline-block + vertical-align keeps the pip on the text baseline when a
	   caller drops it straight into prose; flex:none stops it being squeezed
	   inside .card-glyph, which is an inline-flex row. */
	.suit {
		width: 1em;
		height: 1em;
		flex: none;
		display: inline-block;
		vertical-align: middle;
	}
</style>
