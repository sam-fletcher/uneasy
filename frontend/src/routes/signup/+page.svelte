<script lang="ts">
	import { goto } from '$app/navigation';
	import { createAccount } from '$lib/api';

	let username = $state('');
	let code = $state('');
	let email = $state('');
	let error = $state('');
	let loading = $state(false);

	async function submit() {
		if (!username.trim() || !code) {
			error = 'Username and code are required.';
			return;
		}
		loading = true; error = '';
		try {
			await createAccount({
				username: username.trim(),
				code,
				email: email.trim() || null,
			});
			goto('/profile');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Sign up failed.';
		} finally {
			loading = false;
		}
	}
</script>

<div class="wrap">
	<h1>Create an account</h1>
	<form class="card" onsubmit={(e) => { e.preventDefault(); submit(); }}>
		<label for="u">Username</label>
		<input id="u" bind:value={username} maxlength={40} disabled={loading} />

		<label for="c" style="margin-top:1rem">Code (your password)</label>
		<input id="c" type="password" bind:value={code} disabled={loading} />

		<label for="e" style="margin-top:1rem">Email (optional, for recovery and notifications)</label>
		<input id="e" type="email" bind:value={email} disabled={loading} />

		{#if error}<p class="error">{error}</p>{/if}

		<button class="primary" type="submit" disabled={loading} style="margin-top:1.25rem">
			{loading ? 'Creating…' : 'Sign up'}
		</button>
		<button type="button" class="secondary" onclick={() => goto('/login')} disabled={loading}>
			I already have an account
		</button>
	</form>
</div>

<style>
	.wrap { display:flex; flex-direction:column; align-items:center; padding-top:3rem; gap:1rem; }
	h1 { font-size:1.5rem; color:#c8a96e; }
	.card { width:100%; max-width:380px; background:#252525; border:1px solid #333; border-radius:12px; padding:2rem; display:flex; flex-direction:column; gap:0.4rem; }
	label { font-size:0.85rem; color:#aaa; }
	.error { color:#e07070; font-size:0.9rem; margin-top:0.5rem; }
	.primary { background:#c8a96e; color:#1a1a1a; font-weight:600; }
	.primary:hover:not(:disabled) { background:#d9bb80; }
	.secondary { background:#333; color:#e8e4d9; margin-top:0.5rem; }
	button:disabled { opacity:0.5; cursor:not-allowed; }
</style>
