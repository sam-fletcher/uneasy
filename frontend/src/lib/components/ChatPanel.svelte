<!--
	ChatPanel.svelte — the unified game-wide chat surface.

	Layout: the panel fills the column the page gives it (docs/STYLE_GUIDE.md
	"Layout widths"). On a phone that column is one pane among three, shown when
	its tab is chosen; from two columns up it is docked on the right. The page
	owns which, and says whether the pane is on screen through `visible`. The
	keyboard only appears when the user taps the input box, never on arrival
	(matches Discord/Slack/iMessage).

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
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import '$lib/components/shared/jumpPulse.css';
	import { onMount, onDestroy, tick, untrack } from 'svelte';
	import {
		createPlayerPost,
		type Asset,
		type ChatPost,
		type Plan,
		type PlanType,
		type Player,
		type Scene,
		type ScenePeerView,
	} from '$lib/api';
	import { controlledPeers } from '$lib/controlledPeers';
	import { renderLogBody } from '$lib/logMarkup';
	import { playerColorByID } from '$lib/playerColor';
	import { scrollBehavior } from '$lib/reducedMotion';
	import { SEVERITY } from '$lib/severity';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import {
		type ChatFeedContext,
		type FeedItem,
		type SceneGroupItem,
		type PlanGroupItem,
		type PlanOutcome,
		buildFeedItems,
		countUnread,
		isNearBottom,
		fetchOlderPage,
		markForCode,
		reportReadMarker,
		returnToNow,
		parseSceneStartedData,
	} from '$lib/chatFeed';
	import {
		planHeaderName,
		planMarkFamily,
		resolutionStatus,
		unreadDividerAria,
		unreadDividerLabel,
	} from '$lib/chatView';
	import LogMark from '$lib/components/LogMark.svelte';
	import FeedDivider from '$lib/components/chat/FeedDivider.svelte';
	import SpanHeader from '$lib/components/chat/SpanHeader.svelte';
	import BarbuteMark from '$lib/components/shared/BarbuteMark.svelte';
	import Chip from '$lib/components/shared/Chip.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';
	import RowMark from '$lib/components/shared/RowMark.svelte';
	import ToneMark from '$lib/components/shared/ToneMark.svelte';
	import PaneName from '$lib/components/table/PaneName.svelte';

	// System-log bodies carry a tiny server-written markup subset — **asset
	// names** and @@id|player names@@ — parsed in $lib/logMarkup (which also
	// documents the escaping order and why each delimiter is doubled). Player
	// chat messages do NOT pass through it; their asterisks show verbatim.
	// Bound here to the roster so the template can call it with one argument,
	// and so `players` is read during render and stays reactive.
	function renderBody(body: string): string {
		return renderLogBody(body, players);
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
		 * The table's plans. A plan's header row in the feed shows who prepared it
		 * (their shape) and, for war, wears war's mark; both come from here, by the
		 * plan id the span carries. Without it the header still names the plan and
		 * its status, from the posts.
		 */
		plans?: Plan[];
		/**
		 * Jump request from the Public Record. When this changes to a non-null
		 * value, the panel scrolls to the post with the given ID. The `key`
		 * field disambiguates repeated requests for the same post so the effect
		 * re-runs. By the time this fires, the caller (+page.svelte) has already
		 * ensured the post is loaded into `feed` — resolving the anchor and
		 * entering history mode if it wasn't in the live window — and, on a
		 * phone, switched to this pane.
		 */
		jumpRequest?: { postID: number; key: number } | null;
		/**
		 * Whether this pane is on screen. A phone shows one pane at a time, so
		 * the others are laid out but hidden; the chat must not count messages as
		 * seen, or land on the unread divider, until the reader can see it.
		 */
		visible?: boolean;
		/** Start the header with the pane's name (from three columns up, where no
		 *  tab names it). */
		showName?: boolean;
		/** "Tones · N set" in the header opens the Tones sheet, which the page owns. */
		tonesSetCount?: number;
		onOpenTones?: () => void;
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
		plans = [],
		jumpRequest = null,
		visible = true,
		showName = false,
		tonesSetCount = 0,
		onOpenTones = () => {},
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
		| { kind: 'asset'; assetID: number; ownerID: number; label: string };

	const selfName = $derived(playerName(currentPlayerID) || 'You');

	// Only the focus player's main character is implicitly present in a
	// turn-scene; everyone else's MC appears in activeScenePeers if the focus
	// player added it. A plan-scene has no implicit-MC shortcut — the
	// preparer's main character is an explicit scene_peers row like every
	// other participant's (see PlanSceneStager in the backend), so it already
	// surfaces via myControlledPeers below without this clause.
	const isFocusPlayer = $derived(
		activeScene != null &&
			activeScene.kind === 'turn' &&
			currentPlayerID != null &&
			activeScene.focus_player_id === currentPlayerID
	);

	const ownMainCharacter = $derived(
		currentPlayerID == null
			? null
			: assets.find(a => a.is_main_character && a.owner_id === currentPlayerID && !a.is_destroyed) ?? null
	);

	const myControlledPeers = $derived(
		activeScene == null ? [] : controlledPeers(activeScenePeers, assets, currentPlayerID)
	);

	const personae = $derived.by<Persona[]>(() => {
		const list: Persona[] = [];
		// Character personae are available only while a scene is active.
		if (activeScene != null) {
			if (isFocusPlayer && ownMainCharacter) {
				list.push({
					kind: 'asset',
					assetID: ownMainCharacter.id,
					ownerID: ownMainCharacter.owner_id,
					label: ownMainCharacter.name,
				});
			}
			for (const peer of myControlledPeers) {
				// Avoid duplicating the main character if it's also in the peer list.
				if (peer.id === ownMainCharacter?.id) continue;
				list.push({ kind: 'asset', assetID: peer.id, ownerID: peer.owner_id, label: peer.name });
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

	// ── Visibility ─────────────────────────────────────────────────────────────
	// Hiding the pane (choosing another tab) flushes any pending read-marker
	// report, as closing the old mobile sheet did — see "Read-marker reporting"
	// below. Starting hidden reports nothing: messages stay unread until seen.
	let wasVisible = untrack(() => visible);
	$effect(() => {
		const now = visible;
		untrack(() => {
			if (wasVisible && !now) flushReadReport();
			wasVisible = now;
		});
	});

	// ── Unread state (server-side marker; adr/CHAT_OVERHAUL_PLAN.md Phase 1c/2a) ─
	const unreadCount = $derived(countUnread(posts, feed.lastReadPostID, currentPlayerID));

	// ── "Hide bookkeeping" toggle (Phase 3) ────────────────────────────────────
	// Player messages (author_id != null) are always shown; the toggle only
	// affects system posts, hiding everything below SEVERITY.DEFAULT. Persisted
	// account-wide (not per-game) in localStorage; defaults to hidden.
	const HIDE_BOOKKEEPING_KEY = 'uneasy.chat.hideBookkeeping';
	function loadHideBookkeeping(): boolean {
		if (typeof localStorage === 'undefined') return true;
		return localStorage.getItem(HIDE_BOOKKEEPING_KEY) !== 'false';
	}
	let hideBookkeeping = $state(loadHideBookkeeping());
	function toggleHideBookkeeping() {
		hideBookkeeping = !hideBookkeeping;
		if (typeof localStorage !== 'undefined') {
			localStorage.setItem(HIDE_BOOKKEEPING_KEY, String(hideBookkeeping));
		}
	}
	const severityThreshold = $derived(hideBookkeeping ? SEVERITY.DEFAULT : SEVERITY.TRACE);

	const visiblePosts = $derived(
		posts.filter(p => p.author_id != null || p.severity >= severityThreshold)
	);

	// The unread divider says how many are new ("4 new"). The count lives here
	// rather than on the tab: it is counted from the same frozen marker the divider
	// sits at, so the figure and the line agree even as the live marker moves.
	const unreadSinceDivider = $derived(
		countUnread(visiblePosts, feed.initialReadMarker, currentPlayerID)
	);

	// A plan's header row names who prepared it and, for war, wears war's mark;
	// both come from the plan list, by the id the span carries. Null while the list
	// hasn't loaded (or the plan isn't in it): the header then falls back to what
	// the posts say.
	function planInfoFor(planID: number | null): { planType: PlanType | null; preparer: Player | null } {
		const plan = planID == null ? undefined : plans.find(p => p.id === planID);
		if (!plan) return { planType: null, preparer: null };
		return { planType: plan.plan_type, preparer: players.find(p => p.id === plan.preparer_id) ?? null };
	}
	// Whose shape stands beside a message's byline: the speaker's, or, for a
	// character, its owning retinue's (a character always wears its owner's colour).
	const playerByID = (id: number | null) =>
		id == null ? null : players.find(p => p.id === id) ?? null;

	// Render list: day dividers + the "New messages" divider (Phase 2a).
	// `feed.initialReadMarker` is a snapshot taken when the window was last
	// (re)loaded from scratch, so the divider holds still while the live
	// marker (`feed.lastReadPostID`) advances underneath it.
	const feedItems = $derived(
		buildFeedItems(visiblePosts, { unreadAfterID: feed.initialReadMarker, currentPlayerID })
	);

	// ── Scene containers (Phase 4c) ────────────────────────────────────────────
	// Expansion is in-memory only (per session, per the plan) — a plain map of
	// explicit user overrides, keyed by scene id. Absent an override, a group
	// defaults to expanded while open (still active, or the window truncated
	// before its close) or while the unread divider falls inside it, and
	// collapsed once ended and fully read. The game's actual live scene (from
	// `activeScene`, not just "this group has no endPost yet") is always
	// expanded and cannot be collapsed — a truncated-but-really-already-ended
	// group the user hasn't loaded the close for is not locked, just defaulted.
	let sceneExpandOverrides = $state<Record<number, boolean>>({});

	function isSceneLive(group: SceneGroupItem): boolean {
		return activeScene != null && group.sceneID === activeScene.id;
	}

	function isSceneExpanded(group: SceneGroupItem): boolean {
		if (isSceneLive(group)) return true;
		const override = sceneExpandOverrides[group.sceneID];
		if (override != null) return override;
		return group.open || group.unreadDividerInside;
	}

	function toggleSceneExpanded(group: SceneGroupItem) {
		if (isSceneLive(group)) return;
		sceneExpandOverrides = { ...sceneExpandOverrides, [group.sceneID]: !isSceneExpanded(group) };
	}

	// ── Plan-resolution containers (hierarchy-plan S3) ────────────────────────
	// The plain (no-scene) rendering of a plan-resolution span collapses on
	// exactly the scene rule above, minus the isSceneLive lock: expanded while
	// the span is still open (no terminal post loaded — still resolving, or the
	// window truncated) or while the unread divider sits inside it, collapsed
	// once resolved and read, and an explicit tap always wins. The header and
	// the outcome footer stay visible either way, so a collapsed card still
	// says which plan resolved and how.
	let planExpandOverrides = $state<Record<number, boolean>>({});

	function isPlanExpanded(group: PlanGroupItem): boolean {
		const override = planExpandOverrides[group.planID];
		if (override != null) return override;
		return group.open || group.unreadDividerInside;
	}

	function togglePlanExpanded(group: PlanGroupItem) {
		planExpandOverrides = { ...planExpandOverrides, [group.planID]: !isPlanExpanded(group) };
	}

	// Recursively finds the containers whose *collapsed* contents would hide the
	// given post id — the whole outermost→innermost path, since expanding only
	// one of a nested pair still leaves the target hidden. A post that IS a
	// container's own header/footer (a scene's start post, a plan span's
	// resolving or outcome post) renders whatever the expansion state, so it
	// contributes no entry.
	type ContainerItem = SceneGroupItem | PlanGroupItem;
	function findCollapsingContainers(items: FeedItem[], postID: number): ContainerItem[] {
		for (const item of items) {
			if (item.kind !== 'scene-group' && item.kind !== 'plan-group') continue;
			if (item.kind === 'scene-group' && item.startPost?.id === postID) return [];
			if (item.resolvingPost?.id === postID || item.outcomePost?.id === postID) return [];
			if (containsPostID(item.items, postID)) {
				return [item, ...findCollapsingContainers(item.items, postID)];
			}
		}
		return [];
	}
	function containsPostID(items: FeedItem[], postID: number): boolean {
		return items.some((item) => {
			if (item.kind === 'post') return item.post.id === postID;
			if (item.kind === 'ranking-group') return item.posts.some((p) => p.id === postID);
			if (item.kind === 'scene-group' || item.kind === 'plan-group') {
				return containsPostID(item.items, postID);
			}
			return false;
		});
	}
	function isContainerExpanded(item: ContainerItem): boolean {
		return item.kind === 'scene-group' ? isSceneExpanded(item) : isPlanExpanded(item);
	}
	function forceExpandContainer(item: ContainerItem) {
		if (item.kind === 'scene-group') {
			sceneExpandOverrides = { ...sceneExpandOverrides, [item.sceneID]: true };
		} else {
			planExpandOverrides = { ...planExpandOverrides, [item.planID]: true };
		}
	}

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
			if (!visible || posts.length === 0) return;
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
		if (feed.mode !== 'live' || !visible) return;
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
		void visible;
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

	// ── Jump-to-anchor (from the Public Record) ───────────────────────────────
	// By the time a jumpRequest arrives, +page.svelte has already resolved the
	// anchor and (if needed) loaded it into `feed` via history mode — this
	// effect only scrolls the post into view.
	$effect(() => {
		if (!jumpRequest) return;
		const targetID = jumpRequest.postID;
		untrack(() => {
			// A collapsed scene/plan container hides its inner posts entirely
			// (not just visually) — expand them first so the target actually
			// exists in the DOM by the time we query for it below.
			for (const collapsing of findCollapsingContainers(feedItems, targetID)) {
				if (!isContainerExpanded(collapsing)) forceExpandContainer(collapsing);
			}
		});
		void tick().then(() => {
			if (!feedEl) return;
			const el = feedEl.querySelector(`[data-post-id="${targetID}"]`) as HTMLElement | null;
			if (!el) return;
			// The jump still takes the reader there under reduced motion — it
			// just arrives instead of travelling (lib/reducedMotion.ts). The
			// flash below is guarded in shared/jumpPulse.css.
			el.scrollIntoView({ block: 'center', behavior: scrollBehavior() });
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
		// Clear the box up front rather than after the await. The compose
		// textarea is deliberately never `disabled` (see the markup below), so
		// the user can keep typing while the POST is in flight; clearing on
		// success would eat whatever they'd typed in the meantime. On failure
		// we put the text back, but only if they haven't started a new message.
		newBody = '';
		try {
			const speakingAsAssetID =
				selectedPersona && selectedPersona.kind === 'asset'
					? selectedPersona.assetID
					: null;
			await createPlayerPost(gameID, body, { speakingAsAssetID });
			// The WS broadcast will append the post to the feed; no optimistic
			// insert needed. Sending your own message should always land you
			// back at the tail, even if you'd scrolled up to read history.
			scrollToBottomNow();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not send.';
			if (!newBody) newBody = body;
			else error += ' Unsent message: ' + body;
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

	function fmtFullTime(iso: string) {
		return new Date(iso).toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' });
	}

	// Timestamp diet (adr/CHAT_VISUAL_HIERARCHY_PLAN.md S1): per-line times
	// are hidden until hover (pointer devices) or tap. One revealed row at a
	// time, so taps don't accrete clutter down the feed.
	let revealedTimeID = $state<number | null>(null);
	function toggleTimeReveal(id: number) {
		revealedTimeID = revealedTimeID === id ? null : id;
	}
</script>

<!--
	The panel fills its column. The page wraps it in the Chat column and gives
	that column's name, tab and sheet; this renders only the feed, the header's
	two controls and the composer.
-->

<div class="panel">
	<header class="panel-header">
		{#if showName}
			<PaneName pane="chat" marked={unreadCount > 0} />
		{/if}
		<!-- Two controls and, from three columns up, the column's name before them.
		     On a phone there is no heading: the tab already says "Chat". Tones are
		     reference for what is said, so they sit where it is said. The chip's
		     wording is an open question (docs/work/questions/tones-chip-wording.md). -->
		<Chip compact aria-haspopup="dialog" onclick={onOpenTones}>Tones · {tonesSetCount} set</Chip>
		<label class="bookkeeping-toggle" title="Hide low-severity system events">
			<span>Hide bookkeeping</span>
			<input
				type="checkbox"
				role="switch"
				checked={hideBookkeeping}
				onchange={toggleHideBookkeeping}
			/>
		</label>
	</header>

	<!--
		A plan resolution's terminal post, shared by both renderings of the span (the
		plain one and the plan-scene it folds into). It is an ordinary line under the
		span's rule, and it stays visible when the span is folded, so a shut span
		still says how the plan landed and what comes next. How it landed is the
		header's status word (Make, Mar, Fell through); this line is the sentence
		behind it, so it does not say the word a second time.
	-->
	{#snippet planOutcome(post: ChatPost)}
		<div
			class="plan-outcome"
			data-post-id={post.id}
			data-code={post.system_code}
			title={fmtFullTime(post.created_at)}
		>
			<!-- eslint-disable-next-line svelte/no-at-html-tags -->
			{@html renderBody(post.body)}
		</div>
	{/snippet}

	<!--
		One log line: a family mark, then the words. War's lines wear the barbute in
		war red in the mark's place (a war posts a line for every player on every row,
		so they are worth picking out); every other family is one of LogMark's.
	-->
	{#snippet logMark(family: string | null)}
		<span class="log-mark" aria-hidden="true">
			{#if family === 'war'}
				<BarbuteMark />
			{:else}
				<LogMark {family} />
			{/if}
		</span>
	{/snippet}

	<!--
		Recursive so a scene-group's inner items (Phase 4) render through the
		exact same day-divider/unread-divider/ranking-group/post logic as the
		top level — single chronology, just wrapped in a container. `inScene`
		is true only for items rendered inside a scene-group's body, and
		drives the in-character/table-talk message register (Phase 4d).
	-->
	{#snippet feedEntry(item: FeedItem, inScene: boolean)}
		{#if item.kind === 'day-divider'}
			<FeedDivider kind="day">{item.label}</FeedDivider>
		{:else if item.kind === 'unread-divider'}
			<FeedDivider
				kind="unread"
				data-feed-unread-divider
				aria-label={unreadDividerAria(unreadSinceDivider)}
			>
				{unreadDividerLabel(unreadSinceDivider)}
			</FeedDivider>
		{:else if item.kind === 'ranking-group'}
			<div
				class="ranking-group"
				class:continues-run={item.continuesRun}
				data-post-id={item.posts[0].id}
				data-code={item.posts[0].system_code}
			>
				{#each item.posts as post (post.id)}
					{#if post.system_code === 'ranking.category'}
						<h4 class="ranking-category" data-post-id={post.id}>
							<!-- eslint-disable-next-line svelte/no-at-html-tags -->
							{@html renderBody(post.body)}
						</h4>
					{:else if post.system_code === 'ranking.standing'}
						<div class="ranking-standing" data-post-id={post.id}>
							<!-- eslint-disable-next-line svelte/no-at-html-tags -->
							{@html renderBody(post.body)}
						</div>
					{:else if post.system_code === 'ranking.updated'}
						<!-- The group's own mark slot (log-marks S2). This post is
						     SeverityBoundary but never reaches the boundary branch below
						     — every ranking.* post collapses in here — so the headline is
						     where the podium goes. -->
						<div class="ranking-headline" data-post-id={post.id}>
							{@render logMark('ranking')}
							<!-- eslint-disable-next-line svelte/no-at-html-tags -->
							<span>{@html renderBody(post.body)}</span>
							<span class="log-time">{fmtTime(post.created_at)}</span>
						</div>
					{:else}
						<div class="ranking-line" data-post-id={post.id}>
							<!-- eslint-disable-next-line svelte/no-at-html-tags -->
							{@html renderBody(post.body)}
						</div>
					{/if}
				{/each}
			</div>
		{:else if item.kind === 'scene-group'}
			{@const data = item.startPost ? parseSceneStartedData(item.startPost.system_data) : null}
			{@const focusColor = data ? playerColorByID(data.focus_player_id, players) : undefined}
			{@const live = isSceneLive(item)}
			{@const isExpanded = isSceneExpanded(item)}
			{@const plan = item.resolvingPost ? planInfoFor(item.planID) : null}
			{@const banner = item.startPost ? item.startPost.body : 'Scene in progress (earlier posts not loaded)'}
			<div class="scene-group span" class:plan-scene={item.resolvingPost != null}>
				<!-- A plan-scene folds its resolution span in rather than nesting two
				     containers (hierarchy-plan S3): the header is the plan's (its mark,
				     name, preparer, status) and the scene's own banner becomes the first
				     line under the rule. The four plan types that stage a scene arrive
				     here; every other plan's resolution is the plain span below. -->
				<SpanHeader
					class="scene-header"
					data-post-id={item.startPost?.id}
					data-scene-id={item.sceneID}
					family={plan ? planMarkFamily(plan.planType) : 'scene'}
					markColor={plan ? undefined : focusColor}
					title={plan && item.resolvingPost
						? planHeaderName(plan.planType, item.resolvingPost.body)
						: banner}
					preparer={plan?.preparer}
					status={plan
						? resolutionStatus(item.outcome, plan.planType)
						: live
							? { word: 'Live', tone: 'neutral' }
							: null}
					expanded={isExpanded}
					locked={live}
					unread={item.unreadCount > 0}
					onclick={() => toggleSceneExpanded(item)}
				/>
				{#if plan || data?.prompt || (data && data.participants.length > 0) || isExpanded || item.outcomePost}
					<div class="span-body">
						{#if plan && item.resolvingPost}
							<div
								class="scene-banner"
								data-post-id={item.resolvingPost.id}
								data-code={item.resolvingPost.system_code}
							>
								<span class="scene-banner-mark" style:color={focusColor} aria-hidden="true">
									<LogMark family="scene" />
								</span>
								<span>{banner}</span>
							</div>
						{/if}
						{#if data?.prompt}
							<p class="scene-prompt">{data.prompt}</p>
						{/if}
						{#if data && data.participants.length > 0}
							<p class="scene-participants">With {data.participants.join(', ')}</p>
						{/if}
						{#if isExpanded}
							{#each item.items as inner (inner.key)}
								{@render feedEntry(inner, true)}
							{/each}
						{/if}
						{#if item.outcomePost}
							<!-- Always shown, folded or not: chronologically it precedes the
							     "the scene ends" line (EmitPlanResolved writes it before
							     closePlanSceneIfAny), but it reads as the span's result. -->
							{@render planOutcome(item.outcomePost)}
						{/if}
					</div>
				{/if}
			</div>
		{:else if item.kind === 'plan-group'}
			{@const isExpanded = isPlanExpanded(item)}
			{@const plan = planInfoFor(item.planID)}
			<div class="plan-group span">
				<!-- Unlike a live scene (which the server confirms is still running, and
				     so locks open), an open span may equally mean the window was truncated
				     before the terminal post — so it stays foldable. It carries a caret
				     only when there is something inside to fold; the outcome line sits
				     outside the fold. -->
				<SpanHeader
					class="scene-header plan-header"
					data-post-id={item.resolvingPost.id}
					data-code={item.resolvingPost.system_code}
					data-plan-id={item.planID}
					family={planMarkFamily(plan.planType)}
					title={planHeaderName(plan.planType, item.resolvingPost.body)}
					preparer={plan.preparer}
					status={resolutionStatus(item.outcome, plan.planType)}
					expanded={isExpanded}
					caret={item.items.length > 0}
					unread={item.unreadCount > 0}
					onclick={() => togglePlanExpanded(item)}
				/>
				{#if (isExpanded && item.items.length > 0) || item.outcomePost}
					<div class="span-body">
						{#if isExpanded}
							{#each item.items as inner (inner.key)}
								{@render feedEntry(inner, inScene)}
							{/each}
						{/if}
						{#if item.outcomePost}
							{@render planOutcome(item.outcomePost)}
						{/if}
					</div>
				{/if}
			</div>
		{:else}
			{@const post = item.post}
			{#if post.author_id == null && post.severity >= SEVERITY.BOUNDARY}
				<!-- A row beginning (and the other structural boundaries: rankings,
				     the Shake-Up's banners) is the deep gold weight of divider: the
				     game's own colour. The row number is already in the body text, so
				     the old filled number pill is gone. -->
				<FeedDivider kind="row" data-post-id={post.id} data-code={post.system_code}>
					<!-- eslint-disable-next-line svelte/no-at-html-tags -->
					{@html renderBody(post.body)}
				</FeedDivider>
			{:else if post.author_id == null}
				<!-- Tap-to-reveal the timestamp; supplementary metadata only (the
				     full date also lives in `title`), so the row stays a plain div. -->
				<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
				<div
					class="log"
					class:important={post.severity >= SEVERITY.IMPORTANT}
					class:minor={post.severity < SEVERITY.DEFAULT}
					class:continues-run={item.continuesRun}
					class:show-time={revealedTimeID === post.id}
					data-post-id={post.id}
					data-code={post.system_code}
					title={fmtFullTime(post.created_at)}
					onclick={() => toggleTimeReveal(post.id)}
				>
					{@render logMark(markForCode(post.system_code))}
					<!-- eslint-disable-next-line svelte/no-at-html-tags -->
					<span class="log-body">{@html renderBody(post.body)}</span>
					<span class="log-time">{fmtTime(post.created_at)}</span>
				</div>
			{:else}
				{@const inCharacter = post.speaking_as_asset_id != null}
				{@const speakingAsset = inCharacter
					? assets.find(a => a.id === post.speaking_as_asset_id)
					: undefined}
				{@const color = speakingAsset
					? playerColorByID(speakingAsset.owner_id, players)
					: playerColorByID(post.author_id, players)}
				{@const personaName = inCharacter
					? speakingAsset?.name || playerName(post.author_id)
					: playerName(post.author_id)}
				{@const borrowed = speakingAsset != null && speakingAsset.owner_id !== post.author_id}
				{@const shapeOf = playerByID(speakingAsset ? speakingAsset.owner_id : post.author_id)}
				<!-- A character always wears its OWNING retinue's colour, no
				     matter who is roleplaying it right now (owner ruling,
				     2026-07-19); only the byline tag names the puppeteer, and
				     only when they're borrowing a peer they don't own. Posts as
				     oneself keep the author's own colour. The shape beside the
				     name is that same player's, so the byline reads without
				     colour. -->
				<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
				<div
					class="message"
					data-post-id={post.id}
					class:own={post.author_id === currentPlayerID}
					class:in-character={inScene && inCharacter}
					class:table-talk={inScene && !inCharacter}
					class:show-time={revealedTimeID === post.id}
					style:--player-color={color}
					title={fmtFullTime(post.created_at)}
					onclick={() => toggleTimeReveal(post.id)}
				>
					<span class="msg-byline">
						{#if shapeOf}<PlayerShape player={shapeOf} />{/if}
						<span class="msg-author">{personaName}</span>
						{#if borrowed}
							<span class="msg-player-tag">({playerName(post.author_id)})</span>
						{/if}
						<span class="msg-time">{fmtTime(post.created_at)}</span>
					</span>
					<span class="msg-body">{post.body}</span>
				</div>
			{/if}
		{/if}
	{/snippet}

	<div class="feed-wrap">
		<div class="feed" bind:this={feedEl} onscroll={onScroll}>
			{#if visiblePosts.length === 0}
				<p class="empty">
					{posts.length === 0 ? 'No messages yet. Say something.' : 'No events match the current filter.'}
				</p>
			{:else}
				{#each feedItems as item (item.key)}
					{@render feedEntry(item, false)}
				{/each}
			{/if}
		</div>

		<!-- A chip floating over the foot of the feed. Words only: the arrows that
		     used to lead these were typed characters, which each platform draws from
		     a different font. -->
		{#if feed.mode === 'history'}
			<div class="feed-jump">
				<Chip compact class="return-to-now" onclick={handleReturnToNow}>Return to now</Chip>
			</div>
		{:else if pendingNewCount > 0}
			<div class="feed-jump">
				<Chip compact class="new-pill" onclick={scrollToBottomNow}>{pendingNewCount} new below</Chip>
			</div>
		{/if}
	</div>

	<div class="typing" aria-live="polite">{typingLabel}</div>

	{#if error}<ErrorText message={error} extra="error" />{/if}

	<!-- Focus must survive a send, so you can fire off several messages in a
	     row the way you would in Discord. Two rules keep it there:
	     1. The textarea is never `disabled` — disabling a focused element
	        blurs it, and on mobile that blur closes the soft keyboard, which
	        a later programmatic .focus() can't reopen (it's past the user
	        gesture). Double-submit is already guarded inside send().
	     2. The Send button swallows mousedown's default, so clicking it never
	        moves focus off the textarea — otherwise the button ends up focused,
	        then goes disabled as the box empties, and focus falls to <body>.
	     The keyboard still opens only when the box is tapped, never on arrival
	     (docs/STYLE_GUIDE.md; the chat design ruling): nothing here calls focus(). -->
	<div class="compose">
		{#if currentPlayerID != null && personae.length > 1}
			{@const selfPlayer = playerByID(currentPlayerID)}
			{@const selfColor = playerColorByID(currentPlayerID, players)}
			<!-- A character persona shows its owning retinue's colour (muted, the
			     mask register), which for a borrowed peer is NOT the speaker's
			     own; "self" keeps the speaker's vivid colour. Shown only when there
			     is a choice to make: outside a scene you simply speak as yourself. -->
			{@const personaOwner = selectedPersona?.kind === 'asset'
				? playerByID(selectedPersona.ownerID)
				: selfPlayer}
			{@const personaColor = selectedPersona?.kind === 'asset'
				? playerColorByID(selectedPersona.ownerID, players)
				: selfColor}
			<div class="persona-bar">
				<button
					type="button"
					class="persona-btn"
					class:character={selectedPersona?.kind === 'asset'}
					style:--player-color={personaColor}
					onclick={() => { pickerOpen = !pickerOpen; }}
					aria-haspopup="listbox"
					aria-expanded={pickerOpen}
				>
					<span class="persona-label">Speaking as</span>
					{#if personaOwner}<PlayerShape player={personaOwner} />{/if}
					<span class="persona-value">{selectedPersona?.label ?? selfName}</span>
					<RowMark kind="caret" open={pickerOpen} />
				</button>
				{#if pickerOpen}
					<ul class="persona-menu" role="listbox">
						{#each personae as p (p.kind === 'self' ? 'self' : p.assetID)}
							{@const isSelected =
								(p.kind === 'self' && selectedPersonaID === 'self') ||
								(p.kind === 'asset' && selectedPersonaID === p.assetID)}
							{@const optionOwner = p.kind === 'asset' ? playerByID(p.ownerID) : selfPlayer}
							<li>
								<button
									type="button"
									class="persona-option"
									class:selected={isSelected}
									class:character={p.kind === 'asset'}
									style:--player-color={p.kind === 'asset'
										? playerColorByID(p.ownerID, players)
										: selfColor}
									onclick={() => {
										selectedPersonaID = p.kind === 'self' ? 'self' : p.assetID;
										pickerOpen = false;
									}}
									role="option"
									aria-selected={isSelected}
								>
									{#if optionOwner}<PlayerShape player={optionOwner} />{/if}
									<span class="persona-option-name">{p.label}</span>
									{#if isSelected}<span class="persona-tick"><ToneMark tone="success" /></span>{/if}
								</button>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		{/if}

		<div class="compose-row">
			<textarea
				placeholder="Write a message…"
				aria-label="Message"
				bind:value={newBody}
				oninput={onInput}
				onkeydown={onKeydown}
				rows={2}
				maxlength={TEXT_LIMITS.LONG_TEXT}
			></textarea>
			<!-- Greyed out (the house button's disabled look) until there is something
			     to send: it says "nothing to do yet" without spending the main
			     button's deep gold. -->
			<button
				type="button"
				class="action-btn primary send"
				onmousedown={(e) => e.preventDefault()}
				onclick={send}
				disabled={sending || !newBody.trim()}
			>
				{sending ? '…' : 'Send'}
			</button>
		</div>
	</div>
</div>

<style>
	/* ── Panel and header ─────────────────────────────────────────────────── */

	/* Fills the Chat column. The column is drawn at phone width, so there is no
	   overlay, no strip and no scrim: the pane is either on screen or not. */
	.panel {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-height: 0;
		background: var(--color-bg);
		color: var(--color-text-secondary);
	}

	/* One line: the column's name (from three columns up), the Tones chip,
	   and the switch pushed to the far end. 52px tall like the other two
	   columns' name bars, so the three line up. */
	.panel-header {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: var(--layout-column-head);
		padding: 0 var(--space-2);
		border-bottom: 1px solid var(--color-border);
		flex-shrink: 0;
	}

	/* "Hide bookkeeping" is a switch, not a checkbox: it is on or off, and it acts
	   at once. The whole label is the 44px tap target. The input is the switch
	   itself (appearance: none) so there is one control, one focus ring, and a
	   screen reader hears "Hide bookkeeping, switch, on". */
	.bookkeeping-toggle {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: var(--size-tap);
		margin-left: auto;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
		cursor: pointer;
	}

	.bookkeeping-toggle input {
		appearance: none;
		position: relative;
		flex: none;
		width: 40px;
		height: 24px;
		margin: 0;
		box-sizing: border-box;
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-chip);
		background: var(--color-surface);
		cursor: pointer;
	}
	.bookkeeping-toggle input::before {
		content: '';
		position: absolute;
		top: 3px;
		left: 3px;
		width: 16px;
		height: 16px;
		border-radius: var(--radius-chip);
		background: var(--color-text-secondary);
		transition: transform 0.15s ease;
	}
	/* On is a pick, so it wears the pick's mark: the bright gold border (the second
	   pixel an inset shadow, so the switch never changes size) and a gold knob. */
	.bookkeeping-toggle input:checked {
		border-color: var(--color-you);
		box-shadow: inset 0 0 0 1px var(--color-you);
	}
	.bookkeeping-toggle input:checked::before {
		transform: translateX(16px);
		background: var(--color-you);
	}
	.bookkeeping-toggle input:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}
	@media (prefers-reduced-motion: reduce) {
		.bookkeeping-toggle input::before { transition: none; }
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
		padding: var(--space-2) var(--space-4);
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		min-height: 0;
	}

	.empty {
		color: var(--color-text-faint);
		text-align: center;
		margin-top: var(--space-5);
		font-size: var(--text-small);
	}

	/* The brief accent flash after a Public-Record jump lands here now lives in
	   shared/jumpPulse.css — the prologue's claim beat says the same sentence
	   about a tile, and it carries the reduced-motion guard for both. */

	/* "N new below" / "Return to now": a chip pinned over the foot of the feed.
	   It is a control, so it is the house chip (raised fill, neutral border). Only
	   sheets cast shadows, so it has none; its solid fill is what keeps the lines
	   scrolling behind it from showing through. */
	.feed-jump {
		position: absolute;
		left: 50%;
		bottom: var(--space-3);
		transform: translateX(-50%);
		z-index: 5;
		white-space: nowrap;
	}

	/* ── Messages ─────────────────────────────────────────────────────────── */

	/* Byline above body (adr/CHAT_VISUAL_HIERARCHY_PLAN.md S1): a
	   side-by-side author/body/time grid crushed prose into a sliver next to
	   long persona bylines at phone width. Prose gets the full column. The byline
	   is the one place a message spends player colour (colour the byline OR a
	   rule, never both), and the shape beside it says the same thing without
	   colour. */
	.message {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.msg-byline {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--space-2);
	}

	.msg-author {
		color: var(--player-color, var(--color-accent));
		font-size: var(--text-small);
	}

	.msg-body {
		font-family: var(--font-serif);
		font-size: var(--text-body);
		line-height: 1.5;
		color: var(--color-text);
		white-space: pre-wrap;
		word-break: break-word;
	}

	.msg-time {
		font-size: var(--text-label);
		color: var(--color-text-faint);
		white-space: nowrap;
		margin-left: auto;
		display: none;
	}

	/* In-character vs table-talk (Phase 4d) — only meaningful inside a scene
	   container; outside a scene every player post is the plain `.message`.
	   In-character is told by typography alone now (small capitals, in the mask's
	   colour): a ground behind it would be a box around something nobody touches.
	   Table-talk keeps the player's own colour on the name, the same as outside a
	   scene; grey OOC styling is retired for player content everywhere. */
	/* The mask, not the player: a character's words aren't the player's own
	   voice (or interests), so the in-character byline wears a muted cast of
	   the player's colour — hue survives for attribution, but the vivid
	   jewel tone is reserved for the player speaking as themselves. Recipe,
	   not a hand-picked hex (docs/STYLE_GUIDE.md). */
	.message.in-character .msg-author {
		font-variant: small-caps;
		letter-spacing: 0.04em;
		color: color-mix(in srgb, var(--player-color, var(--color-accent)) 55%, var(--color-text-secondary));
	}

	/* ── Log lines ────────────────────────────────────────────────────────── */

	.log {
		display: grid;
		grid-template-columns: auto 1fr auto;
		gap: var(--space-2);
		align-items: baseline;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
	/* Important system posts (hierarchy-plan S2): the game's own colour, deep
	   gold. Weight and colour are importance (one job per channel), so there is no
	   rule beside it: a line down the side would collide with a span's own rule.
	   Same `auto 1fr auto` grid as plain rows, so toggling "hide bookkeeping"
	   never reflows a row's shape, only its colour. The chat tab itself carries no
	   gold for an important event, only the pale dot (Screen anatomy). */
	.log.important {
		color: var(--color-accent);
	}
	/* Minor/Trace tier (hierarchy-plan S2): visible only with "hide
	   bookkeeping" off. Faint, so the ledger keeps three legible registers
	   (important, plain, minor) without a fourth size. */
	.log.minor {
		color: var(--color-text-faint);
	}

	/* The family mark's slot (adr/LOG_MARKS_PLAN.md). Sizes the box that
	   LogMark.svelte's svg fills, and carries the colour it strokes with.

	   16px and centred, both measured. `line-height` is never set anywhere in
	   this chain, so Spectral's own hhea metrics decide the row: 21.00px at
	   14px... a 16px mark centred in the row grows it by 0px, where one aligned
	   to the baseline would add 1.50px. Hence centre, not baseline. (Optical
	   centring is also simply what a geometric mark wants; baseline alignment is
	   for letterforms, which is what these no longer are.) War's barbute brings
	   its own colour (BarbuteMark), so this only sets the default. */
	.log-mark {
		display: flex;
		width: 16px;
		height: 16px;
		align-self: center;
		flex-shrink: 0;
		color: var(--color-text-faint);
	}
	.log.important .log-mark { color: var(--color-accent); }

	/* Player-name marks (hierarchy-plan S4). A log body that names a player
	   paints the name in that player's own colour — the identity channel the
	   message bylines already teach, so "who did this" reads the same whether
	   the line is speech or bookkeeping. Vivid, not the muted `color-mix` cast
	   the in-character bylines wear: that muting means "a character's voice,
	   not the player's own", and a log body naming a player is the player as
	   themselves. Colour is the whole treatment — no weight, no italics; those
	   channels are spent elsewhere (importance, asset names).

	   Reached through `.feed` because `renderLogBody` output is injected with
	   {@html} and so carries no scoping class (same pattern as HelpContent's
	   `.body :global(p)`). `--mark-player-color` is set per-span by the
	   renderer, and only ever to a validated hex; anything else leaves it
	   unset and this fallback carries the name at normal body colour. */
	.feed :global(.player-mark) {
		color: var(--mark-player-color, var(--color-text-secondary));
	}
	.log-time { font-size: var(--text-label); color: var(--color-text-faint); white-space: nowrap; }

	/* Timestamp diet (hierarchy-plan S1): per-line times are metadata, not
	   content — hidden until hover (pointer devices) or tap (one row at a
	   time; `show-time` from revealedTimeID), with the full date in the
	   row's `title`. */
	.log .log-time,
	.ranking-group .log-time {
		display: none;
	}
	@media (hover: hover) {
		.message:hover .msg-time,
		.log:hover .log-time,
		.ranking-group:hover .log-time {
			display: inline;
		}
	}
	.message.show-time .msg-time,
	.log.show-time .log-time {
		display: inline;
	}

	/* Tighter ledger spacing (Phase 3 item 4): a system post immediately
	   following another system post (no divider/player message between them)
	   pulls up toward it, so a run of bookkeeping reads as a compact ledger
	   while player prose keeps the normal, more generous gap. */
	.log.continues-run,
	.ranking-group.continues-run {
		margin-top: calc(-1 * var(--space-2));
	}

	/* ── Rankings ─────────────────────────────────────────────────────────── */

	/* A whole EmitRankingUpdated burst (headline + per-category sections +
	   standings) is one group. It is read, not touched, so it gets no box: the
	   headline, then its lines indented under it. */
	.ranking-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
	.ranking-headline {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--color-accent);
	}
	/* Gold like the headline it leads — `.log-mark`'s faint default targets the
	   mark element directly, so the parent's `color` alone can't reach it. */
	.ranking-headline .log-mark { color: var(--color-accent); }
	.ranking-headline .log-time { margin-left: auto; }
	.ranking-category {
		margin: var(--space-2) 0 0;
		font-size: var(--text-label);
		color: var(--color-accent);
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}
	.ranking-line { padding-left: var(--space-4); }
	.ranking-standing {
		padding-left: var(--space-4);
		color: var(--color-text);
	}

	/* ── Folding spans: a plan's resolution, or a scene ───────────────────────
	   A span's header (SpanHeader) is the only thing in it that is pressed. Its
	   lines sit indented behind a thin rule: no card, no fill, no second border.
	   The rule is the --color-border hairline and starts under the header's mark,
	   so it reads as hanging from it. A plan that stages a scene folds into the
	   scene's container, so the two share this. */
	.span {
		display: flex;
		flex-direction: column;
	}

	.span-body {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		margin-left: var(--space-2);
		padding: var(--space-3) 0 var(--space-1) var(--space-3);
		border-left: 1px solid var(--color-border);
	}

	.scene-banner {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-small);
		color: var(--color-text);
	}
	.scene-banner-mark {
		display: flex;
		flex: none;
		width: 16px;
		height: 16px;
		color: var(--color-text-faint);
	}

	.scene-prompt {
		margin: 0;
		font-style: italic;
		color: var(--color-text-secondary);
		font-size: var(--text-small);
	}

	.scene-participants {
		margin: 0;
		font-size: var(--text-label);
		color: var(--color-text-faint);
	}

	/* The sentence behind the header's status word: what the plan did, and what
	   comes next and who acts. An ordinary line under the rule, in the log's
	   register. */
	.plan-outcome {
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}

	/* ── Typing + compose ─────────────────────────────────────────────────── */

	.typing {
		font-size: var(--text-label);
		color: var(--color-text-faint);
		min-height: 1.2em;
		padding: 0 var(--space-4);
		flex-shrink: 0;
	}

	/* :global — the <p> is rendered by ErrorText, so a plainly-scoped
	   selector wouldn't reach it. Scoped by .panel, not loose. */
	.panel :global(.error) {
		padding: 0 var(--space-4);
		flex-shrink: 0;
	}

	/* The composer: "Speaking as" above the box when there is a choice of voice,
	   then the box and Send. */
	.compose {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-4);
		border-top: 1px solid var(--color-border);
		flex-shrink: 0;
	}

	.compose-row {
		display: flex;
		gap: var(--space-2);
		align-items: flex-end;
	}

	textarea {
		flex: 1;
		min-width: 0;
		min-height: 52px;
		box-sizing: border-box;
		padding: var(--space-3);
		border-radius: var(--radius-control);
		border: 1px solid var(--color-border-control);
		background: var(--color-surface);
		color: var(--color-text);
		font-family: inherit;
		/* 16px, like every input: iPhone Safari zooms the page for anything less. */
		font-size: var(--text-body);
		line-height: 1.4;
		resize: none;
	}
	textarea::placeholder { color: var(--color-text-faint); }
	textarea:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}

	/* The house main button, as tall as the box beside it. It greys out by itself
	   while disabled (actionButton.css), which is how Send reads "nothing to send". */
	.send {
		flex: none;
		align-self: flex-end;
		min-height: 52px;
		min-width: 64px;
	}

	/* ── Player tag in messages ──────────────────────────────────────────── */
	.msg-player-tag {
		color: var(--color-text-faint);
		font-size: var(--text-label);
	}

	/* ── "Speaking as" ───────────────────────────────────────────────────── */

	.persona-bar {
		position: relative;
		align-self: flex-start;
		max-width: 100%;
	}

	/* A control: neutral border, raised fill, the button corner. Its value takes
	   the colour you speak in, vivid as yourself and the muted mask-cast while a
	   character is selected (the same 55% recipe as .in-character bylines). */
	.persona-btn {
		--persona-color: var(--player-color, var(--color-accent));
		display: flex;
		align-items: center;
		gap: var(--space-2);
		max-width: 100%;
		min-height: var(--size-tap);
		padding: 0 var(--space-2) 0 var(--space-3);
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
		background: var(--color-surface);
		color: var(--color-text-secondary);
		font: inherit;
		font-size: var(--text-small);
		cursor: pointer;
	}
	.persona-btn.character {
		--persona-color: color-mix(in srgb, var(--player-color, var(--color-accent)) 55%, var(--color-text-secondary));
	}
	.persona-btn:hover { border-color: var(--color-border-control-hover); }
	.persona-btn:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}

	.persona-label { color: var(--color-text-faint); }

	.persona-value {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--persona-color);
		text-align: left;
	}

	/* The list that opens above the control. A layer over the feed, drawn as a
	   raised surface with a control's border; only sheets cast shadows. */
	.persona-menu {
		position: absolute;
		left: 0;
		bottom: calc(100% + var(--space-1));
		min-width: 100%;
		margin: 0;
		padding: var(--space-1);
		list-style: none;
		background: var(--color-surface);
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
		z-index: 60;
	}

	.persona-option {
		--persona-color: var(--player-color, var(--color-accent));
		display: flex;
		align-items: center;
		gap: var(--space-2);
		width: 100%;
		min-height: var(--size-tap);
		padding: 0 var(--space-3);
		border: none;
		border-radius: var(--radius-control);
		background: none;
		color: var(--persona-color);
		font: inherit;
		font-size: var(--text-small);
		text-align: left;
		cursor: pointer;
	}
	.persona-option.character {
		--persona-color: color-mix(in srgb, var(--player-color, var(--color-accent)) 55%, var(--color-text-secondary));
	}
	.persona-option:hover { background: var(--color-surface-2); }
	.persona-option:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: -2px;
	}
	.persona-option-name { flex: 1; white-space: nowrap; }
	/* The chosen option wears the pick's tick, in bright gold: the tick says it
	   without colour. */
	.persona-tick {
		display: flex;
		flex: none;
		color: var(--color-you);
	}
</style>
