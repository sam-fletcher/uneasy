<!--
  CrownGlyph.svelte — the line-of-succession marker (ADR-007, Phase D).

  A reigning monarch shows a filled, accent crown; a successor shows an outline
  crown with a small adjacent ordinal (1 = next in line). The number sits beside
  the crown rather than inside it: a digit inside a thumb-sized crown is illegible
  on mobile (feedback_mobile_first). Rendered only where a crown actually applies
  — callers gate on having a CrownMark, and the whole UI is hidden until the
  throne is established.
-->
<script lang="ts">
	import type { CrownMark } from '$lib/succession';

	let {
		mark,
		size = 16,
	}: {
		mark: CrownMark;
		/** Glyph edge length in px. */
		size?: number;
	} = $props();

	const isMonarch = $derived(mark.role === 'monarch');
	const label = $derived.by(() => {
		if (isMonarch) return 'Reigning monarch';
		// No ordinal (e.g. the Prologue picker, which deliberately omits the live
		// order) → a generic line-of-succession label rather than "#undefined".
		if (mark.ordinal == null) return 'In the line of succession';
		return mark.ordinal === 1
			? 'Successor — next in line'
			: `Successor — #${mark.ordinal} in line`;
	});
</script>

<span
	class="crown"
	class:monarch={isMonarch}
	class:successor={!isMonarch}
	title={label}
	aria-label={label}
	role="img"
>
	<svg
		viewBox="0 0 24 24"
		width={size}
		height={size}
		fill={isMonarch ? 'currentColor' : 'none'}
		stroke="currentColor"
		stroke-width={isMonarch ? 0 : 2}
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<!-- Chunky three-peak crown: vertical outer edges and a short, beefy base.
		     Same shape as the brand favicon — keep in sync with static/favicon.svg. -->
		<path d="M3 19 L3 8.5 L7 13.5 L12 5.5 L17 13.5 L21 8.5 L21 19 Z" />
	</svg>
	{#if !isMonarch && mark.ordinal != null}
		<span class="ordinal" aria-hidden="true">{mark.ordinal}</span>
	{/if}
</span>

<style>
	.crown {
		display: inline-flex;
		align-items: center;
		gap: 0.1rem;
		flex-shrink: 0;
		line-height: 1;
	}
	/* The reigning crown is the dominant gold cue; successors are a quieter,
	   muted-gold outline — present but visibly secondary. */
	.crown.monarch { color: var(--color-accent); }
	.crown.successor { color: var(--color-accent-dim); }
	.crown svg { display: block; }
	.ordinal {
		font-size: 0.62rem;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		color: inherit;
	}
</style>
