<!--
	ChatPanel.svelte — the unified game-wide chat surface.

	Layout (driven by CSS, not by the parent):
	- <1024px: collapsed = a thin strip pinned to the bottom of the viewport
	  showing the latest message + unread badge. Tapping the strip expands the
	  panel into a full-screen sheet below the page header (header stays
	  visible). Expanding does NOT auto-focus the input — the keyboard only
	  appears when the user taps the input box (matches Discord/Slack/iMessage).
	- ≥1024px: always-open right column. No collapse toggle.

	Three kinds of entries flow through the same feed:
	- 'message'  — free-text from a player (author_id set).
	- 'boundary' — system-emitted phase/row/scene markers; rendered as a divider.
	- 'log'      — (future) action-log entries; rendered with a system glyph and
	               severity-tinted background.

	The parent owns the posts array and pushes new entries via WS; this
	component is a controlled view + an input that POSTs new player messages.
-->
<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { createPlayerPost, type ChatPost, type Player } from '$lib/api';

	interface Props {
		gameID: string | number;
		posts: ChatPost[];
		players: Player[];
		currentPlayerID: number | null;
		typingLabel: string;
	}

	const { gameID, posts, players, currentPlayerID, typingLabel }: Props = $props();

	const playerName = (id: number | null) =>
		id == null ? '' : players.find(p => p.id === id)?.display_name ?? 'Unknown';

	// ── Expand/collapse (mobile only; desktop ignores this state) ─────────────
	let expanded = $state(false);
	function toggleExpanded() { expanded = !expanded; }

	// ── Unread tracking ───────────────────────────────────────────────────────
	// We track the last post ID the user has "seen" — i.e. either the panel is
	// open (desktop, or mobile expanded) or the player just expanded it. New
	// posts arriving while collapsed (and not authored by the current player)
	// count as unread.
	let lastSeenID = $state<number>(0);

	// Initialize lastSeenID once we have posts on first render.
	let initialized = false;
	$effect(() => {
		if (!initialized && posts.length > 0) {
			lastSeenID = posts[posts.length - 1].id;
			initialized = true;
		}
	});

	// While the panel is "open" — desktop column or mobile expanded — keep the
	// seen marker pinned to the latest post. We can't know desktop-vs-mobile
	// directly in JS without window.matchMedia, so we err on the side of
	// keeping it current whenever the panel is rendered open OR on desktop.
	const isOpen = $derived(expanded);
	$effect(() => {
		if (isOpen && posts.length > 0) {
			lastSeenID = posts[posts.length - 1].id;
		}
	});

	// On desktop the panel is always open visually, even if `expanded` is
	// false — use a window matchMedia check to keep unread at 0 there.
	let isDesktop = $state(false);
	onMount(() => {
		const mq = window.matchMedia('(min-width: 1024px)');
		const sync = () => { isDesktop = mq.matches; };
		sync();
		mq.addEventListener('change', sync);
		return () => mq.removeEventListener('change', sync);
	});
	$effect(() => {
		if (isDesktop && posts.length > 0) {
			lastSeenID = posts[posts.length - 1].id;
		}
	});

	const unreadPosts = $derived(
		posts.filter(p => p.id > lastSeenID && p.author_id !== currentPlayerID)
	);
	const unreadCount = $derived(unreadPosts.length);
	const hasImportantUnread = $derived(
		unreadPosts.some(p => p.severity === 'important' || p.kind === 'boundary')
	);

	// ── Latest message preview (for the collapsed strip) ──────────────────────
	const latestPost = $derived(posts.length > 0 ? posts[posts.length - 1] : null);
	const latestPreview = $derived.by(() => {
		if (!latestPost) return 'No messages yet';
		if (latestPost.kind === 'boundary') return latestPost.body;
		if (latestPost.kind === 'log') return latestPost.body;
		const author = playerName(latestPost.author_id);
		return author ? `${author}: ${latestPost.body}` : latestPost.body;
	});

	// ── Auto-scroll to bottom ─────────────────────────────────────────────────
	let feedEl = $state<HTMLElement | null>(null);
	$effect(() => {
		// Re-runs when posts.length changes; tick() lets the new node render
		// before we scroll.
		void posts.length;
		if (!feedEl) return;
		void tick().then(() => {
			if (feedEl) feedEl.scrollTop = feedEl.scrollHeight;
		});
	});

	// ── Compose box ───────────────────────────────────────────────────────────
	let newBody = $state('');
	let sending = $state(false);
	let error = $state('');
	let lastTypingSent = 0;
	let typingStopTimeout: ReturnType<typeof setTimeout> | null = null;

	function onInput() {
		const now = Date.now();
		if (now - lastTypingSent > 2500) {
			window.dispatchEvent(new CustomEvent('uneasy:typing', { detail: { typing: true } }));
			lastTypingSent = now;
		}
		if (typingStopTimeout) clearTimeout(typingStopTimeout);
		typingStopTimeout = setTimeout(() => {
			window.dispatchEvent(new CustomEvent('uneasy:typing', { detail: { typing: false } }));
		}, 2000);
	}

	async function send() {
		const body = newBody.trim();
		if (!body || sending) return;
		sending = true;
		error = '';
		try {
			await createPlayerPost(gameID, body);
			newBody = '';
			// The WS broadcast will append the post to the parent's array; no
			// optimistic insert needed.
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not send.';
		} finally {
			sending = false;
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			send();
		}
	}

	function fmtTime(iso: string) {
		return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}
