<!-- Landing page: redirects to /profile if logged in, otherwise shows entry options. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { getMe } from '$lib/api';

	let loading = $state(true);

	onMount(async () => {
		const me = await getMe();
		if (me) {
			goto('/profile');
		} else {
			loading = false;
		}
	});
</script>

<div class="landing">
	<h1>Uneasy Lies the Head</h1>
	<p class="subtitle">A play-by-post royal court drama</p>

	{#if !loading}
		<div class="card">
			<button class="primary" onclick={() => goto('/signup')}>Create an account</button>
			<button class="secondary" onclick={() => goto('/login')}>Log in</button>
		</div>
	{/if}
</div>

<style>
	.landing {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding-top: 4rem;
		gap: 1rem;
	}
	h1 { font-size: 2rem; font-weight: 700; color: #c8a96e; text-align: center; }
	.subtitle { color: #999; text-align: center; }
	.card {
		width: 100%;
		max-width: 380px;
		background: #252525;
		border: 1px solid #333;
		border-radius: 12px;
		padding: 2rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin-top: 1rem;
	}
	.primary { background: #c8a96e; color: #1a1a1a; font-weight: 600; }
	.primary:hover { background: #d9bb80; }
	.secondary { background: #333; color: #e8e4d9; }
	.secondary:hover { background: #3e3e3e; }
</style>
