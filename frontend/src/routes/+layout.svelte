<script lang="ts">
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { getMe, type Account } from '$lib/api';

	let { children }: { children: Snippet } = $props();

	let me = $state<Account | null>(null);
	let loaded = $state(false);

	const HIDDEN_PATHS = ['/login', '/signup', '/'];
	let showHeader = $derived(
		loaded && me !== null
		&& !HIDDEN_PATHS.includes(page.url.pathname)
		&& !page.url.pathname.startsWith('/table/')
	);

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
		<a class="home" href="/profile" aria-label="Home">
			<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
				<path d="M3 11l9-8 9 8" />
				<path d="M5 10v10h14V10" />
			</svg>
		</a>
	</header>
{/if}

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

	.site-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.6rem 1rem;
		background: #202020;
		border-bottom: 1px solid #2e2e2e;
	}
	.home {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 44px;
		height: 44px;
		margin: -0.35rem 0;
		color: #c8a96e;
		border-radius: 6px;
		text-decoration: none;
	}
	.home:hover { color: #d9bb80; background: #2a2a2a; }
	.home:focus-visible { outline: 2px solid #c8a96e; outline-offset: 1px; }

	main {
		max-width: 1500px;
		margin: 0 auto;
		padding: 1rem;
	}
</style>
