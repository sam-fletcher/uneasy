<!-- Combined entry screen: title + log in / sign up in one mobile-first page. -->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { slide } from 'svelte/transition';
	import { page } from '$app/state';
	import { getMe, login, createAccount } from '$lib/api';
	import HelpButton from '$lib/components/HelpButton.svelte';
	import { TEXT_LIMITS } from '$lib/textLimits';

	type Mode = 'login' | 'signup';

	let mode = $state<Mode>(
		page.url.searchParams.get('mode') === 'signup' ? 'signup' : 'login'
	);
	let formReady = $state(false);

	let username = $state('');
	let password = $state('');
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
		if (!username.trim() || !password) {
			error = 'Player name and password are required.';
			return;
		}
		loading = true;
		error = '';
		try {
			if (mode === 'login') {
				await login(username.trim(), password);
			} else {
				await createAccount({
					username: username.trim(),
					password,
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
	<div class="top-actions">
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
				<input id="u" autocomplete="username" placeholder=" " bind:value={username} maxlength={TEXT_LIMITS.USERNAME} disabled={loading} />
				<label for="u">Player name</label>
			</div>

			<div class="field">
				<input id="c" type="password" autocomplete={mode === 'login' ? 'current-password' : 'new-password'} placeholder=" " bind:value={password} disabled={loading} />
				<label for="c">Password</label>
			</div>

			<!-- {#if mode === 'signup'}
				<div class="field" transition:slide={{ duration: 180 }}>
					<input id="e" type="email" autocomplete="email" placeholder=" " bind:value={email} disabled={loading} />
					<label for="e">Email (optional: recovery & notifications)</label>
				</div>
			{/if} -->

			{#if error}<p class="error-text">{error}</p>{/if}

			<button class="action-btn primary" type="submit" disabled={loading}>
				{#if loading}
					{mode === 'login' ? 'Logging in…' : 'Creating…'}
				{:else}
					{mode === 'login' ? 'Log in' : 'Sign up'}
				{/if}
			</button>

			{#if mode === 'login'}
				<p class="forgot-password">
					Forgot your password? <a href="/locked-out">Request a reset</a>.
				</p>
			{/if}
		</form>
	{/if}

	<p class="credits">
		Digital adaption by
		<a href="https://sam-fletcher.itch.io/" target="_blank" rel="noopener noreferrer">Sam Fletcher</a>
	</p>
</div>

<style>
	.screen {
		position: relative;
		/* Cancels the shared <main> wrapper's 1rem top/bottom padding so this
		   min-height:100dvh screen doesn't end up 2rem taller than the
		   viewport (and scrollable) on top of it. */
		margin: -1rem 0;
		min-height: 100dvh;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 1.5rem;
		padding: 1.5rem 1rem;
	}

	/* "Buy the book" + Help, top-right. Generous padding on .buy gives it a
	   ≥44px tap target while the text itself stays unobtrusive. */
	.top-actions {
		position: absolute;
		top: max(0.5rem, env(safe-area-inset-top));
		right: max(0.5rem, env(safe-area-inset-right));
		display: flex;
		align-items: center;
		gap: 0.15rem;
	}
	.top-link {
		display: inline-flex;
		align-items: center;
		gap: 0.3em;
		min-height: 44px;
		padding: 0 0.75rem;
		color: var(--color-text-muted);
		font-size: 0.9rem;
		text-decoration: none;
		border-radius: 6px;
	}
	.top-link:hover { color: var(--color-accent); }
	.top-link:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
	.external-icon { flex-shrink: 0; }

	.hero {
		text-align: center;
	}
	.kicker {
		font-family: var(--font-display);
		text-transform: uppercase;
		letter-spacing: 0.28em;
		/* Trailing letter-spacing pushes the centered text right; nudge it back. */
		margin-right: -0.28em;
		font-size: clamp(0.8rem, 3.5vw, 1.05rem);
		color: var(--color-accent);
	}
	h1 {
		display: flex;
		flex-direction: column;
		align-items: center;
		margin-top: 0.35rem;
		font-family: var(--font-display);
		font-size: clamp(2.8rem, 14vw, 4.25rem);
		line-height: 0.95;
		text-transform: uppercase;
		letter-spacing: 0.01em;
		color: var(--color-accent);
	}
	h1 .the {
		font-size: 0.5em;
		letter-spacing: 0.12em;
	}
	.subtitle {
		margin-top: 0.75rem;
		font-family: var(--font-serif);
		font-size: 1.1rem;
		color: var(--color-text-muted);
		/* font-style: italic; */
	}

	.card {
		width: 100%;
		max-width: 380px;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: 12px;
		padding: 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.toggle {
		display: flex;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		padding: 3px;
		margin-bottom: 0.25rem;
	}
	.toggle button {
		flex: 1;
		min-height: 44px;
		padding: 0.5rem;
		background: transparent;
		color: var(--color-text-muted);
		border-radius: 6px;
	}
	.toggle button.active {
		background: var(--color-border);
		color: var(--color-text);
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
		color: var(--color-text-faint);
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
		color: var(--color-accent);
	}

	.primary { margin-top: 0.25rem; align-self: center; }

	.forgot-password {
		text-align: center;
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}
	.forgot-password a { color: var(--color-accent); }

	.credits {
		position: absolute;
		bottom: max(1.5rem, env(safe-area-inset-bottom));
		left: 50%;
		transform: translateX(-50%);
		font-size: 0.75rem;
		color: var(--color-text-muted);
		text-align: center;
		white-space: nowrap;
	}
	.credits a {
		color: var(--color-accent);
		text-decoration: none;
	}
	.credits a:hover { color: var(--color-accent-hover); }
	.credits a:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; border-radius: 2px; }
</style>
