<!-- PrologueView.svelte
  Structured prologue (Phase 4b). Modes driven by game.prologue_ranking_step:

    null   →  choosing: pick boxes from the three sheets; cards make-or-take
    declare_X        →  ANY-card declaration for the current track (the server
                        still spells the step "declare_" + track, and the API
                        still calls the cards hearts — see HandStrip)
    place_set_asides_X →  top-ranked player slots zero-suit players in
    closing          →  "The Stage is Set": name your main character, ≤3p
                         create an extra peer, then ready up. All-ready
                         auto-advances to the main event.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import '$lib/components/shared/trackCode.css';
	import '$lib/components/shared/choicePip.css';
	import '$lib/components/shared/jumpPulse.css';
	import {
		getPrologueSheets,
		getPrologueCards,
		placePrologueSetAsides,
		listAssets,
		getPrologueRankingState,
		commitTrackHearts,
		setPrologueDone,
	} from '$lib/api';
	import type {
		Game,
		Player,
		Asset,
		Ranking,
		RankingCategory,
		PrologueSheet,
		PrologueClaim,
		PlayerCardRow,
		PrologueSheetType,
		CommittedHeart,
		TrackDone,
		PrologueTrack,
		ExtraPeer,
		ClosingReady,
		Law,
		Rumor,
	} from '$lib/api';
	import { onMount, onDestroy, tick } from 'svelte';
	import ClaimChoiceModal from './ClaimChoiceModal.svelte';
	import TrackBoard from './prologue/TrackBoard.svelte';
	import HandStrip from './prologue/HandStrip.svelte';
	import SetAsidePlacer from './prologue/SetAsidePlacer.svelte';
	import ClosingStage from './prologue/ClosingStage.svelte';
	import TurnCard, { type JustClaimed } from './prologue/TurnCard.svelte';
	import StandingStrip from './prologue/StandingStrip.svelte';
	import AssetTypeIcon from '$lib/components/AssetTypeIcon.svelte';
	import WeightMeter from '$lib/components/shared/WeightMeter.svelte';
	import { computeBrightHearts } from '$lib/prologue/refund';
	import { scrollBehavior } from '$lib/reducedMotion';
	import {
		cardHoldStates,
		cardHolders,
		ownedCardCount,
		sheetTrackProfile,
		spentByCategory,
		stealPreview,
		trackCode,
		trackLabel,
		assetTypeLabel,
		assetTypeFor,
		SUIT_MEANINGS,
		type CardHoldState,
	} from '$lib/prologue/choosing';
	import { notReadyPlayerIDs } from '$lib/prologue/closing';
	import { isNeedlesslyAtRisk } from '$lib/assetRisk';
	import type { WaitingOnState, Waitee } from '$lib/waitingOn';
	import { playerColorByID } from '$lib/playerColor';
	import CrownGlyph from '../CrownGlyph.svelte';
	import type { CrownMark } from '$lib/succession';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	interface Props {
		gameID: string;
		game: Game;
		players: Player[];
		assets: Asset[];
		rankings: Ranking[];
		currentPlayerID: number | null;
		isFacilitator: boolean;
		waitingOn: WaitingOnState;
		laws?: Law[];
		rumors?: Rumor[];
		onResync?: () => void;
		onOpenTones?: () => void;
		onOpenRetinue?: (playerID?: number) => void;
		onOpenLaws?: () => void;
		onOpenRumors?: () => void;
	}

	let {
		gameID,
		game,
		players = $bindable(),
		assets = $bindable(),
		rankings = $bindable(),
		currentPlayerID,
		isFacilitator,
		waitingOn = $bindable(),
		laws,
		rumors,
		onResync,
		onOpenTones,
		onOpenRetinue,
		onOpenLaws,
		onOpenRumors,
	}: Props = $props();

	// ── Loaded reference data ────────────────────────────────────────────────
	let sheets = $state<PrologueSheet[]>([]);
	let claims = $state<PrologueClaim[]>([]);
	let cards = $state<PlayerCardRow[]>([]);
	let activePlayerID = $state<number | null>(null);
	let committed = $state<CommittedHeart[]>([]);
	let doneFlags = $state<TrackDone[]>([]);
	let extraPeers = $state<ExtraPeer[]>([]);
	let closingReady = $state<ClosingReady[]>([]);
	// Two error slots, two lifetimes (adr/ERROR_HANDLING_PLAN.md). loadError
	// explains stale or missing prologue data and is owned by reload();
	// actionError explains one control the player just used. They shared a
	// variable until now, and since reload() runs on every WS event but never
	// cleared it, a single transient failure stuck to the view permanently —
	// the only things that could clear it were the action handlers' own entry
	// clears, i.e. taking an unrelated action.
	let loadError = $state('');
	let actionError = $state('');
	let loading = $state(true);

	async function reload() {
		try {
			const [s, c, st] = await Promise.all([
				getPrologueSheets(gameID),
				getPrologueCards(gameID),
				getPrologueRankingState(gameID),
			]);
			sheets = s.sheets;
			claims = s.claims;
			activePlayerID = s.current_player_id;
			cards = c.cards;
			committed = st.committed;
			doneFlags = st.done;
			extraPeers = st.extra_peers;
			closingReady = st.closing_ready;
			// Every field this function owns has just been replaced, so a
			// previous complaint about them is moot. Clearing on success (not
			// on entry) keeps a *failed* reload showing the last message
			// instead of blanking it mid-flight.
			loadError = '';
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load prologue data.';
		} finally {
			loading = false;
		}
	}

	onMount(reload);

	// ── WebSocket-driven refresh ─────────────────────────────────────────────
	function onClaimEvent() { reload(); }
	function onStepChanged() { reload(); }

	onMount(() => {
		window.addEventListener('uneasy:prologue.choice_claimed', onClaimEvent);
		window.addEventListener('uneasy:prologue.turn_advanced', onClaimEvent);
		window.addEventListener('uneasy:prologue.ranking_step_changed', onStepChanged);
		window.addEventListener('uneasy:prologue.track_ranked', onStepChanged);
		window.addEventListener('uneasy:prologue.set_asides_placed', onStepChanged);
		window.addEventListener('uneasy:prologue.committed_hearts_changed', onClaimEvent);
		window.addEventListener('uneasy:prologue.done_changed', onClaimEvent);
		window.addEventListener('uneasy:prologue.extra_peer_created', onClaimEvent);
		window.addEventListener('uneasy:prologue.closing_ready_changed', onClaimEvent);
	});
	onDestroy(() => {
		window.removeEventListener('uneasy:prologue.choice_claimed', onClaimEvent);
		window.removeEventListener('uneasy:prologue.turn_advanced', onClaimEvent);
		window.removeEventListener('uneasy:prologue.ranking_step_changed', onStepChanged);
		window.removeEventListener('uneasy:prologue.track_ranked', onStepChanged);
		window.removeEventListener('uneasy:prologue.set_asides_placed', onStepChanged);
		window.removeEventListener('uneasy:prologue.committed_hearts_changed', onClaimEvent);
		window.removeEventListener('uneasy:prologue.done_changed', onClaimEvent);
		window.removeEventListener('uneasy:prologue.extra_peer_created', onClaimEvent);
		window.removeEventListener('uneasy:prologue.closing_ready_changed', onClaimEvent);
	});

	// ── Derived: claim lookup ────────────────────────────────────────────────
	const claimMap = $derived.by(() => {
		const m = new Map<string, PrologueClaim>();
		for (const c of claims) m.set(`${c.sheet_type}::${c.choice_name}`, c);
		return m;
	});

	// Prologue picker crown (ADR-007 §8; deliberate picker-only deviation, Round
	// 2 of PROLOGUE_CHOOSING_REDESIGN_PLAN.md). Both the Monarch box and the
	// heir boxes always show their crown from the start of choosing, unconditionally
	// — a picker crown just advertises that the role exists and is contestable,
	// not the live succession order (no ordinals here; picks are still
	// happening). This is a deliberate departure from ADR-007 §8's
	// throne_established gate: the LIVE succession UI elsewhere (succession.ts's
	// computeCrowns, used once play begins) still gates on throne_established,
	// as documented there. The General is deliberately never marked here —
	// narratively it is not in the official line of succession.
	const PROLOGUE_HEIR_TITLES = new Set([
		'true_heir', 'favored_heir', 'claimant', 'consort',
	]);
	function prologueCrown(id: string | undefined): CrownMark | null {
		if (id === 'monarch') return { role: 'monarch' };
		if (id && PROLOGUE_HEIR_TITLES.has(id)) return { role: 'successor' };
		return null;
	}

	// Where the viewer's three choices have gone: the remainder draws the turn
	// card's solid pips, the per-sheet counts draw the pips that moved down to
	// the category headers. One walk, so the two homes always add to three.
	const spent = $derived(spentByCategory(claims, currentPlayerID));
	const isMyTurn = $derived(activePlayerID != null && activePlayerID === currentPlayerID);

	// The off-turn turn card's one suggestion. Same rule as the header chip's
	// red disc (tableHeader.ts), so the card gives that number a destination
	// rather than restating it.
	const myAtRiskCount = $derived(
		currentPlayerID == null
			? 0
			: assets.filter((a) => a.owner_id === currentPlayerID && isNeedlesslyAtRisk(a)).length
	);
	function playerName(id: number | null): string {
		if (id == null) return 'Dummy';
		return players.find(p => p.id === id)?.display_name ?? '?';
	}
	/** Second-person for the viewer's own holdings — reading your own display
	 *  name back at you in "now held by …" lines is jarring. */
	function holderLabel(id: number | null): string {
		return id != null && id === currentPlayerID ? 'you' : playerName(id);
	}

	// ── My hand ──────────────────────────────────────────────────────────────
	const myCards = $derived(cards.filter(c => c.player_id === currentPlayerID));

	// (No suitColor any more: suits have left the game UI entirely — Round 2
	// decision 1 for the choosing view, and the owner's Session 3 ruling on the
	// open question for the ranking and the declare step. Nothing anywhere
	// draws a card face now; SuitGlyph and cardGlyph.css are deleted.)

	// (The per-player "Hands" grid is gone, Round 2 §1f: it duplicated the
	// TrackBoard, which already shows every player's cards by track. The one
	// thing it added was heart visibility, which the standing strip's
	// "ANY n in hand" line now carries for the viewer.)

	// ── Choosing accordion (PROLOGUE_CHOOSING_REDESIGN_PLAN.md S1) ───────────
	// Character-facing panel copy, keyed by the stable sheet type rather than
	// the presentational display_name.
	const SHEET_DESCRIPTIONS: Record<PrologueSheetType, string> = {
		titles:
			'Who are you at court? Claim a station — Monarch, Heretic, Spymaster — and gain an artifact of office. The title goes on your main character.',
		hailing_from: 'Where do you come from? Describe your homeland and gain a holding there.',
		laws_rumors:
			'What do people whisper — or decree? Put a law or rumor on the public record and gain the resource it grants you.',
	};

	// Empty on first load (all three panels collapsed); plain component state
	// so it survives the WS-triggered reload() calls, which only replace data.
	let openSheets = $state<Set<PrologueSheetType>>(new Set());

	// The prose that used to open the page — two paragraphs of lede, three of
	// explanation and a 163px legend, all above the first tappable control
	// (Round 2 §1d). Collapsed by default and kept on the page rather than
	// moved into the global Help menu (owner ruling, decision 9). Plain
	// component state, same reason as openSheets.
	let helpOpen = $state(false);

	async function toggleSheetPanel(type: PrologueSheetType, header: HTMLButtonElement) {
		const next = new Set(openSheets);
		const opening = !next.has(type);
		if (opening) next.add(type); else next.delete(type);
		openSheets = next;
		if (opening) {
			// Wait for the panel body to render before scrolling, so a tall
			// panel doesn't strand the header above the viewport.
			await tick();
			header.scrollIntoView({ block: 'nearest' });
		} else if (expandedBox?.startsWith(`${type}::`)) {
			// Expansion is scoped to its panel — collapsing the panel closes it.
			expandedBox = null;
		}
	}

	// Viewer-relative, unlike the plain held-set it replaced: a card the viewer
	// already holds is a dead slot, not a steal opportunity, and the tile has
	// to say so at a glance rather than only inside the expansion.
	const cardStates = $derived(cardHoldStates(cards, currentPlayerID));
	function stateOf(suit: string, value: string): CardHoldState {
		return cardStates.get(`${suit}::${value}`) ?? 'fresh';
	}

	// Whose asset a take would come out of — the take chip's corner dot wears
	// that player's own colour, so "from whom" survives the scan without the
	// name (which is in the expansion). Absent for a fresh card.
	const holders = $derived(cardHolders(cards));
	function holderOf(suit: string, value: string): number | undefined {
		return holders.get(`${suit}::${value}`);
	}

	// (The weight meter is shared/WeightMeter.svelte since Session 3a — the
	// TrackBoard draws the identical object for every card on a track now that
	// suits have left the ranking too.)

	// Current tile-grid column count, mirroring .tile-grid's container query
	// (2 base / 3 when the column is ≥ 420 — the 440-cap region;
	// docs/STYLE_GUIDE.md "Layout widths"). Needed in script so the
	// contiguous expansion (Round 2) can work out which tile ends a visual
	// row. Measured from the component's own width (== the phase column,
	// the query container), so it can never disagree with the CSS the way a
	// viewport matchMedia could.
	let columnWidth = $state(0);
	const tileCols = $derived(columnWidth >= 420 ? 3 : 2);

	// ── Tap-to-explore expansion (PROLOGUE_CHOOSING_REDESIGN_PLAN.md S2) ─────
	// Exploring a box is open to every player at all times; only the on-turn
	// player additionally sees a "Claim this box" action inside it. Keyed by
	// a string (not an index) so it survives the WS-triggered reload() calls
	// and, if the viewed box gets claimed mid-view, updates in place instead
	// of vanishing (claims/cards data changes, but expandedBox does not).
	let expandedBox = $state<string | null>(null);

	function toggleExpand(key: string) {
		expandedBox = expandedBox === key ? null : key;
	}

	// (suit → asset type now comes from choosing.ts's SUIT_MEANINGS, the same
	// table that drives the legend and the sheet-header track profiles.)

	// ── Choose a box ─────────────────────────────────────────────────────────
	let activeClaim = $state<{ sheet: PrologueSheet; choice: PrologueSheet['choices'][number] } | null>(null);

	function openClaimModal(sheet: PrologueSheet, choice: PrologueSheet['choices'][number]) {
		if (activeClaim) return;
		activeClaim = { sheet, choice };
	}

	// ── The motion beat (Round 2 §3c) ────────────────────────────────────────
	// A claim used to commit into silence: the scroll position was
	// byte-identical afterwards and every signal that anything had happened
	// rendered ~250px above the fold. Three beats now answer it, all local to
	// where the player is looking — the tile pulses where it sits, the pip
	// hollows out, and the turn card confirms what the claim did before
	// decaying into "X is choosing".
	let viewEl = $state<HTMLDivElement | null>(null);
	let justClaimed = $state<JustClaimed | null>(null);
	let justClaimedTimer: ReturnType<typeof setTimeout> | null = null;
	/** Long enough to read two clauses and look up at the tile, short enough
	 *  that it is gone before the next player's claim lands. */
	const JUST_CLAIMED_MS = 6000;

	onDestroy(() => {
		if (justClaimedTimer) clearTimeout(justClaimedTimer);
	});

	/**
	 * What the claim just did, in the two verbs the rules use.
	 *
	 * MUST be called before the reload: it reads the pre-claim `cards`/`assets`,
	 * and that is the whole point — afterwards the taken asset is simply yours
	 * and the fresh cards are simply held, with nothing left to say it just
	 * happened. A card the viewer already held is neither made nor taken (the
	 * server no-ops a self-take), so it contributes to neither count.
	 */
	function describeClaim(choice: PrologueSheet['choices'][number]): JustClaimed {
		let made = 1; // the sheet's own asset, always created
		const takes: JustClaimed['takes'] = [];
		for (const c of choice.cards) {
			const preview = stealPreview(c.suit, c.value, cards, assets, players);
			if (!preview) {
				made++;
			} else if (preview.ownerID !== currentPlayerID) {
				takes.push({ assetName: preview.assetName, ownerName: preview.ownerName });
			}
		}
		return { tileName: choice.name, made, takes };
	}

	/** Take the reader to the tile they just claimed and flash it. The scroll
	 *  survives reduced motion (it carries where, which is information); the
	 *  flash is handled by shared/jumpPulse.css, which no-ops under `reduce`. */
	function pulseClaimedTile(boxKey: string) {
		const el = viewEl?.querySelector<HTMLElement>(`[data-tile-key="${CSS.escape(boxKey)}"]`);
		if (!el) return;
		el.scrollIntoView({ block: 'center', behavior: scrollBehavior() });
		// Removed then re-applied after a forced reflow, so a second claim on
		// the same tile-key restarts the animation instead of being ignored.
		el.classList.remove('jump-pulse');
		void el.offsetWidth;
		el.classList.add('jump-pulse');
		setTimeout(() => el.classList.remove('jump-pulse'), 800);
	}

	async function onClaimSubmitted() {
		const claimed = activeClaim;
		activeClaim = null;
		actionError = '';
		// Snapshot the outcome now, against pre-reload data (see describeClaim).
		const summary = claimed ? describeClaim(claimed.choice) : null;
		try {
			const [, assetData] = await Promise.all([reload(), listAssets(gameID)]);
			assets = assetData.assets;
		} catch (e) {
			// actionError, not loadError, for two reasons. The claim itself
			// already succeeded — this is only the follow-up refresh — so the
			// message belongs with the action. And reload() runs concurrently
			// here and clears loadError on its own success, which could land
			// after this line and silently erase it.
			actionError = e instanceof Error ? e.message : 'Your claim went through, but the screen may be out of date.';
		}
		if (!claimed || !summary) return;
		// After the reload, deliberately: the pip's spend animation runs on the
		// pip that has just *become* spent, and setting this first would start
		// it on the last still-solid one and then jump a slot when the fresh
		// claim count arrived.
		if (justClaimedTimer) clearTimeout(justClaimedTimer);
		justClaimed = summary;
		justClaimedTimer = setTimeout(() => {
			justClaimed = null;
			justClaimedTimer = null;
		}, JUST_CLAIMED_MS);
		await tick();
		pulseClaimedTile(`${claimed.sheet.type}::${claimed.choice.name}`);
	}

	// ── Hearts declaration (max-commitment model) ────────────────────────────
	let savingHearts = $state(false);
	let savingDone = $state(false);

	const myCommittedOnTrack = $derived.by(() => {
		const t = currentTrack;
		if (!t || currentPlayerID == null) return [] as number[];
		return committed
			.filter((h) => h.player_id === currentPlayerID && h.track === t)
			.map((h) => h.card_id);
	});

	const allPlayerIDs = $derived(players.map((p) => p.id));

	const brightForViewer = $derived.by(() => {
		const t = currentTrack;
		if (!t || currentPlayerID == null) return new Set<number>();
		const all = computeBrightHearts(t as PrologueTrack, allPlayerIDs, cards, committed);
		return all.get(currentPlayerID) ?? new Set<number>();
	});

	// Tracks already finalized — anything before the current declare/place
	// step is locked. Hearts committed there cannot be retracted.
	const resolvedTracks = $derived.by(() => {
		const step = game.prologue_ranking_step ?? '';
		const seq: PrologueTrack[] = ['power', 'knowledge', 'esteem'];
		const out = new Set<PrologueTrack>();
		const idx = seq.findIndex(
			(t) => step === `declare_${t}` || step === `place_set_asides_${t}`
		);
		if (idx === -1 && step !== '') {
			// closing or beyond — all resolved.
			seq.forEach((t) => out.add(t));
			return out;
		}
		seq.slice(0, idx).forEach((t) => out.add(t));
		return out;
	});

	const myDoneOnTrack = $derived.by(() => {
		const t = currentTrack;
		if (!t || currentPlayerID == null) return false;
		return doneFlags.some(
			(d) => d.player_id === currentPlayerID && d.track === t && d.done
		);
	});

	async function commitOrRetract(cardID: number, retract: boolean) {
		if (savingHearts || !currentTrack || currentPlayerID == null) return;
		savingHearts = true;
		actionError = '';
		try {
			let next = myCommittedOnTrack.slice();
			if (retract) {
				next = next.filter((id) => id !== cardID);
			} else if (!next.includes(cardID)) {
				next.push(cardID);
			}
			await commitTrackHearts(gameID, currentTrack as PrologueTrack, next);
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not update hearts.';
			// Server rejected — our view of the step may be stale. Pull
			// fresh state so the UI catches up.
			onResync?.();
			reload();
		} finally {
			savingHearts = false;
		}
	}

	async function toggleDone() {
		if (savingDone || !currentTrack) return;
		savingDone = true;
		actionError = '';
		try {
			await setPrologueDone(gameID, currentTrack as PrologueTrack, !myDoneOnTrack);
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not update done.';
			onResync?.();
			reload();
		} finally {
			savingDone = false;
		}
	}

	// ── Place set-asides ─────────────────────────────────────────────────────
	const currentTrack = $derived.by(() => {
		const step = game.prologue_ranking_step;
		if (!step) return null;
		if (step.includes('power')) return 'power' as RankingCategory;
		if (step.includes('knowledge')) return 'knowledge' as RankingCategory;
		if (step.includes('esteem')) return 'esteem' as RankingCategory;
		return null;
	});

	const trackRanksHere = $derived.by(() => {
		const t = currentTrack;
		if (!t) return [];
		return rankings.filter(r => r.category === t).sort((a, b) => a.rank - b.rank);
	});

	const setAsidePlayers = $derived.by(() => {
		const t = currentTrack;
		if (!t) return [];
		const ranked = new Set(rankings.filter(r => r.category === t && r.player_id != null).map(r => r.player_id));
		return players.filter(p => !ranked.has(p.id)).map(p => p.id);
	});

	// The player at the top of the current track: the highest-status *real*
	// player, i.e. the lowest-numbered rank with a non-dummy player. Can't assume
	// rank 1 — in 2–3 player games dummy tokens occupy rank 1, so the top player
	// sits at rank 2. Mirrors the backend's PlaceSetAsides auth check.
	const topTrackPlayerID = $derived.by(() => {
		const t = currentTrack;
		if (!t) return null;
		const real = rankings.filter((r) => r.category === t && r.player_id != null);
		if (real.length === 0) return null;
		return real.reduce((top, r) => (r.rank < top.rank ? r : top)).player_id ?? null;
	});

	const isMyTurnForSetAsides = $derived(topTrackPlayerID === currentPlayerID);

	let setAsideOrdering = $state<number[]>([]);
	$effect(() => {
		// Initialize ordering from set-asides whenever they change.
		setAsideOrdering = [...setAsidePlayers];
	});

	const setAsideOpenRanks = $derived.by(() => {
		const t = currentTrack;
		if (!t) return [];
		const taken = new Set(rankings.filter((r) => r.category === t).map((r) => r.rank));
		const dummies = (() => {
			switch (players.length) {
				case 4: return new Set([3]);
				case 3: return new Set([1, 5]);
				case 2: return new Set([1, 3, 5]);
				default: return new Set<number>();
			}
		})();
		const out: number[] = [];
		for (let r = 1; r <= 5; r++) {
			if (!taken.has(r) && !dummies.has(r)) out.push(r);
		}
		return out;
	});

	let placing = $state(false);
	async function submitSetAsides() {
		if (placing) return;
		placing = true;
		actionError = '';
		try {
			await placePrologueSetAsides(gameID, setAsideOrdering);
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not place set-asides.';
		} finally {
			placing = false;
		}
	}

	// ── Phase classification ─────────────────────────────────────────────────
	type Mode = 'choosing' | 'declare' | 'place' | 'closing';
	const mode = $derived.by<Mode>(() => {
		const step = game.prologue_ranking_step;
		if (!step) return 'choosing';
		if (step.startsWith('declare_')) return 'declare';
		if (step.startsWith('place_set_asides_')) return 'place';
		return 'closing';
	});

	// ── Waiting-on derivation ────────────────────────────────────────────────
	// Mode → who's blocking + what they're doing. Returns empty waitees when
	// everyone has finished the current step (the prologue view falls back to
	// its own "everyone finished" copy in that case).
	const prologueWaitingOn = $derived.by<WaitingOnState>(() => {
		if (loading) return { waitees: [] };
		if (mode === 'choosing') {
			if (activePlayerID == null) return { waitees: [] };
			// "chosen", not "turn N of 12": a turn count next to a personal
			// three-choice allowance reads as a personal allowance of twelve.
			// The pips carry the personal number now; this one is the table's.
			return {
				waitees: [{ kind: 'player', playerID: activePlayerID }],
				stepLabel: `Prologue — ${claims.length} of ${players.length * 3} chosen`,
			};
		}
		if (mode === 'declare') {
			const t = currentTrack;
			if (!t) return { waitees: [] };
			const notDone = players
				.filter(p => !doneFlags.some(d => d.player_id === p.id && d.track === t && d.done))
				.map<Waitee>(p => ({ kind: 'player', playerID: p.id }));
			if (notDone.length === 0) return { waitees: [] };
			const waitees: Waitee[] = notDone.length === players.length
				? [{ kind: 'everyone' }]
				: notDone;
			// The same three letters the choosing view taught and the hand below
			// spends — not "Hearts", which was the last deck word left in the
			// app (Round 2 Session 3's ruling on the open question). "on", not
			// "for": you are putting the card on that track.
			//
			// This is the one place the code stands in a bare sentence fragment
			// with no chip around it, so "Spend ANY on Power" can be misread as
			// the quantifier (Round 3, decision 3 / §1b). It ships because the
			// header sits directly above a hand of cards each stamped ANY —
			// re-read live before changing it to "Spend ANY cards on", which
			// costs 6 chars in the longest of the four mode labels.
			return {
				waitees,
				stepLabel: `Rankings: Spend ${trackCode('H')} on ${t.charAt(0).toUpperCase() + t.slice(1)}`,
			};
		}
		if (mode === 'place') {
			if (topTrackPlayerID == null) return { waitees: [] };
			return {
				waitees: [{ kind: 'player', playerID: topTrackPlayerID }],
				stepLabel: 'Place set-asides',
			};
		}
		// closing
		const notReadyIDs = notReadyPlayerIDs(players, closingReady);
		if (notReadyIDs.length === 0) return { waitees: [] };
		const waitees: Waitee[] = notReadyIDs.length === players.length
			? [{ kind: 'everyone' }]
			: notReadyIDs.map<Waitee>(id => ({ kind: 'player', playerID: id }));
		return { waitees, stepLabel: 'Finish Prologue' };
	});
	$effect(() => { waitingOn = prologueWaitingOn; });
