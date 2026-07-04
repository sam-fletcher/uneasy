<!--
	ChatPanel.svelte — the unified game-wide chat surface.

	Layout (driven by CSS, not by the parent):
	- <1024px: collapsed = a thin strip pinned to the bottom of the viewport
	  showing the latest message + unread badge. Tapping the strip expands the
	  panel into a full-screen sheet below the page header (header stays
	  visible). Expanding does NOT auto-focus the input — the keyboard only
	  appears when the user taps the input box (matches Discord/Slack/iMessage).
	- ≥1024px: always-open right column. No collapse toggle.

	Two shapes of entry flow through the same feed:
	- Player message: author_id != null, system_code == null, severity 0.
	- System post:    author_id == null, system_code != null, severity > 0.
	  The Public Record sidebar's jump gestures key off system_code, not
	  severity — e.g. row.advanced is SEVERITY.BOUNDARY (100), while
	  scene.started and plan.prepared are SEVERITY.IMPORTANT (75).
	  Severity only drives the "hide bookkeeping" filter threshold.

	The parent owns the feed window (via `feed`, a $lib/chatFeed.ChatFeedContext
	— see adr/CHAT_OVERHAUL_PLAN.md Phase 2) and keeps it in sync over WS; this
	component renders it, drives pagination/read-marker/history-mode against
	it, and POSTs new player messages.
