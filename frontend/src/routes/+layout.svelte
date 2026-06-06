<script lang="ts">
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getMe, type Account } from '$lib/api';
	import '../app.css';

	let { children }: { children: Snippet } = $props();

	let me = $state<Account | null>(null);
	let loaded = $state(false);

	const HIDDEN_PATHS = ['/login', '/signup', '/'];
	let showHeader = $derived(
		loaded && me !== null
		&& !HIDDEN_PATHS.includes(page.url.pathname)
		&& !page.url.pathname.startsWith('/table/')
	);
	// The shared top bar currently renders only on /profile (the one logged-in
	// route that isn't an auth page or a full-bleed table). It shows the page
	// title on the left; extend this map as more top-level pages appear.
	const PAGE_TITLES: Record<string, string> = { '/profile': 'Profile' };
	const pageTitle = $derived(PAGE_TITLES[page.url.pathname] ?? '');
	const isTableRoute = $derived(page.url.pathname.startsWith('/table/'));

	onMount(async () => {
		try { me = await getMe(); } catch { /* ignore */ }
		loaded = true;
	});
</script>

<svelte:head>
	<title>Uneasy Lies the Head</title>
</svelte:head>

{#if showHeader && me}
	<header class="site-header">
		<h1 class="page-title">{pageTitle}</h1>
		<a
			class="buy"
			href="https://adambell.itch.io/uneasy-lies-the-head-2e"
			target="_blank"
			rel="noopener noreferrer"
			aria-label="Buy the book on itch.io (opens in a new tab)"
		>Buy the book ↗</a>
	</header>
{/if}

<main class:full-bleed={isTableRoute}>
	{@render children()}
</main>

<style>
	:global(*, *::before, *::after) {
		box-sizing: border-box;
		margin: 0;
		padding: 0;
	}

	:global(body) {
		font-family: var(--font-sans);
		background: var(--color-bg);
		color: var(--color-text);
		min-height: 100dvh;
	}

	/* Headings default to the text serif (Spectral) at its loaded 600 weight;
	   body/UI stays on the sans. The hero title opts into --font-display. */
	:global(h1, h2, h3) {
		font-family: var(--font-serif);
		font-weight: 600;
	}

	:global(button) {
		cursor: pointer;
		font-size: 1rem;
		padding: 0.6rem 1.2rem;
		border-radius: var(--radius-sm);
		border: none;
		font-family: inherit;
	}

	:global(input) {
		font-size: 1rem;
		padding: 0.6rem 0.8rem;
		border-radius: var(--radius-sm);
		border: 1px solid var(--color-border);
		background: var(--color-surface-2);
		color: inherit;
		font-family: inherit;
		width: 100%;
	}

	:global(input:focus) {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}

	.site-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.6rem 1rem;
		background: var(--color-surface-sunken);
		border-bottom: 1px solid var(--color-border-subtle);
	}
	.page-title {
		margin: 0;
		font-size: 1.7rem;
		color: var(--color-accent);
	}
	.buy {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		padding: 0 0.5rem;
		color: var(--color-text-muted);
		font-size: 0.85rem;
		text-decoration: none;
		border-radius: var(--radius-sm);
	}
	.buy:hover { color: var(--color-accent); }
	.buy:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	main {
		max-width: 1500px;
		margin: 0 auto;
		padding: 1rem;
	}
	/* Table route: immersive game UI fills the viewport edge-to-edge. */
	main.full-bleed {
		max-width: 100%;
		padding: 0 0.2rem;
	}
</style>
