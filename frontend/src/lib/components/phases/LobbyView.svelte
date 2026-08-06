<!-- LobbyView.svelte
  Lobby phase: join code, player list, the facilitator's "Start Prologue"
  button, the push-notification soft-ask, and the inline help primer.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { startPrologue } from '$lib/api';
	import type { Game, Player } from '$lib/api';
	import { getPushState, enablePush, onPushPermissionChange, type PushState } from '$lib/push';
	import PushBlockedHelp from '$lib/components/shared/PushBlockedHelp.svelte';
	import HelpContent from '../HelpContent.svelte';
	import HelpGlyph from '../HelpGlyph.svelte';
	import { onMount } from 'svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	let {
		gameID,
		game,
		players,
		isFacilitator,
		vapidPublicKey,
		onFeedback,
	}: {
		gameID: string;
		game: Game;
		players: Player[];
		isFacilitator: boolean;
		vapidPublicKey: string;
		onFeedback: () => void;
	} = $props();

	let error = $state('');

	// ── Join-code copy feedback ───────────────────────────────────────────────
	let joinCodeCopied = $state(false);
	let joinCodeCopyTimer: ReturnType<typeof setTimeout> | null = null;
	async function copyJoinCode() {
		try {
			await navigator.clipboard.writeText(`https://uneasy.up.railway.app/ Table code: ${game.join_code}`);
			joinCodeCopied = true;
			if (joinCodeCopyTimer) clearTimeout(joinCodeCopyTimer);
			joinCodeCopyTimer = setTimeout(() => (joinCodeCopied = false), 1500);
		} catch {
			// Clipboard can reject (permissions / insecure context); leave the
			// label unchanged so the user can still read & copy manually.
		}
	}

	// ── Phase advancement ─────────────────────────────────────────────────────
	let advancing = $state(false);
	async function advancePhase() {
		if (advancing) return;
		advancing = true;
		error = '';
		try {
			await startPrologue(gameID);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not advance phase.';
		} finally {
			advancing = false;
		}
	}

	// ── Push soft-ask ─────────────────────────────────────────────────────────
	const PUSH_PROMPT_DISMISSED_KEY = 'uneasy.push.lobbyPromptDismissed';
	let pushState = $state<PushState>('unsupported');
	let pushCardDismissed = $state(true);
	let pushCardBusy = $state(false);
	let pushError = $state('');
	// Whether *this* visit asked the browser. Gates the blocked-recovery copy:
	// someone who blocked us months ago shouldn't be nagged about it on every
	// lobby they open — that case belongs to the Profile page.
	let pushAttempted = $state(false);
	const showPushCard = $derived(
		!pushCardDismissed &&
			(pushState === 'off' ||
				pushState === 'ios-needs-install' ||
				(pushAttempted && pushState === 'denied'))
	);
	function dismissPushCard() {
		pushCardDismissed = true;
		localStorage.setItem(PUSH_PROMPT_DISMISSED_KEY, '1');
	}
	async function enablePushFromLobby() {
		pushCardBusy = true;
		pushError = '';
		try {
			pushState = await enablePush(vapidPublicKey);
		} catch (e) {
			pushError = e instanceof Error ? e.message : 'Could not turn on notifications.';
			pushState = await getPushState().catch(() => pushState);
		} finally {
			pushAttempted = true;
			pushCardBusy = false;
			// Retire the card only on success. It used to dismiss unconditionally
			// — and dismissal is written to localStorage — so a refusal hid the
			// card forever with no acknowledgement at all: the player saw the
			// browser's "Notifications blocked" bubble and then nothing from us,
			// with no route back short of finding the Profile page.
			if (pushState === 'on') dismissPushCard();
		}
	}

	function refreshPushState() {
		// Not mid-request: the permission prompt blurs the page, so the focus
		// handler below fires *during* enablePush. That read would see
		// granted-but-not-yet-subscribed ('off') and could land after
		// enablePush's own 'on', flipping the card back to the ask.
		if (pushCardBusy) return;
		void getPushState()
			.then((s) => {
				// Both attempt annotations below ("blocked" / "nothing changed")
				// describe the last click. A state change since then can only have
				// come from browser settings, which makes them stale — drop them
				// so a player who has just un-blocked us gets the clean ask back.
				if (s !== pushState) pushAttempted = false;
				pushState = s;
			})
			.catch(() => {});
	}

	onMount(() => {
		pushCardDismissed = localStorage.getItem(PUSH_PROMPT_DISMISSED_KEY) === '1';
		refreshPushState();
		// Following the recovery steps happens in browser settings, not here, so
		// re-read on the way back — otherwise the card keeps insisting they're
		// blocked after they've allowed us. The permission event covers the
		// same-tab case; the focus/visibility pair covers Safari, which rejects
		// permissions.query for notifications. This reads the browser, not the
		// server, so it costs nothing per fire.
		const unsubscribePermission = onPushPermissionChange(refreshPushState);
		const onVisibility = () => {
			if (document.visibilityState === 'visible') refreshPushState();
		};
		document.addEventListener('visibilitychange', onVisibility);
		window.addEventListener('focus', refreshPushState);
		return () => {
			unsubscribePermission();
			document.removeEventListener('visibilitychange', onVisibility);
			window.removeEventListener('focus', refreshPushState);
		};
	});
</script>