-->
<script lang="ts">
	import '$lib/components/shared/statusText.css';
	import { onMount, onDestroy, tick, untrack } from 'svelte';
	import {
		createPlayerPost,
		type Asset,
		type Player,
		type Scene,
		type ScenePeerView,
	} from '$lib/api';
	import { playerColorByID, OOC_COLOR } from '$lib/playerColor';
	import { SEVERITY } from '$lib/severity';
	import {
		type ChatFeedContext,
		buildFeedItems,
		countUnread,
		isNearBottom,
		fetchOlderPage,
		reportReadMarker,
		returnToNow,
	} from '$lib/chatFeed';

	// System-log bodies use a tiny markup subset: **…** spans wrap
	// player-authored asset names (the backend emits them via assetMark).
	// renderLogBody escapes the body first — names are user input — then turns the
	// **…** the server produced into <em>, so it's safe to inject with {@html}.
	// Emphasis is rendered *italic*, not bold — a deliberate style choice so
	// names read distinctly from prose without the visual weight of bold (see
	// assetMark in system_posts.go). The double-asterisk delimiter stays so a
	// stray '*' in quoted marginalia text doesn't trip the parser. Player chat
	// messages do NOT pass through here; their ** is shown verbatim.
	function escapeHtml(s: string): string {
		return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
	}
	function renderLogBody(body: string): string {
		return escapeHtml(body).replace(/\*\*(.+?)\*\*/g, '<em>$1</em>');
	}
	// stripLogMarkup drops the ** delimiters for plain-text contexts (the
	// collapsed-strip preview) where markup can't render.
	function stripLogMarkup(body: string): string {
		return body.replace(/\*\*(.+?)\*\*/g, '$1');
	}

	interface Props {
		gameID: string | number;
		/** Owns the post window, live/history mode, and cursors — see
		 *  $lib/chatFeed. A stable object; its fields are reactive getters
		 *  backed by the page's own $state. */
		feed: ChatFeedContext;
		players: Player[];
		currentPlayerID: number | null;
		typingLabel: string;
		/**
		 * Active scene + present peer rows from the server, or null if no
		 * scene is in progress. Used to populate the persona picker so the
		 * caller sees only assets they are allowed to speak as.
		 */
		activeScene?: Scene | null;
		activeScenePeers?: ScenePeerView[];
		/** Full asset list — used to look up names for the persona picker. */
		assets?: Asset[];
		/**
		 * Jump request from the Public Record sidebar. When this changes to a
		 * non-null value, the panel expands (on mobile) and scrolls to the
		 * post with the given ID. The `key` field disambiguates repeated
		 * requests for the same post so the effect re-runs. By the time this
		 * fires, the caller (+page.svelte) has already ensured the post is
		 * loaded into `feed` — resolving the anchor and entering history mode
		 * if it wasn't in the live window.
		 */
		jumpRequest?: { postID: number; key: number } | null;
		/**
		 * Mobile expand/collapse state. Bindable so the page can close the chat
		 * when another full-screen surface (Tones/Laws/Rumors/Retinue/War) opens
		 * — only one such surface is shown at a time on mobile. Ignored on
		 * desktop, where the panel is a permanent column.
		 */
		expanded?: boolean;
	}

	let {
		gameID,
		feed,
		players,
		currentPlayerID,
		typingLabel,
		activeScene = null,
		activeScenePeers = [],
		assets = [],
		jumpRequest = null,
		expanded = $bindable(false),
	}: Props = $props();

	const posts = $derived(feed.posts);

	const playerName = (id: number | null) =>
		id == null ? '' : players.find(p => p.id === id)?.display_name ?? 'Unknown';
	const assetName = (id: number | null) =>
		id == null ? '' : assets.find(a => a.id === id)?.name ?? '';

	// ── Persona picker ────────────────────────────────────────────────────────
	// Per the rules, players speak as characters only during a Scene. The set
	// of personae the current player may speak as right now:
	//   - Outside a scene: none — they simply speak as themselves.
	//   - During a scene: any peer in the scene whose controller_player_id ==
	//     self, plus their own main character when they are the focus player
	//     (the focus player's MC is implicitly present and never recorded as a
	//     scene_peer).
	// Plus a constant "self" option (posts with no character attribution),
	// labelled with the player's own name.

	type Persona =
		| { kind: 'self'; label: string }
		| { kind: 'asset'; assetID: number; label: string };

	const selfName = $derived(playerName(currentPlayerID) || 'You');

	// Only the focus player's main character is implicitly present; everyone
	// else's MC appears in activeScenePeers if the focus player added it.
	const isFocusPlayer = $derived(
		activeScene != null && currentPlayerID != null && activeScene.focus_player_id === currentPlayerID
	);

	const ownMainCharacter = $derived(
		currentPlayerID == null
			? null
			: assets.find(a => a.is_main_character && a.owner_id === currentPlayerID && !a.is_destroyed) ?? null
	);

	const myControlledPeers = $derived(
		activeScene == null || currentPlayerID == null
			? []
			: activeScenePeers
				.filter(p => p.controller_player_id === currentPlayerID)
				.map(p => assets.find(a => a.id === p.peer_asset_id))
				.filter((a): a is Asset => a != null)
	);

	const personae = $derived.by<Persona[]>(() => {
		const list: Persona[] = [];
		// Character personae are available only while a scene is active.
		if (activeScene != null) {
			if (isFocusPlayer && ownMainCharacter) {
				list.push({ kind: 'asset', assetID: ownMainCharacter.id, label: ownMainCharacter.name });
			}
			for (const peer of myControlledPeers) {
				// Avoid duplicating the main character if it's also in the peer list.
				if (peer.id === ownMainCharacter?.id) continue;
				list.push({ kind: 'asset', assetID: peer.id, label: peer.name });
			}
		}
		list.push({ kind: 'self', label: selfName });
		return list;
	});

	// Currently selected persona. Defaults to speaking as oneself. Clamps back
	// to a valid option whenever personae change — e.g. when a scene ends and
	// the character options disappear.
	let selectedPersonaID = $state<number | 'self'>('self');
	$effect(() => {
		const valid = personae.some(p =>
			(p.kind === 'self' && selectedPersonaID === 'self') ||
			(p.kind === 'asset' && p.assetID === selectedPersonaID)
		);
		if (!valid) {
			const main = personae.find(p => p.kind === 'asset');
			selectedPersonaID = main ? (main as { assetID: number }).assetID : 'self';
		}
	});

	const selectedPersona = $derived(
		personae.find(p =>
			(p.kind === 'self' && selectedPersonaID === 'self') ||
			(p.kind === 'asset' && p.assetID === selectedPersonaID)
		) ?? personae[personae.length - 1]
	);

	let pickerOpen = $state(false);

	// ── Expand/collapse (mobile only; desktop ignores this state) ─────────────
	// `expanded` is a bindable prop (see Props above) so the page can close
	// the sheet when another surface opens. Closing flushes any pending
	// read-marker report — see "Read-marker reporting" below.
	function toggleExpanded() {
		const closing = expanded;
		expanded = !expanded;
		if (closing) flushReadReport();
	}

	// On desktop the panel is always visible as a column, even before the user
	// has explicitly "expanded" anything (that flag is mobile-only).
	let isDesktop = $state(false);
	onMount(() => {
		const mq = window.matchMedia('(min-width: 1024px)');
		const sync = () => { isDesktop = mq.matches; };
		sync();
		mq.addEventListener('change', sync);
		// ESC closes the expanded mobile sheet (matches the other overlays;
		// no-op on desktop where the panel is a permanent column).
		const onKey = (e: KeyboardEvent) => {
			if (e.key === 'Escape' && expanded && !isDesktop) expanded = false;
		};
		window.addEventListener('keydown', onKey);
		return () => {
			mq.removeEventListener('change', sync);
			window.removeEventListener('keydown', onKey);
		};
	});
	const isOpen = $derived(expanded || isDesktop);

	// ── Unread state (server-side marker; adr/CHAT_OVERHAUL_PLAN.md Phase 1c/2a) ─
	const unreadCount = $derived(countUnread(posts, feed.lastReadPostID, currentPlayerID));
	const hasImportantUnread = $derived(
		posts.some(p =>
			p.id > feed.lastReadPostID && p.author_id !== currentPlayerID && p.severity >= SEVERITY.IMPORTANT
		)
	);

	// ── Severity threshold filter ─────────────────────────────────────────────
	// Player messages (author_id != null) are always shown; the threshold
	// only affects system posts. Default to DEFAULT (50) so trace and minor
	// log noise is hidden until the user opts in. (Phase 3 replaces this
	// dropdown with a single "hide bookkeeping" toggle.)
	let severityThreshold = $state<number>(SEVERITY.DEFAULT);

	const visiblePosts = $derived(
		posts.filter(p => p.author_id != null || p.severity >= severityThreshold)
	);

	// Render list: day dividers + the "New messages" divider (Phase 2a).
	// `feed.initialReadMarker` is a snapshot taken when the window was last
	// (re)loaded from scratch, so the divider holds still while the live
	// marker (`feed.lastReadPostID`) advances underneath it.
	const feedItems = $derived(
		buildFeedItems(visiblePosts, { unreadAfterID: feed.initialReadMarker, currentPlayerID })
	);

	// ── Latest message preview (for the collapsed strip) ──────────────────────
	const latestPost = $derived(posts.length > 0 ? posts[posts.length - 1] : null);
	const latestPreview = $derived.by(() => {
		if (!latestPost) return 'No messages yet';
		// System posts (no author) just show the body — boundaries and log
		// entries are already self-contained sentences. Strip the bold markup
		// since the strip is plain text.
		if (latestPost.author_id == null) return stripLogMarkup(latestPost.body);
		const author = playerName(latestPost.author_id);
		return author ? `${author}: ${latestPost.body}` : latestPost.body;
	});

	// ── Scroll behavior (Phase 2c) ─────────────────────────────────────────────
	// Auto-follow the tail only while the reader is at (or returns to) the
	// bottom; otherwise leave their position alone and surface a "↓ N new"
	// pill instead of yanking them down mid-read.
	let feedEl = $state<HTMLElement | null>(null);
	const NEAR_BOTTOM_PX = 150;
	let stickToBottom = $state(true);
	// Baseline post id captured the moment the reader leaves the bottom; the
	// pill counts everything newer than it. Null while stuck to the bottom
	// (no pill to show).
	let bottomBaselineID = $state<number | null>(null);
	const pendingNewCount = $derived(
		bottomBaselineID == null ? 0 : visiblePosts.filter(p => p.id > bottomBaselineID!).length
	);

	function scrollToBottomNow() {
		stickToBottom = true;
		bottomBaselineID = null;
		void tick().then(() => { if (feedEl) feedEl.scrollTop = feedEl.scrollHeight; });
	}

	async function handleReturnToNow() {
		await returnToNow(feed);
		scrollToBottomNow();
	}

	async function loadOlder() {
		if (!feedEl) return;
		const oldScrollHeight = feedEl.scrollHeight;
		const oldScrollTop = feedEl.scrollTop;
		await fetchOlderPage(feed);
		await tick();
		if (feedEl) feedEl.scrollTop = oldScrollTop + (feedEl.scrollHeight - oldScrollHeight);
	}

	// One-shot: on the first render where the panel is actually visible and
	// measurable, land on the "New messages" divider (a quarter down from the
	// top) if there's unread content, else the bottom. Every render after
	// that just follows stickToBottom.
	let didInitialScroll = $state(false);
	$effect(() => {
		void posts.length;
		if (!feedEl) return;
		if (!didInitialScroll) {
			if (!isOpen || posts.length === 0) return;
			didInitialScroll = true;
			void tick().then(() => {
				if (!feedEl) return;
				const divider = feedEl.querySelector('[data-feed-unread-divider]') as HTMLElement | null;
				if (divider) {
					feedEl.scrollTop = Math.max(0, divider.offsetTop - feedEl.clientHeight * 0.25);
					stickToBottom = false;
					bottomBaselineID = posts[posts.length - 1]?.id ?? 0;
				} else {
					feedEl.scrollTop = feedEl.scrollHeight;
					stickToBottom = true;
				}
			});
			return;
		}
		if (stickToBottom) {
			void tick().then(() => { if (feedEl) feedEl.scrollTop = feedEl.scrollHeight; });
		}
	});

	function onScroll() {
		if (!feedEl) return;
		const near = isNearBottom(feedEl.scrollTop, feedEl.scrollHeight, feedEl.clientHeight, NEAR_BOTTOM_PX);
		stickToBottom = near;
		if (near) {
			bottomBaselineID = null;
		} else if (bottomBaselineID == null) {
			bottomBaselineID = posts[posts.length - 1]?.id ?? 0;
		}
		if (feedEl.scrollTop < 100 && feed.hasMoreBefore && !feed.loadingOlder) {
			void loadOlder();
		}
		scheduleReadReport();
	}

	// ── Read-marker reporting (Phase 2d) ───────────────────────────────────────
	// Mark read when the panel is visible, the tab/window is visible, and the
	// reader is scrolled near the bottom — debounced so a burst of arrivals
	// doesn't fire a request per post. Conservative by design: reading halfway
	// through a long unread run then leaving keeps the marker at the old
	// position (see $lib/chatFeed's reportReadMarker for the history-mode
	// guard — this never advances the marker while browsing a jumped-to
	// window that may be discontiguous with the true unread span).
	let readReportTimer: ReturnType<typeof setTimeout> | null = null;
	function scheduleReadReport() {
		if (feed.mode !== 'live' || !isOpen) return;
		if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return;
		if (!feedEl || !isNearBottom(feedEl.scrollTop, feedEl.scrollHeight, feedEl.clientHeight, NEAR_BOTTOM_PX)) return;
		if (readReportTimer) clearTimeout(readReportTimer);
		readReportTimer = setTimeout(() => { void reportReadMarker(feed); }, 2000);
	}
	function flushReadReport() {
		if (readReportTimer) {
			clearTimeout(readReportTimer);
			readReportTimer = null;
		}
		void reportReadMarker(feed);
	}

	$effect(() => {
		void posts.length;
		void isOpen;
		scheduleReadReport();
	});

	onMount(() => {
		const onVisibility = () => scheduleReadReport();
		document.addEventListener('visibilitychange', onVisibility);
		return () => document.removeEventListener('visibilitychange', onVisibility);
	});

	onDestroy(() => {
		flushReadReport();
	});

	// ── Jump-to-anchor (from Public Record sidebar) ───────────────────────────
	// By the time a jumpRequest arrives, +page.svelte has already resolved the
	// anchor and (if needed) loaded it into `feed` via history mode — this
	// effect only expands the panel (mobile) and scrolls the post into view.
	$effect(() => {
		if (!jumpRequest) return;
		const targetID = jumpRequest.postID;
		// On mobile the panel needs to be expanded to show the scroll target.
		// On desktop the panel is always visible as a column — flipping
		// `expanded` there would only trigger the mobile overlay CSS path.
		// Untracked: we don't want this effect to re-fire when the user
		// closes the panel via the X button (otherwise it re-opens itself).
		untrack(() => {
			if (!expanded && !isDesktop) expanded = true;
		});
		void tick().then(() => {
			if (!feedEl) return;
			const el = feedEl.querySelector(`[data-post-id="${targetID}"]`) as HTMLElement | null;
			if (!el) return;
			el.scrollIntoView({ block: 'center', behavior: 'smooth' });
			// Brief accent pulse so the user sees where they landed. Class
			// is removed after the animation so a re-jump triggers it again.
			el.classList.remove('jump-pulse');
			void el.offsetWidth; // force reflow so the class re-application animates
			el.classList.add('jump-pulse');
			setTimeout(() => el.classList.remove('jump-pulse'), 800);
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
			const speakingAsAssetID =
				selectedPersona && selectedPersona.kind === 'asset'
					? selectedPersona.assetID
					: null;
			await createPlayerPost(gameID, body, { speakingAsAssetID });
			newBody = '';
			// The WS broadcast will append the post to the feed; no optimistic
			// insert needed. Sending your own message should always land you
			// back at the tail, even if you'd scrolled up to read history.
			scrollToBottomNow();
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

<!--
	Dimming scrim behind the expanded mobile sheet. Covers the body region
	(it's absolutely positioned inside the table-body, so the header above it
	stays bright and interactive). Tapping it closes the chat. Hidden on
	desktop, where the panel is a permanent column.
-->
{#if expanded}
	<button type="button" class="scrim" onclick={toggleExpanded} aria-label="Close chat"></button>
{/if}

<aside
	class="panel"
	class:expanded
	aria-label="Chat"
	role={expanded ? 'dialog' : undefined}
	aria-modal={expanded ? 'true' : undefined}
>
	<header class="panel-header">
		<h2>Chat</h2>
		<label class="severity-filter" title="Hide low-severity system events">
			<span class="severity-label">Show:</span>
			<select bind:value={severityThreshold}>
				<option value={SEVERITY.TRACE}>All events</option>
				<option value={SEVERITY.MINOR}>Minor and up</option>
				<option value={SEVERITY.DEFAULT}>Default and up</option>
				<option value={SEVERITY.IMPORTANT}>Important only</option>
				<option value={SEVERITY.BOUNDARY}>Boundaries only</option>
			</select>
		</label>
		<button
			type="button"
			class="collapse"
			onclick={toggleExpanded}
			aria-label="Minimize chat"
		>
			✕
		</button>
	</header>

	<div class="feed-wrap">
		<div class="feed" bind:this={feedEl} onscroll={onScroll}>
			{#if visiblePosts.length === 0}
				<p class="empty">
					{posts.length === 0 ? 'No messages yet. Say something.' : 'No events match the current filter.'}
				</p>
			{:else}
				{#each feedItems as item (item.key)}
					{#if item.kind === 'day-divider'}
						<div class="day-divider" role="separator">
							<span class="day-divider-line"></span>
							<span class="day-divider-label">{item.label}</span>
							<span class="day-divider-line"></span>
						</div>
					{:else if item.kind === 'unread-divider'}
						<div class="unread-divider" data-feed-unread-divider role="separator">
							<span class="unread-divider-line"></span>
							<span class="unread-divider-label">New messages</span>
							<span class="unread-divider-line"></span>
						</div>
					{:else}
						{@const post = item.post}
						{#if post.author_id == null && post.severity >= SEVERITY.BOUNDARY}
							<div class="boundary" data-post-id={post.id} data-code={post.system_code}>
								<span class="boundary-line"></span>
								<!-- eslint-disable-next-line svelte/no-at-html-tags -->
								<span class="boundary-label">{@html renderLogBody(post.body)}</span>
								<span class="boundary-line"></span>
							</div>
						{:else if post.author_id == null}
							<div
								class="log"
								class:important={post.severity >= SEVERITY.IMPORTANT}
								data-post-id={post.id}
								data-code={post.system_code}
							>
								{#if post.severity >= SEVERITY.IMPORTANT}
									<!-- eslint-disable-next-line svelte/no-at-html-tags -->
									<span class="log-body">{@html renderLogBody(post.body)}</span>
								{:else}
									<span class="log-glyph" aria-hidden="true">•</span>
									<!-- eslint-disable-next-line svelte/no-at-html-tags -->
									<span class="log-body">{@html renderLogBody(post.body)}</span>
									<span class="log-time">{fmtTime(post.created_at)}</span>
								{/if}
							</div>
						{:else}
							{@const inCharacter = post.speaking_as_asset_id != null}
							{@const color = playerColorByID(post.author_id, players)}
							{@const personaName = inCharacter
								? assetName(post.speaking_as_asset_id) || playerName(post.author_id)
								: playerName(post.author_id)}
							{@const playerTag = inCharacter ? playerName(post.author_id) : ''}
							<div
								class="message"
								data-post-id={post.id}
								class:own={post.author_id === currentPlayerID}
								style:--player-color={color}
							>
								<span class="msg-author">
									{personaName}
									{#if playerTag && playerTag !== personaName}
										<span class="msg-player-tag">({playerTag})</span>
									{/if}
								</span>
								<span class="msg-body">{post.body}</span>
								<span class="msg-time">{fmtTime(post.created_at)}</span>
							</div>
						{/if}
					{/if}
				{/each}
			{/if}
		</div>

		{#if feed.mode === 'history'}
			<button type="button" class="return-to-now" onclick={handleReturnToNow}>
				⬇ Return to now
			</button>
		{:else if pendingNewCount > 0}
			<button type="button" class="new-pill" onclick={scrollToBottomNow}>
				↓ {pendingNewCount} new
			</button>
		{/if}
	</div>

	<div class="typing" aria-live="polite">{typingLabel}</div>

	{#if error}<p class="error-text error">{error}</p>{/if}

	{#if currentPlayerID != null && personae.length > 1}
		{@const isCharacterSelected = selectedPersona && selectedPersona.kind === 'asset'}
		{@const selfColor = playerColorByID(currentPlayerID, players)}
		{@const personaColor = isCharacterSelected ? selfColor : OOC_COLOR}
		<div class="persona-bar">
			<button
				type="button"
				class="persona-btn"
				class:open={pickerOpen}
				style:--player-color={personaColor}
				onclick={() => { pickerOpen = !pickerOpen; }}
				aria-haspopup="listbox"
				aria-expanded={pickerOpen}
			>
				<span class="persona-dot" aria-hidden="true"></span>
				<span class="persona-label">Speaking as</span>
				<span class="persona-value">{selectedPersona?.label ?? selfName}</span>
				<span class="persona-caret" aria-hidden="true">{pickerOpen ? '▴' : '▾'}</span>
			</button>
			{#if pickerOpen}
				<ul class="persona-menu" role="listbox">
					{#each personae as p (p.kind === 'self' ? 'self' : p.assetID)}
						{@const isSelected =
							(p.kind === 'self' && selectedPersonaID === 'self') ||
							(p.kind === 'asset' && selectedPersonaID === p.assetID)}
						<li>
							<button
								type="button"
								class="persona-option"
								class:selected={isSelected}
								onclick={() => {
									selectedPersonaID = p.kind === 'self' ? 'self' : p.assetID;
									pickerOpen = false;
								}}
								role="option"
								aria-selected={isSelected}
							>
								<span
									class="persona-option-dot"
									style:background={p.kind === 'asset' ? selfColor : OOC_COLOR}
									aria-hidden="true"
								></span>
								<span>{p.label}</span>
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{/if}

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
		/* ≥44px tap target plus breathing room. Defined on .table-page so the
		   page-level padding reservation can use the same value. */
		min-height: var(--chat-strip-height, 56px);
		/* Extra bottom padding accounts for the iOS home-indicator safe area
		   so the preview text isn't clipped on devices with a gesture bar. */
		padding: 0.85rem 1rem calc(0.85rem + env(safe-area-inset-bottom));
		display: flex;
		align-items: center;
		gap: 0.6rem;
		background: #2a2620;
		border-top: 1px solid #6a5a3a;
		color: var(--color-text);
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

	.strip.has-unread { color: var(--color-text); }
	.strip.has-important {
		border-top: 2px solid var(--color-accent);
	}

	.unread-badge {
		background: var(--color-accent);
		color: var(--color-bg);
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

	/* Dimming scrim — only shown on mobile while expanded (rendered
	   conditionally, so it only exists in the DOM then). Absolutely positioned
	   inside .table-body, so it dims the body but leaves the header bright. */
	.scrim {
		position: absolute;
		inset: 0;
		z-index: 109;
		border: none;
		padding: 0;
		background: rgba(0, 0, 0, 0.55);
		cursor: pointer;
		animation: chat-scrim-fade 150ms ease-out;
	}
	@keyframes chat-scrim-fade {
		from { opacity: 0; }
		to { opacity: 1; }
	}

	/* Mobile-only: when expanded, rise as a bottom sheet over the page. A
	   small top gap leaves the dimmed body peeking above, and rounded top
	   corners + shadow read clearly as a floating layer rather than a page
	   swap. Scoped to <1024px so a stray `expanded` on desktop can't burst
	   out of the chat column. */
	@media (max-width: 1023px) {
		.panel.expanded {
			display: flex;
			position: absolute;
			left: 0;
			right: 0;
			bottom: 0;
			/* Gap at the top so the scrim-dimmed body shows above the sheet. */
			top: 0.75rem;
			/* Above the scrim (109) and the Public Record overlay (z-index 100
			   in PublicRecord.svelte) so a jump from the rail opens chat *over*
			   the still-expanded PR. Closing chat returns to the PR underneath. */
			z-index: 110;
			border-top: 1px solid var(--color-surface-2);
			border-radius: 14px 14px 0 0;
			box-shadow: 0 -8px 24px rgba(0, 0, 0, 0.5);
			/* Clip the panel header to the rounded top corners. */
			overflow: hidden;
		}
	}

	.panel-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.5rem 0.8rem;
		border-bottom: 1px solid var(--color-surface-2);
		flex-shrink: 0;
	}

	.panel-header h2 {
		margin: 0;
		font-size: 0.95rem;
		color: var(--color-accent);
	}

	.collapse {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 1.1rem;
		cursor: pointer;
		padding: 0.2rem 0.4rem;
		min-width: 44px;
		min-height: 44px;
	}

	.severity-filter {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.75rem;
		color: var(--color-text-muted);
		margin-left: auto;
		margin-right: 0.4rem;
	}

	.severity-filter select {
		background: var(--color-bg);
		color: var(--color-text);
		border: 1px solid var(--color-border);
		border-radius: 3px;
		padding: 0.2rem 0.3rem;
		font-size: 0.75rem;
		cursor: pointer;
	}

	.severity-label { color: var(--color-text-faint); }

	/* On desktop, hide the collapse button and the strip; the panel is the
	   permanent right column. */
	@media (min-width: 1024px) {
		.strip { display: none; }
		.collapse { display: none; }
		.scrim { display: none; }
		.panel {
			display: flex;
			position: static;
			height: 100%;
			width: 100%;
		}
	}

	/* ── Feed ─────────────────────────────────────────────────────────────── */

	.feed-wrap {
		position: relative;
		flex: 1;
		min-height: 0;
		display: flex;
		flex-direction: column;
	}

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
		color: var(--color-text-faint);
		text-align: center;
		margin-top: 2rem;
		font-size: 0.85rem;
	}

	/* Brief accent flash after a Public-Record jump lands here. */
	:global(.feed [data-post-id].jump-pulse) {
		animation: jump-pulse 0.7s ease-out;
		border-radius: 4px;
	}
	@keyframes jump-pulse {
		0%   { background: rgba(200, 169, 110, 0.45); }
		100% { background: transparent; }
	}

	/* Day divider — a plain centered label, distinct from the accent-colored
	   unread divider so the two don't compete visually. */
	.day-divider {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0.2rem 0;
	}
	.day-divider-line {
		flex: 1;
		height: 1px;
		background: var(--color-border-subtle);
	}
	.day-divider-label {
		font-size: 0.72rem;
		color: var(--color-text-faint);
		text-transform: uppercase;
		letter-spacing: 0.06em;
		white-space: nowrap;
	}

	/* "New messages" divider — accent-colored so it reads as the one thing
	   worth stopping to look at while scrolling past the day dividers. */
	.unread-divider {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0.3rem 0;
	}
	.unread-divider-line {
		flex: 1;
		height: 1px;
		background: var(--color-accent);
		opacity: 0.5;
	}
	.unread-divider-label {
		font-size: 0.72rem;
		color: var(--color-accent);
		text-transform: uppercase;
		letter-spacing: 0.08em;
		white-space: nowrap;
	}

	/* "↓ N new" pill / "Return to now" — pinned to the bottom of the feed,
	   floating above the content. ≥44px tap target (mobile-first). */
	.new-pill, .return-to-now {
		position: absolute;
		left: 50%;
		bottom: 0.6rem;
		transform: translateX(-50%);
		min-height: 44px;
		padding: 0 1rem;
		border-radius: 999px;
		border: 1px solid var(--color-border-strong);
		background: var(--color-accent);
		color: var(--color-bg);
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
		z-index: 5;
		white-space: nowrap;
	}
	.return-to-now {
		background: var(--color-surface-2);
		color: var(--color-accent);
		border-color: var(--color-accent);
	}

	.message {
		display: grid;
		grid-template-columns: auto 1fr auto;
		gap: 0.4rem;
		align-items: baseline;
		border-left: 3px solid var(--player-color, var(--color-accent));
		padding-left: 0.5rem;
	}

	.msg-author {
		color: var(--player-color, var(--color-accent));
		font-size: 0.82rem;
		white-space: nowrap;
	}

	.msg-body {
		font-family: var(--font-serif);
		font-size: 1rem;
		line-height: 1.5;
		white-space: pre-wrap;
		word-break: break-word;
	}

	.msg-time {
		font-size: 0.7rem;
		color: var(--color-text-faint);
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
		background: var(--color-border-warm);
	}
	.boundary-label {
		font-size: 0.78rem;
		color: var(--color-accent);
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
	.log.important {
		display: block;
		text-align: center;
		color: var(--color-accent);
		font-size: 0.85rem;
		margin: 0.35rem 0;
	}
	.log-glyph { color: var(--color-text-muted); }
	.log-time { font-size: 0.7rem; color: var(--color-text-faint); white-space: nowrap; }

	/* ── Typing + compose ─────────────────────────────────────────────────── */

	.typing {
		font-size: 0.78rem;
		color: var(--color-text-faint);
		min-height: 1.2em;
		padding: 0 0.8rem;
		flex-shrink: 0;
	}

	.error {
		padding: 0 0.8rem;
		flex-shrink: 0;
	}

	.compose {
		display: flex;
		gap: 0.5rem;
		padding: 0.5rem 0.8rem 0.7rem;
		border-top: 1px solid var(--color-surface-2);
		align-items: flex-end;
		flex-shrink: 0;
	}

	textarea {
		flex: 1;
		font-size: 0.9rem;
		padding: 0.5rem 0.7rem;
		border-radius: 6px;
		border: 1px solid var(--color-border-strong);
		background: var(--color-surface-2);
		color: inherit;
		font-family: inherit;
		resize: none;
		line-height: 1.4;
		min-height: 44px;
	}

	textarea:focus {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}

	.send {
		background: var(--color-accent);
		color: var(--color-bg);
		padding: 0.5rem 0.9rem;
		min-width: 56px;
		min-height: 44px;
		align-self: flex-end;
		border-radius: 6px;
		border: none;
		cursor: pointer;
	}

	.send:disabled { opacity: 0.4; cursor: not-allowed; }

	/* ── Player tag in messages ──────────────────────────────────────────── */
	.msg-player-tag {
		font-weight: 400;
		color: var(--color-text-faint);
		font-size: 0.72rem;
		margin-left: 0.25rem;
	}

	/* ── Persona picker ──────────────────────────────────────────────────── */

	.persona-bar {
		position: relative;
		padding: 0.4rem 0.8rem 0;
		flex-shrink: 0;
	}

	.persona-btn {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		min-height: 36px;
		padding: 0.3rem 0.6rem;
		border: 1px solid var(--color-surface-2);
		border-left: 3px solid var(--player-color, var(--color-accent));
		background: var(--color-surface-sunken);
		color: #d8d4c9;
		border-radius: 5px;
		cursor: pointer;
		font-size: 0.85rem;
		width: 100%;
	}

	.persona-btn:hover { border-color: var(--player-color, var(--color-accent)); }

	.persona-btn.open { background: #221d10; }

	.persona-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--player-color, var(--color-accent));
		flex-shrink: 0;
	}

	.persona-label {
		font-size: 0.72rem;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.persona-value {
		flex: 1;
		color: var(--player-color, var(--color-accent));
		text-align: left;
	}

	.persona-caret { color: var(--color-text-muted); font-size: 0.78rem; }

	.persona-menu {
		position: absolute;
		left: 0.8rem;
		right: 0.8rem;
		bottom: calc(100% + 0.2rem);
		margin: 0;
		padding: 0.25rem;
		list-style: none;
		background: var(--color-bg);
		border: 1px solid var(--color-border-strong);
		border-radius: 5px;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
		z-index: 60;
	}

	.persona-option {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		min-height: 40px;
		padding: 0.4rem 0.6rem;
		border: none;
		background: none;
		color: #d8d4c9;
		font-size: 0.88rem;
		text-align: left;
		cursor: pointer;
		border-radius: 4px;
	}
	.persona-option:hover { background: var(--color-surface); }
	.persona-option.selected { background: #2e2510; color: #e8d8a0; }

	.persona-option-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}
</style>
