<!--
  One titled block of the styleguide. A heading and space, no box: the page
  follows the rules it documents.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		id,
		title,
		blurb,
		children,
	}: { id: string; title: string; blurb?: string; children: Snippet } = $props();
</script>

<section {id} class="section" aria-labelledby="{id}-title">
	<h2 id="{id}-title">{title}</h2>
	{#if blurb}<p class="blurb">{blurb}</p>{/if}
	<div class="body">{@render children()}</div>
</section>

<style>
	.section {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		padding-top: var(--space-5);
		border-top: 1px solid var(--color-border);
		scroll-margin-top: var(--space-4);
	}
	h2 {
		font-size: var(--text-heading);
		color: var(--color-accent);
		margin: 0;
	}
	.blurb {
		margin: 0;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
		line-height: 1.5;
		max-width: 60ch;
	}
	.body {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		min-width: 0;
	}
</style>
