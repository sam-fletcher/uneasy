<script lang="ts">
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getMe, logout, type Account } from '$lib/api';

	let { children }: { children: Snippet } = $props();

	let me = $state<Account | null>(null);
	let loaded = $state(false);

	const HIDDEN_PATHS = ['/login', '/signup', '/'];
	let showHeader = $derived(loaded && me !== null && !HIDDEN_PATHS.includes(page.url.pathname));

	onMount(async () => {
		try { me = await getMe(); } catch { /* ignore */ }
		loaded = true;
	});

	async function doLogout() {
		await logout();
		me = null;
		goto('/');
	}
</script>

<svelte:head>
	<title>Uneasy Lies the Head</title>
</svelte:head>

{#if showHeader && me}
	<header class="site-header">
		<a class="brand" href="/profile">Uneasy Lies the Head</a>
		<div class="right">
			<a class="user" href="/profile">{me.username}</a>
			<button class="logout" onclick={doLogout}>Log out</button>
		</div>
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
	.brand {
		color: #c8a96e;
		text-decoration: none;
		font-weight: 600;
	}
	.brand:hover { color: #d9bb80; }
	.right { display: flex; align-items: center; gap: 0.75rem; }
	.user { color: #aaa; text-decoration: none; font-size: 0.9rem; }
	.user:hover { color: #e8e4d9; }
	.logout {
		background: #333;
		color: #e8e4d9;
		font-size: 0.85rem;
		padding: 0.35rem 0.8rem;
	}
	.logout:hover { background: #3e3e3e; }

	main {
		max-width: 900px;
		margin: 0 auto;
		padding: 1rem;
	}
</style>
