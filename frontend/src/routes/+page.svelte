<!-- Combined entry screen: title + log in / sign up in one mobile-first page. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { slide } from 'svelte/transition';
	import { page } from '$app/state';
	import { getMe, login, createAccount } from '$lib/api';

	type Mode = 'login' | 'signup';

	let mode = $state<Mode>(
		page.url.searchParams.get('mode') === 'signup' ? 'signup' : 'login'
	);
	let formReady = $state(false);

	let username = $state('');
	let code = $state('');
	let email = $state('');
	let error = $state('');
	let loading = $state(false);

	const dest = () => page.url.searchParams.get('next') ?? '/profile';

	onMount(async () => {
		const me = await getMe();
		if (me) {
			goto(dest());
		} else {
			formReady = true;
		}
	});

	function setMode(next: Mode) {
		if (mode === next || loading) return;
		mode = next;
		error = '';
	}

	async function submit() {
		if (!username.trim() || !code) {
			error = 'Username and code are required.';
			return;
		}
		loading = true;
		error = '';
		try {
			if (mode === 'login') {
				await login(username.trim(), code);
			} else {
				await createAccount({
					username: username.trim(),
					code,
					email: email.trim() || null,
				});
			}
			goto(dest());
		} catch (e) {
			error = e instanceof Error ? e.message : `${mode === 'login' ? 'Login' : 'Sign up'} failed.`;
		} finally {
			loading = false;
		}
	}
</script>

<div class="screen">
	<a
		class="buy"
		href="https://adambell.itch.io/uneasy-lies-the-head-2e"
		target="_blank"
		rel="noopener noreferrer"
		aria-label="Buy the book on itch.io (opens in a new tab)"
	>Buy the book ↗</a>

	<header class="hero">
		<p class="kicker">Adam Bell's</p>
		<h1>
			<span class="line">Uneasy</span>
			<span class="line">Lies <span class="the">the</span></span>
			<span class="line">Head</span>
		</h1>
		<p class="subtitle">Competitive GMless Royal Court Roleplaying</p>
	</header>

	{#if formReady}
		<form class="card" onsubmit={(e) => { e.preventDefault(); submit(); }}>
			<div class="toggle" role="tablist" aria-label="Log in or sign up">
				<button
					type="button"
					role="tab"
					aria-selected={mode === 'login'}
					class:active={mode === 'login'}
					onclick={() => setMode('login')}
					disabled={loading}
				>Log in</button>
				<button
					type="button"
					role="tab"
					aria-selected={mode === 'signup'}
					class:active={mode === 'signup'}
					onclick={() => setMode('signup')}
					disabled={loading}
				>Sign up</button>
			</div>

			<div class="field">
				<input id="u" autocomplete="username" placeholder=" " bind:value={username} maxlength={40} disabled={loading} />
				<label for="u">Username</label>
			</div>

			<div class="field">
				<input id="c" type="password" autocomplete={mode === 'login' ? 'current-password' : 'new-password'} placeholder=" " bind:value={code} disabled={loading} />
				<label for="c">{mode === 'login' ? 'Code' : 'Code (your password)'}</label>
			</div>

			{#if mode === 'signup'}
				<div class="field" transition:slide={{ duration: 180 }}>
					<input id="e" type="email" autocomplete="email" placeholder=" " bind:value={email} disabled={loading} />
					<label for="e">Email (optional — recovery & notifications)</label>
				</div>
			{/if}

			{#if error}<p class="error">{error}</p>{/if}

			<button class="primary" type="submit" disabled={loading}>
				{#if loading}
					{mode === 'login' ? 'Logging in…' : 'Creating…'}
				{:else}
					{mode === 'login' ? 'Log in' : 'Sign up'}
				{/if}
			</button>
		</form>
	{/if}
</div>

<style>
	.screen {
		position: relative;
		min-height: 100dvh;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 1.5rem;
		padding: 1.5rem 1rem;
	}

	/* Small "buy the book" link, top-right. Generous padding gives it a ≥44px
	   tap target while the text itself stays unobtrusive. */
	.buy {
		position: absolute;
		top: max(0.5rem, env(safe-area-inset-top));
		right: max(0.5rem, env(safe-area-inset-right));
		padding: 0.6rem 0.75rem;
		color: #999;
		font-size: 0.8rem;
		text-decoration: none;
		border-radius: 6px;
	}
	.buy:hover { color: #c8a96e; }
	.buy:focus-visible { outline: 2px solid #c8a96e; outline-offset: 1px; }

	.hero {
		text-align: center;
	}
	.kicker {
		font-family: Georgia, 'Times New Roman', serif;
		text-transform: uppercase;
		letter-spacing: 0.28em;
		/* Trailing letter-spacing pushes the centered text right; nudge it back. */
		margin-right: -0.28em;
		font-size: clamp(0.8rem, 3.5vw, 1.05rem);
		font-weight: 700;
		color: #c8a96e;
	}
	h1 {
		display: flex;
		flex-direction: column;
		align-items: center;
		margin-top: 0.35rem;
		font-family: Georgia, 'Times New Roman', serif;
		font-size: clamp(2.8rem, 14vw, 4.25rem);
		font-weight: 700;
		line-height: 0.95;
		text-transform: uppercase;
		letter-spacing: 0.01em;
		color: #c8a96e;
	}
	h1 .the {
		font-size: 0.5em;
		letter-spacing: 0.12em;
	}
	.subtitle {
		margin-top: 0.75rem;
		color: #999;
		font-style: italic;
	}

	.card {
		width: 100%;
		max-width: 380px;
		background: #252525;
		border: 1px solid #333;
		border-radius: 12px;
		padding: 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.toggle {
		display: flex;
		background: #1e1e1e;
		border: 1px solid #333;
		border-radius: 8px;
		padding: 3px;
		margin-bottom: 0.25rem;
	}
	.toggle button {
		flex: 1;
		min-height: 44px;
		padding: 0.5rem;
		background: transparent;
		color: #999;
		font-weight: 600;
		border-radius: 6px;
	}
	.toggle button.active {
		background: #333;
		color: #e8e4d9;
	}
	.toggle button:disabled { cursor: not-allowed; }

	/* Floating-label fields: the label sits inside the input, then shrinks to
	   the top edge on focus or when filled — saving idle vertical space without
	   ever hiding the field's name. */
	.field {
		position: relative;
	}
	.field input {
		padding: 1.25rem 0.8rem 0.45rem;
	}
	.field label {
		position: absolute;
		left: 0.85rem;
		top: 50%;
		transform: translateY(-50%);
		color: #8c8c8c;
		font-size: 1rem;
		pointer-events: none;
		transition: top 0.15s ease, font-size 0.15s ease, color 0.15s ease, transform 0.15s ease;
		max-width: calc(100% - 1.7rem);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.field input:focus + label,
	.field input:not(:placeholder-shown) + label {
		top: 0.5rem;
		transform: none;
		font-size: 0.7rem;
		color: #c8a96e;
	}

	.error { color: #e07070; font-size: 0.9rem; }

	.primary {
		min-height: 44px;
		margin-top: 0.25rem;
		background: #c8a96e;
		color: #1a1a1a;
		font-weight: 600;
	}
	.primary:hover:not(:disabled) { background: #d9bb80; }
	.primary:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
