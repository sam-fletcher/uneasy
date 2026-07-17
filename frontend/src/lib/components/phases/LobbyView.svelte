<!-- LobbyView.svelte
  Lobby phase: join code, player list, the facilitator's "Start Prologue"
  button, the push-notification soft-ask, and the inline help primer.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { startPrologue } from '$lib/api';
	import type { Game, Player } from '$lib/api';
	import { getPushState, enablePush, type PushState } from '$lib/push';
	import HelpContent from '../HelpContent.svelte';
	import { onMount } from 'svelte';

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
			await navigator.clipboard.writeText(game.join_code);
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
	const showPushCard = $derived(
		!pushCardDismissed && (pushState === 'off' || pushState === 'ios-needs-install')
	);
	function dismissPushCard() {
		pushCardDismissed = true;
		localStorage.setItem(PUSH_PROMPT_DISMISSED_KEY, '1');
	}
	async function enablePushFromLobby() {
		pushCardBusy = true;
		try {
			pushState = await enablePush(vapidPublicKey);
		} finally {
			pushCardBusy = false;
			dismissPushCard();
		}
	}

	onMount(async () => {
		pushCardDismissed = localStorage.getItem(PUSH_PROMPT_DISMISSED_KEY) === '1';
		pushState = await getPushState();
	});
</script>

<div class="phase-view lobby">
	{#if error}
		<p class="error-text">{error}</p>
	{/if}
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
			{:else}
				<h2>Get notified when it's your turn</h2>
				<p class="muted-text">
					Turn on push notifications for this device so you don't have to keep checking back.
					You can change this any time in your Profile.
				</p>
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
			any time from the ? in the top-right corner.
		</p>
		<HelpContent {onFeedback} />
	</section>
</div>

<style>
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