</script>

<!--
	The component renders TWO surfaces and lets CSS decide which is visible:
	- A "strip" (mobile collapsed only) — a button-like row pinned to the
	  bottom of the viewport.
	- A "panel" — the message list + input. On mobile, only visible when
	  `expanded`; on desktop, always visible.
	This avoids JS-driven layout flips on resize.
-->

<button
	type="button"
	class="strip"
	class:hidden={expanded}
	class:has-unread={unreadCount > 0}
	class:has-important={hasImportantUnread}
	onclick={toggleExpanded}
	aria-label="Open chat"
	aria-expanded={expanded}
>
	<span class="strip-preview">{latestPreview}</span>
	{#if unreadCount > 0}
		<span class="unread-badge">{unreadCount > 99 ? '99+' : unreadCount}</span>
	{/if}
</button>

<aside class="panel" class:expanded aria-label="Chat">
	<header class="panel-header">
		<h2>Chat</h2>
		<button
			type="button"
			class="collapse"
			onclick={toggleExpanded}
			aria-label="Minimize chat"
		>
			✕
		</button>
	</header>

	<div class="feed" bind:this={feedEl}>
		{#if posts.length === 0}
			<p class="empty">No messages yet. Say something.</p>
		{:else}
			{#each posts as post (post.id)}
				{#if post.kind === 'boundary'}
					<div class="boundary" data-code={post.system_code}>
						<span class="boundary-line"></span>
						<span class="boundary-label">{post.body}</span>
						<span class="boundary-line"></span>
					</div>
				{:else if post.kind === 'log'}
					<div class="log" data-severity={post.severity ?? 'default'}>
						<span class="log-glyph" aria-hidden="true">•</span>
						<span class="log-body">{post.body}</span>
						<span class="log-time">{fmtTime(post.created_at)}</span>
					</div>
				{:else}
					<div class="message" class:own={post.author_id === currentPlayerID}>
						<span class="msg-author">{playerName(post.author_id)}</span>
						<span class="msg-body">{post.body}</span>
						<span class="msg-time">{fmtTime(post.created_at)}</span>
					</div>
				{/if}
			{/each}
		{/if}
	</div>

	<div class="typing" aria-live="polite">{typingLabel}</div>

	{#if error}<p class="error">{error}</p>{/if}

	<div class="compose">
		<textarea
			placeholder="Write a message…"
			bind:value={newBody}
			oninput={onInput}
			onkeydown={onKeydown}
			rows={2}
			disabled={sending}
		></textarea>
		<button class="send" onclick={send} disabled={sending || !newBody.trim()}>
			{sending ? '…' : 'Send'}
		</button>
	</div>
</aside>

<style>
	/* ── Strip (mobile collapsed) ────────────────────────────────────────── */

	.strip {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		min-height: 56px; /* ≥44px tap target plus breathing room */
		/* Extra bottom padding accounts for the iOS home-indicator safe area
		   so the preview text isn't clipped on devices with a gesture bar. */
		padding: 0.85rem 1rem calc(0.85rem + env(safe-area-inset-bottom));
		display: flex;
		align-items: center;
		gap: 0.6rem;
		background: #2a2620;
		border-top: 1px solid #6a5a3a;
		color: #e8e4d9;
		font-size: 0.9rem;
		text-align: left;
		z-index: 50;
		cursor: pointer;
		box-shadow: 0 -4px 12px rgba(0, 0, 0, 0.35);
	}

	.strip-preview {
		flex: 1;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.strip.has-unread { color: #e8e4d9; }
	.strip.has-important {
		border-top: 2px solid #c8a96e;
	}

	.unread-badge {
		background: #c8a96e;
		color: #1a1a1a;
		font-weight: 700;
		font-size: 0.72rem;
		padding: 0.1rem 0.5rem;
		border-radius: 999px;
		min-width: 1.5rem;
		text-align: center;
	}

	.strip.hidden { display: none; }

	/* ── Panel (mobile expanded sheet, desktop right column) ─────────────── */

	.panel {
		display: none;
		flex-direction: column;
		min-height: 0;
		background: #181818;
		color: #d8d4c9;
	}

	.panel.expanded {
		display: flex;
		position: absolute;
		inset: 0;
		z-index: 40;
		border-top: 1px solid #2a2a2a;
	}

	.panel-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.5rem 0.8rem;
		border-bottom: 1px solid #2a2a2a;
		flex-shrink: 0;
	}

	.panel-header h2 {
		margin: 0;
		font-size: 0.95rem;
		color: #c8a96e;
		font-weight: 600;
	}

	.collapse {
		background: none;
		border: none;
		color: #888;
		font-size: 1.1rem;
		cursor: pointer;
		padding: 0.2rem 0.4rem;
		min-width: 44px;
		min-height: 44px;
	}

	/* On desktop, hide the collapse button and the strip; the panel is the
	   permanent right column. */
	@media (min-width: 1024px) {
		.strip { display: none; }
		.collapse { display: none; }
		.panel {
			display: flex;
			position: static;
			height: 100%;
			width: 100%;
		}
	}

	/* ── Feed ─────────────────────────────────────────────────────────────── */

	.feed {
		flex: 1;
		overflow-y: auto;
		padding: 0.6rem 0.8rem;
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
		min-height: 0;
	}

	.empty {
		color: #666;
		text-align: center;
		margin-top: 2rem;
		font-size: 0.85rem;
	}

	.message {
		display: grid;
		grid-template-columns: auto 1fr auto;
		gap: 0.4rem;
		align-items: baseline;
	}

	.msg-author {
		font-weight: 600;
		color: #c8a96e;
		font-size: 0.82rem;
		white-space: nowrap;
	}

	.message.own .msg-author { color: #e8d8a0; }

	.msg-body {
		font-size: 0.9rem;
		line-height: 1.45;
		white-space: pre-wrap;
		word-break: break-word;
	}

	.msg-time {
		font-size: 0.7rem;
		color: #555;
		white-space: nowrap;
	}

	/* Boundary divider */
	.boundary {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0.4rem 0;
	}
	.boundary-line {
		flex: 1;
		height: 1px;
		background: #3a3020;
	}
	.boundary-label {
		font-size: 0.78rem;
		color: #c8a96e;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		white-space: nowrap;
	}

	/* Log entry */
	.log {
		display: grid;
		grid-template-columns: auto 1fr auto;
		gap: 0.45rem;
		align-items: baseline;
		font-size: 0.85rem;
		color: #b0a890;
	}
	.log[data-severity='important'] { color: #e8d8a0; }
	.log-glyph { color: #888; }
	.log-time { font-size: 0.7rem; color: #555; white-space: nowrap; }

	/* ── Typing + compose ─────────────────────────────────────────────────── */

	.typing {
		font-size: 0.78rem;
		color: #777;
		min-height: 1.2em;
		padding: 0 0.8rem;
		flex-shrink: 0;
	}

	.error {
		color: #e07070;
		font-size: 0.8rem;
		padding: 0 0.8rem;
		margin: 0;
		flex-shrink: 0;
	}

	.compose {
		display: flex;
		gap: 0.5rem;
		padding: 0.5rem 0.8rem 0.7rem;
		border-top: 1px solid #2a2a2a;
		align-items: flex-end;
		flex-shrink: 0;
	}

	textarea {
		flex: 1;
		font-size: 0.9rem;
		padding: 0.5rem 0.7rem;
		border-radius: 6px;
		border: 1px solid #444;
		background: #2a2a2a;
		color: inherit;
		font-family: inherit;
		resize: none;
		line-height: 1.4;
		min-height: 44px;
	}

	textarea:focus {
		outline: 2px solid #c8a96e;
		outline-offset: 1px;
	}

	.send {
		background: #c8a96e;
		color: #1a1a1a;
		font-weight: 600;
		padding: 0.5rem 0.9rem;
		min-width: 56px;
		min-height: 44px;
		align-self: flex-end;
		border-radius: 6px;
		border: none;
		cursor: pointer;
	}

	.send:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
