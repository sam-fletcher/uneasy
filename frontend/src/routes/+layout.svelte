<script lang="ts">
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getMe, type Account } from '$lib/api';
	import HelpButton from '$lib/components/HelpButton.svelte';
	import { registerServiceWorker } from '$lib/push';
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
		// Register eagerly (not gated on notification opt-in) so the service
		// worker is already active by the time a player taps "enable" in
		// Profile or the lobby soft-ask — registration itself never prompts.
		registerServiceWorker();
	});
</script>

<svelte:head>
	<title>Uneasy Lies the Head</title>
</svelte:head>

{#if showHeader && me}
	<header class="site-header">
		<h1 class="page-title">{pageTitle}</h1>
		<div class="header-actions">
			<a
				class="top-link"
				href="https://adambell.itch.io/uneasy-lies-the-head-2e"
				target="_blank"
				rel="noopener noreferrer"
				aria-label="Buy the book on itch.io (opens in a new tab)"
			>The Book
				<svg class="external-icon" viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M7 17L17 7" /><path d="M8 7h9v9" /></svg>
			</a>
			<a
				class="top-link"
				href="https://github.com/sam-fletcher/uneasy/"
				target="_blank"
				rel="noopener noreferrer"
				aria-label="View source on GitHub (opens in a new tab)"
			>GitHub
				<svg class="external-icon" viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M7 17L17 7" /><path d="M8 7h9v9" /></svg>
			</a>
			<HelpButton />
		</div>
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
		font-family: var(--font-serif);
		/* Never algorithmically fake a weight we don't have a real cut for —
		   missing weights fall back to the nearest real face cleanly. */
		font-synthesis: none;
		background: var(--color-bg);
		color: var(--color-text);
		min-height: 100dvh;
	}

	/* The whole UI is Spectral (set on body above). Headings default to its
	   600 weight; the hero title opts into --font-display. */
	:global(h1, h2, h3) {
		font-family: var(--font-serif);
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
		border-bottom: 1px solid var(--color-border);
	}
	/* Matches the .phase-badge treatment on table pages ("LOBBY", "MAIN
	   EVENT") so top-level page titles read as the same kind of label. */
	.page-title {
		margin: 0;
		font-size: 0.8rem;
		background: var(--color-border-warm);
		color: var(--color-accent);
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	.header-actions {
		display: flex;
		align-items: center;
		gap: 0.25rem;
	}
	.top-link {
		display: inline-flex;
		align-items: center;
		gap: 0.3em;
		min-height: 44px;
		padding: 0 0.5rem;
		color: var(--color-text-muted);
		font-size: 0.9rem;
		text-decoration: none;
		border-radius: var(--radius-sm);
	}
	.top-link:hover { color: var(--color-accent); }
	.top-link:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
	.external-icon { flex-shrink: 0; }

	main {
		max-width: 1500px;
		margin: 0 auto;
		padding: 1rem;
	}
	/* Table route: immersive game UI fills the viewport edge-to-edge.
	   overflow-x: clip guards against the table page's edge-to-edge strips
	   (see .top-strip in table/[id]/+page.svelte) landing a fraction of a
	   pixel past this box at fractional viewport widths — without it,
	   document.documentElement.scrollWidth can exceed clientWidth by ~1px,
	   letting the whole page rubber-band sideways on touch scroll. */
	main.full-bleed {
		max-width: 100%;
		padding: 0 0.2rem;
		overflow-x: clip;
	}
</style>
