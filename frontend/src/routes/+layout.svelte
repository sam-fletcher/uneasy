<script lang="ts">
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getMe, type Account } from '$lib/api';
	import { watchForNewVersion } from '$lib/appVersion';
	import HelpButton from '$lib/components/HelpButton.svelte';
	import UpdateBanner from '$lib/components/UpdateBanner.svelte';
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
	// Routes whose own content column supplies the horizontal gutter, so main
	// must not add one on top (see main.flush). Only pages that participate in
	// the layout width system belong here — the auth pages are plain centered
	// cards with no cap to hit, and keep main's default gutter.
	const FLUSH_PATHS = ['/profile'];
	const isFlushRoute = $derived(FLUSH_PATHS.includes(page.url.pathname));

	onMount(async () => {
		try { me = await getMe(); } catch { /* ignore */ }
		loaded = true;
		// Register eagerly (not gated on notification opt-in) so the service
		// worker is already active by the time a player taps "enable" in
		// Profile or the lobby soft-ask — registration itself never prompts.
		registerServiceWorker();
	});

	// Separate from the async onMount above: that one can't return a teardown.
	// Lives in the layout so every route gets the notice — a tab goes stale
	// wherever it happens to be sitting, and the table route (which hides the
	// shared header) is the one most likely to be left open for hours.
	onMount(() => watchForNewVersion());
</script>

<svelte:head>
	<title>Uneasy Lies the Head</title>
</svelte:head>

<UpdateBanner />

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

<main class:full-bleed={isTableRoute} class:flush={isFlushRoute}>
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

	/* Table route only: make the body a viewport-height flex column so the
	   update banner (auto height) and the game shell (fills the rest) split
	   the viewport exactly. Without this the shell's own 100dvh would sit
	   *below* the banner and push the bottom-pinned chat strip off-screen.
	   Doing it in CSS rather than by measuring the bar in JS keeps the two in
	   sync through every reflow — the copy re-wraps at different widths, and a
	   measured value is always a frame behind. Other routes scroll normally
	   and just take the banner as an ordinary block above them. */
	:global(body:has(main.full-bleed)) {
		display: flex;
		flex-direction: column;
		height: 100dvh;
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
	/* Same shape as the shared PhaseBadge ("LOBBY", "MAIN EVENT") so
	   top-level page titles read as the same kind of label — but warm gold,
	   not the badge's violet: violet marks procedural *game* info (ADR-009),
	   while this is a site-level page name. */
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
	/* Pages whose own column supplies the gutter (FLUSH_PATHS above). Same
	   principle as .full-bleed: a column in the width system has to derive
	   from the raw viewport, or its cap is never the width it claims. With
	   main's gutter outside the capped box, /profile measured 408 at a 440
	   viewport while every other content column measured 440 — and its 888
	   two-up cap needed a 920 viewport to bind. The page keeps the vertical
	   padding and pads itself horizontally instead. */
	main.flush {
		padding-inline: 0;
	}
	/* Table route: immersive game UI fills the viewport edge-to-edge.
	   Horizontal padding here must stay ZERO — the layout width system
	   (docs/STYLE_GUIDE.md "Layout widths") derives every column from the
	   raw viewport, so any gutter on this box is subtracted from the phase
	   column: a 0.2rem one used to leave the record-phase content 293.6px
	   on a 360 phone instead of the 300px floor components rely on, and put
	   both dock thresholds (790, 1070) ~4px short of the track widths they
	   were sized for.
	   overflow-x: clip guards against the table page's edge-to-edge strips
	   (see .top-strip in table/[id]/+page.svelte) landing a fraction of a
	   pixel past this box at fractional viewport widths — without it,
	   document.documentElement.scrollWidth can exceed clientWidth by ~1px,
	   letting the whole page rubber-band sideways on touch scroll. */
	main.full-bleed {
		max-width: 100%;
		padding: 0;
		overflow-x: clip;
		/* Undo the base rule's `margin: 0 auto`. That centres a block-level
		   main under the 1500px cap, but on the table route body is a flex
		   column (see body:has above) — and auto margins on the cross axis
		   suppress a flex item's default `stretch`, leaving main at its
		   shrink-to-fit width and centred with dead space either side. On a
		   412px phone the lobby rendered ~302px wide because of this. */
		margin-inline: 0;
		/* Claim the flex column's leftover height (see body:has above) and let
		   it shrink below content — the game shell scrolls internally. */
		flex: 1 1 auto;
		min-height: 0;
	}
</style>
