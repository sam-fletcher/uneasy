<script lang="ts">
	import type { Snippet } from 'svelte';

	let { children }: { children: Snippet } = $props();
</script>

<svelte:head>
	<title>Uneasy Lies the Head</title>
</svelte:head>

<main>
	{@render children()}
</main>

<style>
	:global(*, *::before, *::after) {
		box-sizing: border-box;
		margin: 0;
		padding: 0;
	}

	:global(body) {
		font-family: system-ui, -apple-system, sans-serif;
		background: #1a1a1a;
		color: #e8e4d9;
		min-height: 100dvh;
	}

	:global(button) {
		cursor: pointer;
		font-size: 1rem;
		padding: 0.6rem 1.2rem;
		border-radius: 6px;
		border: none;
		font-family: inherit;
	}

	:global(input) {
		font-size: 1rem;
		padding: 0.6rem 0.8rem;
		border-radius: 6px;
		border: 1px solid #444;
		background: #2a2a2a;
		color: inherit;
		font-family: inherit;
		width: 100%;
	}

	:global(input:focus) {
		outline: 2px solid #c8a96e;
		outline-offset: 1px;
	}

	main {
		max-width: 900px;
		margin: 0 auto;
		padding: 1rem;
	}
</style>
