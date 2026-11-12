<!--
  One labelled specimen: a faint caption above the real thing. The caption
  names the state being shown, so the page reads as a table of states.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let { label, children }: { label: string; children: Snippet } = $props();
</script>

<div class="cell">
	<span class="caption">{label}</span>
	<div class="specimen">{@render children()}</div>
</div>

<style>
	.cell {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		min-width: 0;
	}
	/* Plain faint text, NOT small capitals: capitals in a colour are the status
	   word recipe, and a caption must never be mistaken for the thing it names. */
	.caption {
		font-size: var(--text-label);
		color: var(--color-text-faint);
	}
	.specimen {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-3);
		min-width: 0;
	}
</style>
