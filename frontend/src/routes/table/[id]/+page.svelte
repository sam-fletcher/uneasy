<!-- Table page: post feed, presence, typing indicators. -->
<script lang="ts">
	import { page } from '$app/state';
	import { onMount, onDestroy } from 'svelte';
	import { getTable, listPosts, createPost } from '$lib/api';
	import { createConnection, type WSMessage } from '$lib/ws';
	import type { Game, Player, Post, PresenceMember } from '$lib/api';

	const gameID = $derived(page.params.id);

	let game = $state<Game | null>(null);
	let posts = $state<Post[]>([]);
	let members = $state<PresenceMember[]>([]);
	let typingNames = $state<string[]>([]);
	let newPostBody = $state('');
	let sending = $state(false);
	let error = $state('');
	let feedEl = $state<HTMLElement | null>(null);

	// Track which players are currently typing (keyed by player_id).
	let typingMap = new Map<number, string>();
	let typingTimeouts = new Map<number, ReturnType<typeof setTimeout>>();

	// WebSocket connection (managed by ws.ts).
	let disconnect: (() => void) | null = null;

	// Scroll the feed to the bottom whenever posts change.
	$effect(() => {
		if (posts.length > 0 && feedEl) {
			feedEl.scrollTop = feedEl.scrollHeight;
		}
	});

	function handleWSMessage(msg: WSMessage) {
		switch (msg.type) {
			case 'post.created': {
				const post = msg.payload.post as Post;
				// Avoid duplicates if the HTTP response already added it.
				if (!posts.find((p) => p.id === post.id)) {
					posts = [...posts, post];
				}
				break;
			}
			case 'presence.snapshot': {
				members = msg.payload.members as PresenceMember[];
				break;
			}
			case 'typing.update': {
				const { player_id, display_name, typing } = msg.payload as {
					player_id: number;
					display_name: string;
					typing: boolean;
				};
				// Clear any existing auto-stop timeout for this player.
				const existingTimeout = typingTimeouts.get(player_id);
				if (existingTimeout) clearTimeout(existingTimeout);

				if (typing) {
					typingMap.set(player_id, display_name);
					// Auto-clear after 4 seconds in case typing.stop is missed.
					typingTimeouts.set(player_id, setTimeout(() => {
						typingMap.delete(player_id);
						typingNames = [...typingMap.values()];
					}, 4000));
				} else {
					typingMap.delete(player_id);
					typingTimeouts.delete(player_id);
				}
				typingNames = [...typingMap.values()];
				break;
			}
		}
	}

	onMount(async () => {
		try {
			const data = await getTable(gameID);
			game = data.game;
			members = data.players.map((p: Player) => ({ id: p.id, display_name: p.display_name, online: false }));

			const postsData = await listPosts(gameID);
			posts = postsData.posts;

			// Open WebSocket — it takes over presence and live updates from here.
			disconnect = createConnection(gameID, handleWSMessage);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load table.';
		}
	});

	onDestroy(() => {
		disconnect?.();
		typingTimeouts.forEach(clearTimeout);
	});

	// Throttled typing indicator sender.
	let lastTypingSent = 0;
	let typingStopTimeout: ReturnType<typeof setTimeout> | null = null;

	function onInput() {
		const now = Date.now();
		if (now - lastTypingSent > 2500) {
			// Emit typing.start (ws.ts handles this)
			window.dispatchEvent(new CustomEvent('uneasy:typing', { detail: { typing: true } }));
			lastTypingSent = now;
		}
		if (typingStopTimeout) clearTimeout(typingStopTimeout);
		typingStopTimeout = setTimeout(() => {
			window.dispatchEvent(new CustomEvent('uneasy:typing', { detail: { typing: false } }));
		}, 2000);
	}

	async function sendPost() {
		const body = newPostBody.trim();
		if (!body || sending) return;
		sending = true;
		// Optimistic: show the post immediately (it will be deduped when the WS echoes it).
		try {
			const { post } = await createPost(gameID, body);
			newPostBody = '';
			// Dedup: the WS post.created event may have arrived first and already
			// added this post to the feed. Only add it here if it's not present.
			if (!posts.find((p) => p.id === post.id)) {
				posts = [...posts, post];
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not send.';
		} finally {
			sending = false;
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey) {
			e.preventDefault();
			sendPost();
		}
	}

	const typingLabel = $derived(
		typingNames.length === 0 ? '' :
		typingNames.length === 1 ? `${typingNames[0]} is writing…` :
		typingNames.length === 2 ? `${typingNames[0]} and ${typingNames[1]} are writing…` :
		'Several people are writing…'
	);
</script>

<div class="table-page">
	<!-- Header -->
	<header>
		<div class="game-info">
			<span class="game-title">The Table</span>
			{#if game}
				<button class="code-badge" onclick={() => navigator.clipboard.writeText(game!.join_code)}>
					{game.join_code}
					<span class="copy-hint">copy</span>
				</button>
			{/if}
		</div>

		<!-- Member list (inline on mobile, sidebar on wider screens) -->
		<div class="members">
			{#each members as member}
				<span class="member" class:online={member.online}>
					<span class="dot"></span>
					{member.display_name}
				</span>
			{/each}
		</div>
	</header>

	<!-- Post feed -->
	<div class="feed" bind:this={feedEl}>
		{#if posts.length === 0}
			<p class="empty">No posts yet. Say something.</p>
		{:else}
			{#each posts as post (post.id)}
				<div class="post">
					<span class="post-author">
						{members.find((m) => m.id === post.author_id)?.display_name ?? 'Unknown'}
					</span>
					<span class="post-body">{post.body}</span>
					<span class="post-time">{new Date(post.created_at).toLocaleTimeString()}</span>
				</div>
			{/each}
		{/if}
	</div>

	<!-- Typing indicator -->
	<div class="typing-indicator" aria-live="polite">
		{typingLabel}
	</div>

	<!-- Input -->
	<div class="input-row">
		{#if error}
			<p class="error">{error}</p>
		{/if}
		<textarea
			placeholder="Write something… (Enter to send, Shift+Enter for newline)"
			bind:value={newPostBody}
			oninput={onInput}
			onkeydown={onKeydown}
			rows={2}
			disabled={sending}
		></textarea>
		<button class="send" onclick={sendPost} disabled={sending || !newPostBody.trim()}>
			{sending ? '…' : 'Send'}
		</button>
	</div>
</div>

<style>
	.table-page {
		display: flex;
		flex-direction: column;
		height: 100dvh;
		max-width: 100%;
	}

	header {
		padding: 0.75rem 0;
		border-bottom: 1px solid #333;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.game-info {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.game-title {
		font-weight: 700;
		font-size: 1.1rem;
		color: #c8a96e;
	}

	.code-badge {
		font-family: monospace;
		font-size: 0.85rem;
		background: #333;
		color: #e8e4d9;
		padding: 0.2rem 0.6rem;
		border-radius: 4px;
		letter-spacing: 0.1em;
		display: flex;
		gap: 0.4rem;
		align-items: center;
	}

	.copy-hint {
		font-size: 0.7rem;
		color: #888;
	}

	.members {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.member {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.85rem;
		color: #888;
	}

	.member.online {
		color: #e8e4d9;
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: #555;
	}

	.member.online .dot {
		background: #6dbf7a;
	}

	.feed {
		flex: 1;
		overflow-y: auto;
		padding: 1rem 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.empty {
		color: #666;
		text-align: center;
		margin-top: 2rem;
	}

	.post {
		display: grid;
		grid-template-columns: auto 1fr auto;
		gap: 0.5rem;
		align-items: baseline;
	}

	.post-author {
		font-weight: 600;
		color: #c8a96e;
		font-size: 0.9rem;
		white-space: nowrap;
	}

	.post-body {
		line-height: 1.5;
		white-space: pre-wrap;
		word-break: break-word;
	}

	.post-time {
		font-size: 0.75rem;
		color: #666;
		white-space: nowrap;
	}

	.typing-indicator {
		font-size: 0.8rem;
		color: #888;
		height: 1.2em;
		padding: 0.1rem 0;
	}

	.input-row {
		display: flex;
		gap: 0.5rem;
		padding-top: 0.5rem;
		border-top: 1px solid #333;
		align-items: flex-end;
	}

	textarea {
		flex: 1;
		font-size: 1rem;
		padding: 0.6rem 0.8rem;
		border-radius: 6px;
		border: 1px solid #444;
		background: #2a2a2a;
		color: inherit;
		font-family: inherit;
		resize: none;
		line-height: 1.4;
	}

	textarea:focus {
		outline: 2px solid #c8a96e;
		outline-offset: 1px;
	}

	.send {
		background: #c8a96e;
		color: #1a1a1a;
		font-weight: 600;
		padding: 0.6rem 1rem;
		min-width: 60px;
		align-self: flex-end;
	}

	.send:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.error {
		color: #e07070;
		font-size: 0.85rem;
	}
</style>