<div class="phase-view lobby">
	{#if error}
		<ErrorText message={error} />
	{/if}
	<p class="prologue-lede">
		In <em>Uneasy Lies the Head</em>, you and your friends will:
	</p>
	<ul class="prologue-lede-list">
		<li> Plot and scheme as antagonistic members of a royal court </li>
		<li> Prepare plans to spread rumors, enact laws, host festivities, propose duels, and more </li>
	</ul>
	<section class="lobby-join">
		<h2>Join Code</h2>
		<button class="code-badge" class:copied={joinCodeCopied} onclick={copyJoinCode} aria-label="Copy join code">
			{game.join_code}
			<span class="copy-hint" aria-live="polite">{joinCodeCopied ? 'Copied!' : 'copy'}</span>
		</button>
		<p class="muted-text">
			Share this code with your friends to invite them. The game needs 2–5 players.
		</p>
	</section>
	<div class="player-list">
		{#each players as p}
			<div class="player-row">
				{p.display_name}
				{#if p.is_facilitator}<span class="tag">facilitator</span>{/if}
			</div>
		{/each}
	</div>
	{#if isFacilitator && players.length >= 2}
		<button class="action-btn primary" onclick={advancePhase} disabled={advancing}>
			{advancing ? '…' : 'Start Prologue'}
		</button>
	{:else if isFacilitator}
		<p class="muted-text">Need at least 2 players to start.</p>
	{/if}

	{#if showPushCard}
		<section class="push-card">
			{#if pushState === 'ios-needs-install'}
				<h2>Add Uneasy to your Home Screen</h2>
				<p class="muted-text">
					iPhone/iPad only deliver notifications to installed apps: tap the Share icon,
					then "Add to Home Screen". Open Uneasy from there to get notified when it's your turn.
				</p>
				<button class="action-btn secondary" onclick={dismissPushCard}>Got it</button>
			{:else if pushState === 'denied'}
				<div role="status">
					<h2>Your browser blocked the request</h2>
					<p class="muted-text">
						That's a browser setting, so we can't ask again from here — but you can undo it:
					</p>
					<PushBlockedHelp />
				</div>
				<button class="action-btn secondary" onclick={dismissPushCard}>Got it</button>
			{:else}
				<h2>Get notified when it's your turn</h2>
				<p class="muted-text">
					Turn on push notifications for this device so you don't have to keep checking back.
					You can change this any time in your Profile.
				</p>
				{#if pushError}
					<ErrorText message={pushError} />
				{:else if pushAttempted}
					<p class="muted-text small" role="status">
						Nothing changed — the browser prompt was closed without an answer. You can try again.
					</p>
				{/if}
				<div class="push-card-actions">
					<button class="action-btn primary" onclick={enablePushFromLobby} disabled={pushCardBusy}>
						{pushCardBusy ? '…' : 'Enable notifications'}
					</button>
					<button class="action-btn secondary" onclick={dismissPushCard}>Not now</button>
				</div>
			{/if}
		</section>
	{/if}

	<section class="lobby-help">
		<h2>New to the game? Start here.</h2>
		<p class="muted-text">
			A two-minute primer while you wait for everyone to arrive. You can reopen this
			any time from the <span class="help-cue" role="img" aria-label="help button"
				><HelpGlyph size="1.15em" /></span
			> in the top-right corner.
			A skim is enough for now.
		</p>
		<HelpContent {onFeedback} />
	</section>
</div>

<style>
	.prologue-lede-list {
		margin: 0;
		/* padding-inline-start: 1.25rem;
		list-style-position: outside; */
		padding-left: 1.25rem;
		color: var(--color-text);
		font-size: 1.05rem;
		line-height: 1.45;
	}
	.phase-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		padding: 1rem 0.75rem;
		gap: 1rem;
		overflow-y: auto;
		min-height: 0;
	}

	.lobby h2 {
		color: var(--color-accent);
		font-size: 1.15rem;
		margin: 0 0 0.35rem;
	}

	.player-list { display: flex; flex-direction: column; gap: 0.4rem; }

	.player-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.95rem;
	}

	.tag {
		font-size: 0.7rem;
		background: var(--color-chip-violet-bg);
		border: 1px solid var(--color-chip-violet-border);
		color: var(--color-chip-violet-text);
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		text-transform: uppercase;
	}

	.code-badge {
		font-family: monospace;
		font-size: 0.85rem;
		background: var(--color-border);
		color: var(--color-text);
		padding: 0.2rem 0.6rem;
		border-radius: 4px;
		letter-spacing: 0.1em;
		display: flex;
		gap: 0.4rem;
		align-items: center;
	}
	.copy-hint {
		font-size: 0.7rem;
		color: var(--color-text-muted);
	}
	.code-badge.copied .copy-hint { color: var(--color-accent); }

	.push-card {
		margin-top: 0.75rem;
		padding: 1rem;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: 12px;
	}
	.push-card .muted-text { margin-bottom: 0.75rem; }
	.push-card-actions { display: flex; gap: 0.6rem; flex-wrap: wrap; }

	.lobby-help {
		margin-top: 0.5rem;
		padding-top: 1rem;
		border-top: 1px solid var(--color-border);
	}
	.lobby-help .muted-text { margin-bottom: 0.9rem; }
	/* Mid-sentence stand-in for the header's help control: same glyph, same
	   gold, sized a touch over 1em so the circle reads at caption size and
	   nudged onto the text baseline (a geometric mark centres, a letterform
	   sits). Inline-flex keeps it from stretching the line box. */
	.help-cue {
		display: inline-flex;
		align-items: center;
		color: var(--color-accent);
		vertical-align: -0.2em;
	}

	.lobby-join { margin-bottom: 0.5rem; }
	.lobby-join .muted-text {
		margin-top: 0.5rem;
		margin-bottom: 0.2rem;
	}
	.lobby-join .code-badge {
		display: inline-flex;
		font-size: 1rem;
		padding: 0.35rem 0.8rem;
	}
</style>
