<!-- Combined entry screen: title + log in / sign up in one mobile-first page. -->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/chip.css';
	import '$lib/components/shared/statusText.css';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { slide } from 'svelte/transition';
	import { page } from '$app/state';
	import { getMe, login, createAccount } from '$lib/api';
	import HelpButton from '$lib/components/HelpButton.svelte';
	import CharCounter from '$lib/components/CharCounter.svelte';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

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
		<form class="entry" onsubmit={(e) => { e.preventDefault(); submit(); }}>
			<div class="toggle" role="tablist" aria-label="Log in or sign up">
				<button
					type="button"
					class="chip"
					class:selected={mode === 'login'}
					role="tab"
					aria-selected={mode === 'login'}
					onclick={() => setMode('login')}
					disabled={loading}
				>Log in</button>
				<button
					type="button"
					class="chip"
					class:selected={mode === 'signup'}
					role="tab"
					aria-selected={mode === 'signup'}
					onclick={() => setMode('signup')}
					disabled={loading}
				>Sign up</button>
			</div>

			<div class="field">
				<input id="u" autocomplete="username" placeholder=" " bind:value={username} maxlength={TEXT_LIMITS.USERNAME} disabled={loading} />
				<label for="u">Player name</label>
				<!-- Sign-up only: on log in the name is already chosen, and a
				     counter on a remembered credential is just noise. -->
				{#if mode === 'signup'}
					<span class="field-counter"><CharCounter value={username} max={TEXT_LIMITS.USERNAME} /></span>
				{/if}
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

			{#if error}<ErrorText message={error} />{/if}

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
		/* Cancels the shared <main> wrapper's top and bottom padding, so this
		   min-height:100dvh screen doesn't end up a gutter taller than the
		   viewport (and scrollable) on top of it. */
		margin: calc(-1 * var(--space-4)) 0;
		min-height: 100dvh;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: var(--space-5);
		padding: var(--space-5) var(--space-4);
	}

	/* "The Book" and GitHub, with Help, top-right. The links keep the touch
	   floor, so each is a 44px target while the text stays unobtrusive. */
	.top-actions {
		position: absolute;
		top: max(var(--space-2), env(safe-area-inset-top));
		right: max(var(--space-2), env(safe-area-inset-right));
		display: flex;
		align-items: center;
		gap: var(--space-1);
	}
	.top-link {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-height: var(--size-tap);
		padding: 0 var(--space-3);
		color: var(--color-text-secondary);
		font-size: var(--text-small);
		text-decoration: none;
		border-radius: var(--radius-control);
	}
	.top-link:hover { color: var(--color-accent); }
	.top-link:focus-visible { outline: 2px solid var(--color-you); outline-offset: 2px; }
	.external-icon { flex-shrink: 0; }

	.hero {
		text-align: center;
	}
	.kicker {
		font-family: var(--font-display);
		text-transform: uppercase;
		letter-spacing: 0.28em;
		/* Trailing letter-spacing pushes the centred text right; this pulls it back. */
		margin-right: calc(-1 * var(--space-1));
		font-size: var(--text-small);
		color: var(--color-accent);
	}
	h1 {
		display: flex;
		flex-direction: column;
		align-items: center;
		margin-top: var(--space-1);
		font-family: var(--font-display);
		font-size: var(--text-display);
		line-height: 0.95;
		text-transform: uppercase;
		letter-spacing: 0.01em;
		color: var(--color-accent);
	}
	/* "the" is half the title's size, so it stays a small word in the line at
	   every width. */
	h1 .the {
		font-size: calc(var(--text-display) * 0.5);
		letter-spacing: 0.12em;
	}
	.subtitle {
		margin-top: var(--space-3);
		font-family: var(--font-serif);
		font-size: var(--text-body);
		color: var(--color-text-secondary);
	}

	/* The sign-in form sits on the page with no box: a section gets space, and
	   only the controls in it are boxed. */
	.entry {
		width: 100%;
		max-width: 380px;
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}

	/* Log in and Sign up, as two chips: the one you're on wears the selected
	   border, the way any choice does. */
	.toggle {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-2);
	}
	.toggle button:disabled { cursor: not-allowed; }

	/* Floating-label fields: the label sits inside the input, then shrinks to
	   the top edge on focus or when filled — saving idle vertical space without
	   ever hiding the field's name. */
	.field {
		position: relative;
	}
	.field input {
		min-height: var(--size-tap);
		padding: var(--space-5) var(--space-3) var(--space-1);
	}
	/* Pinned to the field's top-right, opposite the floating label, so the
	   counter appearing mid-typing never reflows the form. */
	.field-counter {
		position: absolute;
		top: var(--space-2);
		right: var(--space-3);
		pointer-events: none;
	}
	.field label {
		position: absolute;
		left: var(--space-3);
		top: 50%;
		transform: translateY(-50%);
		color: var(--color-text-faint);
		font-size: var(--text-body);
		pointer-events: none;
		transition: top 0.15s ease, font-size 0.15s ease, color 0.15s ease, transform 0.15s ease;
		max-width: calc(100% - var(--space-5));
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.field input:focus + label,
	.field input:not(:placeholder-shown) + label {
		top: var(--space-2);
		transform: none;
		font-size: var(--text-label);
		color: var(--color-accent);
	}
	/* Reduced motion (docs/STYLE_GUIDE.md "Motion & the deck"): the label still
	   docks to the top of the field in the accent colour — that is what tells
	   you the field is live and what it wants — it simply appears there rather
	   than sliding and shrinking on the way. The colour is bundled in with the
	   travel here because it is one shorthand; the state it announces is
	   unaffected. */
	@media (prefers-reduced-motion: reduce) {
		.field label { transition: none; }
	}

	.primary { margin-top: var(--space-1); align-self: center; }

	.forgot-password {
		text-align: center;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
	.forgot-password a { color: var(--color-accent); }

	.credits {
		position: absolute;
		bottom: max(var(--space-5), env(safe-area-inset-bottom));
		left: 50%;
		transform: translateX(-50%);
		font-size: var(--text-label);
		color: var(--color-text-secondary);
		text-align: center;
		white-space: nowrap;
	}
	.credits a {
		color: var(--color-accent);
		text-decoration: none;
	}
	.credits a:hover { color: var(--color-accent-hover); }
	.credits a:focus-visible { outline: 2px solid var(--color-you); outline-offset: 2px; border-radius: var(--radius-control); }
</style>