</script>

<!-- One chip per card slot, in place of the card face (Round 2, §2c): what the
     slot makes, which track it feeds, and — for a take — a corner dot in the
     current holder's colour. The value is NOT here: 4 segments × 2 chips × 12
     tiles is 96 micro-bars per open sheet, and weight only matters when
     comparing two tiles, which is when you'd open one anyway. -->
{#snippet cardChip(card: { suit: string; value: string }, claimed: boolean)}
	<!-- Claimed wins this slot outright, so a claimed tile's chips go neutral:
	     both the take fill and the already-yours strike describe what claiming
	     WOULD do, and nobody can claim this one. Same ruling as the
	     "Both cards already yours" note, which a claimed tile also suppresses —
	     and a blue opportunity chip on an unclaimable tile is a louder lie than
	     the 1px ring that used to sit there. -->
	{@const state = claimed ? 'fresh' : stateOf(card.suit, card.value)}
	{@const holderID = holderOf(card.suit, card.value)}
	<span
		class="tile-chip"
		class:take={state === 'steal'}
		class:inert={state === 'mine'}
		class:wild={trackLabel(card.suit) === ''}
	>
		<AssetTypeIcon type={assetTypeFor(card.suit)} size={12} />
		<span class="chip-code">{trackCode(card.suit)}</span>
		{#if state === 'steal' && holderID != null}
			<span
				class="chip-owner"
				role="img"
				aria-label={`held by ${playerName(holderID)}`}
				style:background={playerColorByID(holderID, players)}
			></span>
		{/if}
	</span>
{/snippet}

<div class="prologue-view" bind:clientWidth={columnWidth} bind:this={viewEl}>
	<!-- Load errors sit at the top of the view: they explain the content
	     below being stale or missing. Action errors render beside the control
	     that raised them, further down in each mode's branch. -->
	{#if loadError}
		<ErrorText message={loadError} />
	{/if}

	{#if loading}
		<p class="muted-text">Loading prologue…</p>

	{:else if mode === 'choosing'}
		<!-- Round 2 order: the turn card first (it names the action and holds
		     the pips), then the prose behind its disclosure, then where you
		     stand, then the picker — which now starts ~240px into the column
		     instead of 658px, i.e. on the first screen. -->
		<div class="choosing-stack">
			<TurnCard
				activeName={activePlayerID == null ? null : playerName(activePlayerID)}
				{isMyTurn}
				unspent={currentPlayerID == null ? null : 3 - spent.total}
				atRiskCount={myAtRiskCount}
				{justClaimed}
				onOpenRetinue={() => onOpenRetinue?.()}
			/>

			<section class="help-disclosure" class:open={helpOpen}>
				<button
					type="button"
					class="disc-head"
					aria-expanded={helpOpen}
					aria-controls={helpOpen ? 'prologue-help-body' : undefined}
					onclick={() => (helpOpen = !helpOpen)}
				>
					<span class="disc-glyph" aria-hidden="true">?</span>
					<span class="disc-title">How the prologue works</span>
					<span class="disc-caret" aria-hidden="true">▾</span>
				</button>
				{#if helpOpen}
					<div class="disc-body" id="prologue-help-body">
						<p class="prologue-lede">
							We start the game with the prologue, where we take turns fleshing out
							our characters and the world they inhabit.
						</p>
						<p class="prologue-lede">
							One turn you might decide your character is the monarch, and then the
							next you might say that they hail from a castle on the coast.
						</p>
						<p class="prologue-subtext">
							You get three choices — spend them however you like. If you want three
							titles, take three titles. Each tile you claim creates an asset and
							grants 2 cards.
						</p>
						<p class="prologue-subtext">
							Cards let you create <span class="steal-color">or steal</span> another asset,
							and improve your rank in either Power, Knowledge, or Esteem.
						</p>
						<p class="prologue-subtext">
							You can edit your assets (including your main character) at any time in your player menu (top of the screen).
						</p>
						<!-- Type → code → track, in the same two icons and three letters the
						     tiles use, so the legend teaches the tile rather than a third
						     notation (Round 2, §2e). It replaces the suit legend, which taught
						     only "♣ makes a holding" while the track board below taught only
						     "♣ is Power" — and the two readings collide (♠ makes an artifact
						     but raises Esteem), so half a lesson was worse than none. -->
						<table class="legend">
							<thead>
								<tr>
									<th scope="col">When you make a…</th>
									<th scope="col">It raises your…</th>
								</tr>
							</thead>
							<tbody>
								{#each SUIT_MEANINGS as m (m.suit)}
									<tr>
										<th scope="row">
											<AssetTypeIcon type={assetTypeFor(m.suit)} size={14} />
											{m.assetType}
										</th>
										<td>
											<span class="track-code" class:wild={m.track == null}>{m.code}</span>
											{#if m.track}
												{m.track}
											{:else}
												<!-- The chip says "any"; this says "later". It used to read
												     "any track — you choose at the end", which was the row
												     explaining WLD by saying the word the code should have
												     been — and after the rename it stuttered (Round 3,
												     decision 4). -->
												<span class="wild-note">You choose the track at the end</span>
											{/if}
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
						<!-- No held/already-yours legend rows: both states are already spelled
						     out in words inside the tile expansion ("Takes X from Y" /
						     "You already hold X — nothing to take"), and repeating them here
						     cost two lines of scroll on the screen we're trying to shorten. -->
						<p class="legend-note">
							<WeightMeter value="K" />
							Rank tie-breaker weight. If two players hold the same number
							of a track's cards, the heavier ones break the tie.
						</p>
						<h4>Starting rankings</h4>
						<!-- "A card marked ANY", not "Your ANY cards": the code is a real
						     English word now, so in running prose it parses as the
						     quantifier first and the label second. "marked" is what tells
						     the reader this is a label (Round 3, decision 3). -->
						<p class="prologue-subtext">
							The cards you gather set the initial rankings. A card marked ANY ranks
							nothing on its own — you choose which track to spend each one on at
							the end.
						</p>
					</div>
				{/if}
			</section>

			<StandingStrip {players} {cards} {rankings} {committed} {doneFlags} {currentPlayerID} />

			<!-- The choosing mode's one action path is the claim modal, which
			     closes before its follow-up refresh can fail — so this sits above
			     the tiles the player just came back to. -->
			{#if actionError}
				<ErrorText message={actionError} />
			{/if}

				<div class="sheet-accordion">
				{#each sheets as sheet (sheet.type)}
					{@const isOpen = openSheets.has(sheet.type)}
					{@const profile = sheetTrackProfile(sheet)}
					{@const spentHere = spent.bySheet.get(sheet.type) ?? 0}
					<section class="sheet-panel" class:open={isOpen}>
						<button
							type="button"
							class="sheet-header"
							aria-expanded={isOpen}
							aria-controls={isOpen ? `sheet-body-${sheet.type}` : undefined}
							onclick={(e: MouseEvent) => toggleSheetPanel(sheet.type, e.currentTarget as HTMLButtonElement)}
						>
							<span class="sheet-name">{sheet.display_name}</span>
							<!-- Which tracks this category feeds. Every sheet supplies
							     exactly two of the three, so picking a category is already
							     a ranking decision — and since all three panels start
							     collapsed, these headers are the whole first screen.
							     Only the ranked tracks: every sheet carries wilds (the
							     legend covers them) and the absent track is implied by the
							     two that are named. Rides the title row, which frees the
							     description to run full width below. -->
							<span class="sheet-tracks">
								{#each profile.tracks as s (s)}
									<span class="track-code">{trackCode(s)}</span>
								{/each}
							</span>
							<!-- Only when non-zero: an always-present slot leaves an
							     orphan divider on the two categories you haven't spent
							     on, and the absence is the message on those. -->
							{#if spentHere > 0}
								<span
									class="sheet-pips"
									aria-label={`${spentHere} of your three choices spent on ${sheet.display_name}`}
								>
									{#each Array(spentHere) as _, i (i)}
										<span class="choice-pip"></span>
									{/each}
								</span>
							{/if}
							<span class="sheet-caret" aria-hidden="true">▾</span>
							<span class="sheet-desc">{SHEET_DESCRIPTIONS[sheet.type]}</span>
						</button>
						{#if isOpen}
							{@const expandedIndex = sheet.choices.findIndex(c => `${sheet.type}::${c.name}` === expandedBox)}
							{@const rowEnd = expandedIndex === -1
								? -1
								: Math.min(Math.floor(expandedIndex / tileCols) * tileCols + tileCols - 1, sheet.choices.length - 1)}
							<div class="sheet-body" id={`sheet-body-${sheet.type}`} role="region" aria-label={sheet.display_name}>
								<div class="tile-grid">
									{#each sheet.choices as choice, i (choice.name)}
										{@const existingClaim = claimMap.get(`${sheet.type}::${choice.name}`)}
										{@const boxKey = `${sheet.type}::${choice.name}`}
										{@const isExpanded = expandedBox === boxKey}
										{@const tileID = `choice-${sheet.type}-${choice.name}`}
										{@const ownedHere = ownedCardCount(choice, cardStates)}
										<button
											type="button"
											id={tileID}
											data-tile-key={boxKey}
											class="choice-btn"
											class:claimed={!!existingClaim}
											class:expanded={isExpanded}
											aria-expanded={isExpanded}
											aria-controls={isExpanded ? `${tileID}-detail` : undefined}
											aria-label={existingClaim ? `${choice.name}, claimed by ${playerName(existingClaim.player_id)}` : undefined}
											style:box-shadow={existingClaim ? `inset 3px 0 0 ${playerColorByID(existingClaim.player_id, players)}` : undefined}
											onclick={() => toggleExpand(boxKey)}
										>
											<span class="choice-name">
												{choice.name}
												{#if sheet.type === 'titles'}
													{@const crown = prologueCrown(choice.id)}
													{#if crown}<CrownGlyph mark={crown} size={13} />{/if}
												{/if}
											</span>
											<span class="choice-cards">
												{@render cardChip(choice.cards[0], !!existingClaim)}
												{@render cardChip(choice.cards[1], !!existingClaim)}
											</span>
											{#if existingClaim}
												<!-- Claimed wins this slot outright. A claimed tile is often
												     one whose cards you hold — claiming it is what put them in
												     your hand — and "Both cards already yours" there reads as
												     the *reason* the tile can't be picked, which is wrong: it
												     can't be picked because it's taken. -->
												<span class="choice-note">Claimed by {playerName(existingClaim.player_id)}</span>
											{:else if ownedHere === 2}
												<!-- Both cards are no-ops for this viewer, so the tile grants
												     them the sheet asset and nothing else. Worth saying in
												     words: two struck glyphs are easy to miss while scanning
												     36 boxes, and the tile is still perfectly claimable, so
												     it can't be dimmed the way a claimed tile is.

												     "Both", not "Both cards" (Round 3, decision 9): the two
												     chips this note sits under are what "both" refers to, so
												     the noun was restating what the reader is already looking
												     at — and this note shares a 126px tile with them. -->
												<span class="choice-note">Both already yours</span>
											{/if}
										</button>
										{#if i === rowEnd}
											{@const expChoice = sheet.choices[expandedIndex]}
											{@const expClaim = claimMap.get(`${sheet.type}::${expChoice.name}`)}
											{@const expTileID = `choice-${sheet.type}-${expChoice.name}`}
											<div class="choice-detail" id={`${expTileID}-detail`} role="region" aria-labelledby={expTileID}>
												{#if expChoice.description}
													<p class="detail-desc">{expChoice.description}</p>
												{/if}
												{#if expClaim}
													<p class="detail-claimed">Claimed by {playerName(expClaim.player_id)}.</p>
												{/if}
												<div class="detail-cards">
													{#each expChoice.cards as c}
														{@const preview = stealPreview(c.suit, c.value, cards, assets, players)}
														{@const state = stateOf(c.suit, c.value)}
														{@const track = trackLabel(c.suit)}
														<!-- Both state markings are about what claiming would do,
														     so a claimed tile shows neither — its rows say where the
														     card sits now instead. -->
														<div
															class="detail-card-row"
															class:take={!expClaim && state === 'steal'}
															class:inert={!expClaim && state === 'mine'}
														>
															<AssetTypeIcon type={assetTypeFor(c.suit)} size={13} />
															<span class="detail-card-text">
																{#if !preview}
																	Make a new {assetTypeLabel(c.suit)}
																{:else if expClaim}
																	<!-- Claimed tiles can never be claimed again, so describe
																	     where the card sits now instead of previewing a take
																	     that will never happen. -->
																	{#if preview.assetName}
																		<em>{preview.assetName}</em> — now held by
																		<span style:color={playerColorByID(preview.ownerID, players)}>{holderLabel(preview.ownerID)}</span>
																	{:else}
																		Now held by
																		<span style:color={playerColorByID(preview.ownerID, players)}>{holderLabel(preview.ownerID)}</span>
																	{/if}
																{:else if preview.ownerID === currentPlayerID}
																	<!-- Card pairs repeat across tiles, so a card you already
																	     hold turns up on tiles that are still open — and you
																	     can't take from yourself (the claim is a no-op). -->
																	{#if preview.assetName}
																		You already hold <em>{preview.assetName}</em> — nothing to take
																	{:else}
																		Already yours — nothing to take
																	{/if}
																{:else if preview.assetName}
																	Takes <em>{preview.assetName}</em> from
																	<span style:color={playerColorByID(preview.ownerID, players)}>{preview.ownerName}</span>
																{:else}
																	Already held by
																	<span style:color={playerColorByID(preview.ownerID, players)}>{preview.ownerName}</span>
																{/if}
															</span>
															<WeightMeter value={c.value} />
															<!-- No "4th → 2nd": rank slots are literal 1–5 with dummy
															     tokens in some of them, so an ordinal lies in a 2–3
															     player game (decision 7) — and the hearts step re-orders
															     everything at the end anyway. The arrow says the one
															     durable thing: this slot moves you up that track. Absent
															     on a wild (no track yet), on a card you already hold (the
															     claim is a no-op) and on a claimed tile (nothing about it
															     is still on offer). -->
															<!-- "any track", not bare "any": this cell is
															     text-transform:uppercase, so it sits beside POWER /
															     KNOWLEDGE / ESTEEM, and its whole job (see .detail-track)
															     is to expand the tile's abbreviation rather than repeat
															     it. Nine characters, same as KNOWLEDGE, which already
															     sets the cell's width (Round 3, decision 5). -->
															<span class="detail-track">
																{track || 'any track'}
																{#if track && state !== 'mine' && !expClaim}
																	<span class="rise" role="img" aria-label="raises you on this track">↑</span>
																{/if}
															</span>
														</div>
													{/each}
												</div>
												{#if !expClaim && isMyTurn}
													<button
														type="button"
														class="action-btn primary detail-claim-btn"
														onclick={() => openClaimModal(sheet, expChoice)}
													>
														Claim this tile
													</button>
												{/if}
											</div>
										{/if}
									{/each}
								</div>
							</div>
						{/if}
					</section>
				{/each}
				</div>
		</div>

	{:else if mode === 'declare'}
		{#if currentTrack}
			<TrackBoard
				{players}
				{cards}
				{rankings}
				{committed}
				{doneFlags}
				activeTrack={currentTrack as PrologueTrack}
				{currentPlayerID}
			/>

			<HandStrip
				myCards={myCards}
				{committed}
				activeTrack={currentTrack as PrologueTrack}
				brightSet={brightForViewer}
				busy={savingHearts}
				{resolvedTracks}
				onCommit={(id) => commitOrRetract(id, false)}
				onRetract={(id) => commitOrRetract(id, true)}
			/>

			<button
				class="action-btn primary done-btn"
				class:active={myDoneOnTrack}
				disabled={savingDone}
				onclick={toggleDone}
			>
				{savingDone ? '…' : myDoneOnTrack ? 'Done ✓ (tap to undo)' : "I'm done"}
			</button>
			{#if actionError}
				<ErrorText message={actionError} />
			{/if}
			<!-- Not "ANY cards doing work": in this step the hand is entirely
			     these cards, so the qualifier was redundant even before the code
			     became a word that reads as a quantifier (Round 3, decision 3). -->
			<p class="muted-text small">
				Once every player marks Done, this track resolves: the cards doing work lock in, the rest return to your hand.
			</p>
		{/if}

	{:else if mode === 'place'}
		{#if currentTrack}
			<TrackBoard
				{players}
				{cards}
				{rankings}
				{committed}
				{doneFlags}
				activeTrack={currentTrack as PrologueTrack}
				{currentPlayerID}
			/>
			{#if topTrackPlayerID != null && setAsideOrdering.length > 0}
				<SetAsidePlacer
					{players}
					{setAsideOrdering}
					openRanks={setAsideOpenRanks}
					topTrackPlayerID={topTrackPlayerID}
					isMyTurn={isMyTurnForSetAsides}
					busy={placing}
					onReorder={(next) => (setAsideOrdering = next)}
					onConfirm={submitSetAsides}
				/>
			{/if}
			{#if actionError}
				<ErrorText message={actionError} />
			{/if}
		{/if}

	{:else if mode === 'closing'}
		<ClosingStage
			{gameID}
			{players}
			bind:assets
			{currentPlayerID}
			{closingReady}
			{extraPeers}
			{sheets}
			{claims}
			{rankings}
			{cards}
			{committed}
			{doneFlags}
			{laws}
			{rumors}
			onReload={reload}
			{onResync}
			{onOpenTones}
			{onOpenRetinue}
			{onOpenLaws}
			{onOpenRumors}
		/>

	{/if}

</div>

{#if activeClaim}
	<ClaimChoiceModal
		{gameID}
		sheet={activeClaim.sheet}
		choice={activeClaim.choice}
		cards={cards}
		assets={assets}
		players={players}
		{currentPlayerID}
		onClose={() => activeClaim = null}
		onSubmitted={onClaimSubmitted}
	/>
{/if}

<style>
	.prologue-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		padding: 1rem 0.75rem;
		gap: 1rem;
		overflow-y: auto;
		min-height: 0;
	}
	/* (No h3 rule any more: Round 2 §1f/§1g removed the last three headings in
	   this component — "Your Retinue", "Hands" and "Starting rankings". The
	   disclosure's own h4 is styled below.) */

	/* The choosing view's own rhythm, tighter than the 1rem the other three
	   modes use. Four objects now stack above the picker where two paragraphs
	   used to, and 1rem between them costs ~24px of the first screen — which
	   is the screen this whole round is about. Scoped to a wrapper rather than
	   applied to .prologue-view so declare/place/closing keep their spacing. */
	.choosing-stack {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		min-width: 0;
	}

	.prologue-lede {
		margin: 0;
		color: var(--color-text);
		font-size: 1.05rem;
		line-height: 1.45;
	}
	.prologue-subtext {
		margin: 0;
		color: var(--color-text-secondary);
		font-size: 0.9rem;
		line-height: 1.4;
	}
	/* Matches the take chips on the tiles below — blue/attention, because a
	   take is an opportunity for the reader, not a warning to them. See the
	   .tile-chip.take comment further down for the full reasoning. */
	.steal-color { color: var(--color-highlight); }

	/* Genuinely tabular (one row per asset type, two independent readings per
	   row), so a real table — the column headers are what make the second
	   reading legible, and they carry to screen readers for free. */
	.legend {
		border-collapse: collapse;
		/* Content-sized, not stretched: a full-width table pushes the columns
		   apart until the row stops reading as one statement. Shrunk to its
		   contents, the block centres visibly and each row scans as
		   "[icon] Holding → POW Power". */
		width: max-content;
		max-width: 100%;
		margin-inline: auto;
		font-size: 0.9rem;
		color: var(--color-text-secondary);
		text-align: left;
	}
	.legend th,
	.legend td {
		padding: 0.15rem 0.75rem 0.15rem 0;
		font-weight: inherit;
		vertical-align: middle;
	}
	.legend th:last-child,
	.legend td:last-child { padding-right: 0; }
	.legend thead th {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
		padding-bottom: 0.3rem;
		/* The peer row's "any track — you choose at the end" is the widest cell
		   in the table by some way; without this the squeeze lands on the
		   headings, which then wrap while their neighbour doesn't. Let the long
		   note wrap instead — it's a sentence, they're labels. */
		white-space: nowrap;
	}
	/* The icon rides in the row header beside its own word, which is what the
	   tile chips show too — one glance teaches both. */
	.legend tbody th {
		display: flex;
		align-items: center;
		gap: 0.35rem;
	}
	.legend tbody td { color: var(--color-text); }
	/* A wild ranks nothing until you assign it, so the cell reads as a note
	   rather than a track name. */
	.wild-note { color: var(--color-text-secondary); font-style: italic; }
	/* The weight meter's one appearance outside a tile expansion: a static
	   sample, so the meter has been seen before it turns up on a row. */
	.legend-note {
		display: flex;
		/* Top-aligned, not centred: the note runs to three lines at 375 and a
		   centred meter lands beside the middle one, reading as a stray mark
		   rather than the thing the sentence is about. */
		align-items: flex-start;
		gap: 0.4rem;
		margin: 0;
		color: var(--color-text-faint);
		font-size: 0.75rem;
		line-height: 1.35;
	}
	/* :global, because the meter is shared/WeightMeter.svelte's root element —
	   it carries that component's scope class, not this one's. */
	.legend-note :global(.weight) { margin-top: 0.15rem; }

	/* Local help disclosure (Round 2 §1d). Same frame as a collapsed sheet
	   header — it sits directly above three of them, and a second collapsed-row
	   idiom on one screen would just be noise. */
	.help-disclosure {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.disc-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		min-height: 44px;
		padding: 0.5rem 0.7rem;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		color: inherit;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.help-disclosure.open .disc-head {
		border-bottom-color: transparent;
		border-bottom-left-radius: 0;
		border-bottom-right-radius: 0;
	}
	.disc-glyph {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 18px;
		height: 18px;
		border-radius: 50%;
		border: 1px solid var(--color-accent);
		color: var(--color-accent);
		font-size: 0.7rem;
		flex: none;
	}
	.disc-title { flex: 1; color: var(--color-text-secondary); font-size: 0.88rem; min-width: 0; }
	.disc-caret {
		flex: none;
		color: var(--color-accent);
		font-size: 0.75rem;
		transform: rotate(-90deg);
		transition: transform 0.15s ease;
	}
	.help-disclosure.open .disc-caret { transform: rotate(0); }
	/* Reduced motion (docs/STYLE_GUIDE.md "Motion & the deck"): the caret still
	   turns — its direction is the open/closed state — it just snaps. */
	@media (prefers-reduced-motion: reduce) {
		.disc-caret { transition: none; }
	}
	.disc-body {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		border: 1px solid var(--color-border);
		border-top: none;
		border-bottom-left-radius: 8px;
		border-bottom-right-radius: 8px;
		padding: 0.6rem 0.7rem;
	}
	.disc-body h4 {
		margin: 0.25rem 0 0;
		color: var(--color-accent);
		font-size: 0.9rem;
	}
	/* The lede kept its 1.05rem while it WAS the page opening. Inside a help
	   body it's just the first of six paragraphs, and at full size it reads as
	   a second heading; it keeps the brighter ink to stay the lede. */
	.disc-body .prologue-lede { font-size: 0.95rem; }

	.sheet-accordion {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.sheet-panel {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	/* Title row (name · tracks · count · caret), then the description across
	   the full width beneath it. Putting the tracks up top and dropping the
	   description out of the squeezed middle column is a net saving: the
	   profile no longer needs a row of its own, and the description reflows
	   wider, so each collapsed header lost a line rather than gaining one. */
	.sheet-header {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto auto;
		grid-template-areas:
			'name tracks pips caret'
			'desc desc   desc desc';
		align-items: center;
		gap: 0.25rem 0.5rem;
		width: 100%;
		min-height: 44px;
		padding: 0.55rem 0.7rem;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		color: inherit;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.sheet-panel.open .sheet-header {
		/* Calmer than gold (Round 2): the expanded tile/detail join is the one
		   gold shape on screen now, so the open panel itself steps back to the
		   standard warm ledger border. */
		border-color: var(--color-border-warm);
		border-bottom-color: transparent;
		border-bottom-left-radius: 0;
		border-bottom-right-radius: 0;
	}
	.sheet-name { grid-area: name; color: var(--color-accent); font-size: 0.95rem; }
	.sheet-desc {
		grid-area: desc;
		color: var(--color-text-secondary);
		font-size: 0.78rem;
		line-height: 1.3;
	}
	/* The choices you have spent on this category — the pips that have moved
	   down out of the turn card (Round 2, decision 4). Pushed away from the
	   track codes and given a hairline of its own: a lone gold disc sitting
	   flush against POW KNO reads as a third code. */
	.sheet-pips {
		grid-area: pips;
		display: inline-flex;
		align-items: center;
		align-self: stretch;
		gap: 4px;
		margin-left: 0.35rem;
		padding-left: 0.5rem;
		border-left: 1px solid var(--color-border);
	}

	/* Right-aligned against the pip slot, so the three headers' profiles
	   line up as a column the eye can compare down. */
	.sheet-tracks {
		grid-area: tracks;
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		align-items: center;
		gap: 0.2rem 0.3rem;
	}

	.sheet-caret {
		grid-area: caret;
		color: var(--color-accent);
		font-size: 0.75rem;
		/* Points right when collapsed; rotates down to ▾ on open. */
		transform: rotate(-90deg);
		transition: transform 0.15s ease;
	}
	.sheet-panel.open .sheet-caret { transform: rotate(0); }
	/* Reduced motion (docs/STYLE_GUIDE.md "Motion & the deck"): as .disc-caret
	   above — the direction is the state, the sweep is the decoration. */
	@media (prefers-reduced-motion: reduce) {
		.sheet-caret { transition: none; }
	}

	.sheet-body {
		border: 1px solid var(--color-border-warm);
		border-top: none;
		border-bottom-left-radius: 8px;
		border-bottom-right-radius: 8px;
		padding: 0.5rem;
	}
	.tile-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.5rem;
		/* Referenced by .choice-detail's negative margin below, so the join
		   between the expanded tile's row and its detail panel closes the same
		   gap the grid itself uses at each breakpoint — inherited by every
		   grid child since custom properties cascade through the DOM. */
		--tile-grid-gap: 0.5rem;
	}
	/* Three columns when the phase column is at the top of the phone band
	   (≥ 420: the largest phones and capped desktop columns). Mirrored by
	   `tileCols` in the script — keep the two in sync. Known cost, accepted
	   2026-07-17: the four longest box names wrap at 3-col until they're
	   renamed (adr/LAYOUT_WIDTHS_PLAN.md). */
	@container column (min-width: 420px) {
		.tile-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 0.6rem; --tile-grid-gap: 0.6rem; }
	}

	.choice-btn {
		text-align: left;
		background: var(--color-surface-2);
		color: var(--color-text);
		border: 1px solid var(--color-border-strong);
		border-radius: 6px;
		padding: 0.5rem 0.55rem;
		font-size: 0.85rem;
		min-height: 44px;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		cursor: pointer;
		min-width: 0;
	}

	/* Claimed tiles (Round 2): colour, not text — the claimer's name moved to
	   the expansion and the aria-label; here it's just the dim + a left-edge
	   bar in the claimer's colour (set inline via style:box-shadow, since the
	   colour comes from playerColor.ts at runtime, not a CSS token). Inset,
	   not border-left, so the tile's own 6px corner radius survives. */
	.choice-btn.claimed { opacity: 0.55; }

	/* Contiguous expansion (Round 2): the tile stays in its own grid cell —
	   the detail panel is inserted as a full-width sibling after the last tile
	   of its row (script-side row math) rather than the tile itself spanning
	   the grid. The tile's bottom corners un-round and pull 1px into the gap
	   so its accent border fuses with the panel's top border into one shape;
	   a small downward caret pins the join to this tile's own column,
	   independent of where in the row it sits. */
	.choice-btn.expanded {
		position: relative;
		border-color: var(--color-accent);
		border-bottom-left-radius: 0;
		border-bottom-right-radius: 0;
		margin-bottom: -1px;
	}
	.choice-btn.expanded::after {
		content: '';
		position: absolute;
		left: 50%;
		bottom: -7px;
		width: 0;
		height: 0;
		border-left: 6px solid transparent;
		border-right: 6px solid transparent;
		border-top: 7px solid var(--color-accent);
		transform: translateX(-50%);
		pointer-events: none;
	}

	.choice-name { display: flex; align-items: center; gap: 0.25rem; color: var(--color-accent); line-height: 1.2; word-break: break-word; }
	/* Side by side even in the narrowest case: a 3-up tile at the 440 column
	   cap is 126px, 106.7px of content, and the widest possible pair (POW POW,
	   measured) is 98.4px + this gap — which only fits because the weight
	   meter stays off the chips (decision 2). `wrap` is the safety net, not
	   the plan; if a font fallback ever eats the ~5px of slack the tile grows
	   a line rather than spilling. */
	.choice-cards { display: flex; gap: 0.2rem; flex-wrap: wrap; }
	.choice-note {
		color: var(--color-text-muted);
		font-size: 0.7rem;
		font-style: italic;
		line-height: 1.2;
	}

	/* ── Tile chips (Round 2, §2c) ─────────────────────────────────────────
	   One chip per card slot: the asset type as an icon, the ranking track as
	   three letters. No suit — the suit was a strict alias for exactly these
	   two facts, and it stated neither (decision 1).

	   The code is bare text inside the chip rather than a nested .track-code:
	   the chip is already the bordered box, and a bordered code inside a
	   bordered chip is two frames around one word. The dashed-wild idiom isn't
	   forked, it moves up one level — the chip's own border goes dashed. */
	.tile-chip {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: 0.2rem;
		flex: none;
		padding: 0.1rem 0.2rem;
		border: 1px solid var(--color-border);
		border-radius: 4px;
		color: var(--color-text-muted);
	}
	/* AssetTypeIcon paints itself --color-text; inside a chip that leaves a
	   bright icon beside muted (or blue, or struck-through) letters, so the
	   chip reads as two objects. It's one object — let it take the chip's own
	   ink in every state. */
	.tile-chip :global(.type-icon) { color: inherit; }
	.chip-code {
		font-size: 0.62rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		white-space: nowrap;
	}
	/* A take now marks the whole chip, not a 1px ring on a ~20px card face
	   (decision 5): finding opportunities meant checking 24 chips per open
	   sheet, and a take is the most exciting verb in the prologue. Blue, not
	   orange — the blue family is attention AND opportunity, and this is
	   something the viewer stands to gain. */
	.tile-chip.take {
		background: var(--color-chip-blue-bg);
		border-color: var(--color-chip-blue-border);
		color: var(--color-highlight);
	}
	/* Not a track yet, so it borrows DifficultyMeter's dashed
	   means-not-yet rather than inventing a fourth glyph for the ANY chip
	   (Round 2, decision 3). Same idiom as shared/trackCode.css's
	   .track-code.wild — the class keeps the concept's name, not the label's
	   (Round 3, decision 2). */
	.tile-chip.wild { border-style: dashed; }
	/* This card does nothing for you: the server no-ops a claim on a card you
	   already hold, so that half of the tile yields nothing. Struck as well as
	   dimmed, because dim alone is the CLAIMED signal in this grid and an
	   inert chip can sit on a tile that is still perfectly claimable. */
	.tile-chip.inert { opacity: 0.45; }
	/* A drawn rule, not `text-decoration: line-through`: half the chip is an
	   inline <svg> (AssetTypeIcon), an atomic inline-level box that CSS text
	   decorations are not painted over — the same reason .card-glyph.inert
	   draws its own line. Inset by the chip's padding so it reads as a strike
	   through the content, not a border across the chip. */
	.tile-chip.inert::before {
		content: '';
		position: absolute;
		left: 0.2rem;
		right: 0.2rem;
		top: 50%;
		border-top: 1px solid currentColor;
		pointer-events: none;
	}
	/* Whose asset it is, at zero layout cost. In flow this dot is the ~7px
	   that overflows a 3-up tile at the 440 cap, so it is absolutely
	   positioned into the corner — it still answers "from whom" during the
	   scan, and the name itself is in the expansion. The colour comes from
	   playerColor.ts at runtime (style:background), never a CSS token. */
	.chip-owner {
		position: absolute;
		top: -3px;
		right: -3px;
		width: 7px;
		height: 7px;
		border-radius: 50%;
		/* Ringed in the tile's own ground so the dot reads as a marker on the
		   chip rather than a stray pip between two of them. */
		border: 1px solid var(--color-surface-2);
		box-sizing: content-box;
	}

	/* (The weight meter itself moved to shared/WeightMeter.svelte in Session
	   3a — the TrackBoard needed the same object once its per-player card
	   values became weight marks.) */

	/* Tap-to-explore expansion (PROLOGUE_CHOOSING_REDESIGN_PLAN.md S2). Spans
	   the full row width (unlike the tile it follows, since Round 2 keeps the
	   tile in its own cell) and pulls up by the grid's own row gap so it sits
	   flush against the row above — the expanded tile's extra 1px overlap
	   (see .choice-btn.expanded) then fuses its border into this one. */
	.choice-detail {
		grid-column: 1 / -1;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		background: var(--color-surface-2);
		border: 1px solid var(--color-accent);
		border-radius: 6px;
		padding: 0.65rem 0.7rem;
		margin-top: calc(-1 * var(--tile-grid-gap));
	}
	.detail-desc {
		margin: 0;
		color: var(--color-text);
		font-size: 0.85rem;
		line-height: 1.4;
	}
	.detail-claimed {
		margin: 0;
		color: var(--color-text-muted);
		font-size: 0.8rem;
	}
	.detail-cards {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	/* One row per card slot: type icon · what it does · weight · track. The
	   take marking is the whole row here for the same reason it is the whole
	   chip on the tile (decision 5) — this is the row that says what you'd
	   gain and from whom. */
	.detail-card-row {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.2rem 0.35rem;
		border: 1px solid transparent;
		border-radius: 4px;
		color: var(--color-text-muted);
	}
	.detail-card-row.take {
		background: var(--color-chip-blue-bg);
		border-color: var(--color-chip-blue-border);
	}
	.detail-card-row.inert { opacity: 0.5; }
	.detail-card-text {
		flex: 1;
		min-width: 0;
		font-size: 0.85rem;
		line-height: 1.3;
		color: var(--color-text);
	}
	.detail-card-text :global(em) { font-style: italic; }
	/* The track word, quiet and small — the row's subject is the asset, and
	   the track is the consequence. Uppercased to echo the tile's code chip
	   without repeating the abbreviation the expansion exists to expand. */
	.detail-track {
		flex: none;
		font-size: 0.62rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}
	.rise { color: var(--color-success); margin-left: 0.15rem; }
	.detail-claim-btn {
		align-self: flex-start;
	}

	.done-btn.active { background: var(--color-success); }

</style>
