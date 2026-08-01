<!-- PrologueView.svelte
  Structured prologue (Phase 4b). Modes driven by game.prologue_ranking_step:

    null   →  choosing: pick boxes from the three sheets; cards make-or-take
    declare_X        →  hearts declaration for the current track
    place_set_asides_X →  top-ranked player slots zero-suit players in
    closing          →  "The Stage is Set": name your main character, ≤3p
                         create an extra peer, then ready up. All-ready
                         auto-advances to the main event.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/cardGlyph.css';
	import '$lib/components/shared/statusText.css';
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
	import { computeBrightHearts, cardRank } from '$lib/prologue/refund';
	import {
		openCount,
		cardHoldStates,
		ownedCardCount,
		sheetTrackProfile,
		stealPreview,
		trackLabel,
		assetTypeLabel,
		SUIT_MEANINGS,
		type CardHoldState,
	} from '$lib/prologue/choosing';
	import { notReadyPlayerIDs } from '$lib/prologue/closing';
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
	let turnNumber = $state(1);
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
			turnNumber = s.turn_number;
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

	const myTurns = $derived(claims.filter(c => c.player_id === currentPlayerID).length);
	const isMyTurn = $derived(activePlayerID != null && activePlayerID === currentPlayerID);
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

	function suitColor(s: string): 'red' | 'black' {
		return s === 'H' || s === 'D' ? 'red' : 'black';
	}

	// ── Everyone's hands (public during the prologue) ─────────────────────────
	// Cards are linked to public assets during the prologue, so every player's
	// hand is open information. Show them all as compact per-player tiles.
	// Hearts first so the wild cards cluster and stand out, then S/D/C, each
	// group high-to-low.
	const HAND_SUIT_ORDER: Record<string, number> = { H: 0, S: 1, D: 2, C: 3 };
	const handsByPlayer = $derived.by(() =>
		[...players]
			.sort((a, b) => (a.seat_order ?? 0) - (b.seat_order ?? 0))
			.map((p) => ({
				player: p,
				cards: cards
					.filter((c) => c.player_id === p.id)
					.sort(
						(a, b) =>
							HAND_SUIT_ORDER[a.card_suit] - HAND_SUIT_ORDER[b.card_suit] ||
							cardRank(b.card_value) - cardRank(a.card_value)
					),
			}))
	);

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

	async function onClaimSubmitted() {
		activeClaim = null;
		actionError = '';
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
			return {
				waitees: [{ kind: 'player', playerID: activePlayerID }],
				stepLabel: `Create assets — turn ${turnNumber} of ${players.length * 3}`,
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
			return { waitees, stepLabel: `Rankings: Spend ♥ for ${t.charAt(0).toUpperCase() + t.slice(1)}` };
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

{#snippet suitSvg(suit: string)}
	{#if suit === 'H'}
		<svg class="suit" width="10" height="10" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" aria-hidden="true"><path fill="currentColor" d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"/></svg>
	{:else if suit === 'D'}
		<svg class="suit" width="10" height="10" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" aria-hidden="true"><path fill="currentColor" d="M12 2 L22 12 L12 22 L2 12 Z"/></svg>
	{:else if suit === 'S'}
		<svg class="suit" width="10" height="10" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" aria-hidden="true"><path fill="currentColor" d="M12 2 C 6 9, 3 13, 3 16.5 A 3.5 3.5 0 0 0 10 17 L 9 22 L 15 22 L 14 17 A 3.5 3.5 0 0 0 21 16.5 C 21 13, 18 9, 12 2 Z"/></svg>
	{:else if suit === 'C'}
		<svg class="suit" width="10" height="10" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
			<circle cx="12" cy="8" r="5" fill="currentColor"/>
			<circle cx="8" cy="14" r="5" fill="currentColor"/>
			<circle cx="16" cy="14" r="5" fill="currentColor"/>
			<path fill="currentColor" d="M10 14 L 8.5 22 L 15.5 22 L 14 14 Z"/>
		</svg>
	{/if}
{/snippet}

{#snippet miniCard(value: string, suit: string, state: CardHoldState = 'fresh')}
	<span
		class="card-glyph"
		class:held={state === 'steal'}
		class:inert={state === 'mine'}
		data-color={suitColor(suit)}
	>
		<span class="mc-value">{value}</span>
		{@render suitSvg(suit)}
	</span>
{/snippet}

<!-- A bare suit symbol on the page ground (no parchment card face), for the
     sheet-header track profiles. Black suits take the body text colour —
     .card-glyph's black is the page background, which only works against
     parchment. Mirrors TrackBoard's .col-suit. -->
{#snippet bareSuit(suit: string)}
	<span class="bare-suit" data-color={suitColor(suit)}>{@render suitSvg(suit)}</span>
{/snippet}

<div class="prologue-view" bind:clientWidth={columnWidth}>
	<!-- Load errors sit at the top of the view: they explain the content
	     below being stale or missing. Action errors render beside the control
	     that raised them, further down in each mode's branch. -->
	{#if loadError}
		<ErrorText message={loadError} />
	{/if}

	{#if loading}
		<p class="muted-text">Loading prologue…</p>

	{:else if mode === 'choosing'}
		<p class="prologue-lede">
			We start the game with the prologue,
			where we take turns fleshing out our
			characters and the world they inhabit.
		</p>
		<p class="prologue-lede">
			One turn you might decide your character
			is the monarch, and then the next you
			might say that they hail from a castle on
			the coast.
		</p>

		{#if activePlayerID == null}
			<p class="muted-text">Everyone has finished choosing.</p>
		{/if}

		<!-- The choosing mode's one action path is the claim modal, which
		     closes before its follow-up refresh can fail — so this sits above
		     the tiles the player just came back to. -->
		{#if actionError}
			<ErrorText message={actionError} />
		{/if}

		<div class="prologue-intro">
			<h3>Your Retinue</h3>
			<p class="prologue-subtext">
				Each player will pick 3 tiles from the 3 categories below. 
				Each tile creates an asset and grants 2 playing cards. 
			</p>
			<p class="prologue-subtext"> 
				Playing cards let you create <span class="steal-color">or steal</span> another asset,
				and improve your rank in either Power, Knowledge, or Esteem.
			</p>
			<p class="prologue-subtext"> 
				You can edit your assets (including your main character) at any time in your player menu (top of the screen).
			</p>
			<!-- Every suit means two things at once: the asset it makes now, and
			     the ranking track it feeds when the prologue ends. The legend used
			     to teach only the first, while the track board below taught only
			     the second — and the two readings collide (♠ makes an artifact but
			     raises Esteem), so half a lesson was worse than none. -->
			<table class="suit-legend">
				<thead>
					<tr>
						<th scope="col">Suit</th>
						<th scope="col">You make a…</th>
						<th scope="col">Raises your…</th>
					</tr>
				</thead>
				<tbody>
					{#each SUIT_MEANINGS as m (m.suit)}
						<tr>
							<th scope="row">
								<span class="card-glyph legend-glyph" data-color={suitColor(m.suit)}>
									{@render suitSvg(m.suit)}
								</span>
							</th>
							<td>{m.assetType}</td>
							<td class:wild-cell={m.track == null}>{m.track ?? 'Wild — any track'}</td>
						</tr>
					{/each}
				</tbody>
			</table>
			<!-- No held/already-yours legend rows: both states are already spelled
			     out in words inside the tile expansion ("Takes X from Y" /
			     "You already hold X — nothing to take"), and repeating them here
			     cost two lines of scroll on the screen we're trying to shorten. -->
		</div>

		<p class="muted-text small">Your turns: {myTurns} of 3</p>


		<div class="sheet-accordion">
			{#each sheets as sheet (sheet.type)}
				{@const isOpen = openSheets.has(sheet.type)}
				{@const profile = sheetTrackProfile(sheet)}
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
						     Only the ranked suits: hearts are on every sheet (the
						     legend covers them) and the absent track is implied by the
						     two that are named. Rides the title row, which frees the
						     description to run full width below. -->
						<span class="sheet-tracks">
							{#each profile.tracks as s (s)}
								<span class="track-tag">{@render bareSuit(s)}{trackLabel(s)}</span>
							{/each}
						</span>
						<span class="sheet-open-count"><strong>{openCount(sheet, claims)}</strong> open</span>
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
											{@render miniCard(choice.cards[0].value, choice.cards[0].suit, stateOf(choice.cards[0].suit, choice.cards[0].value))}
											{@render miniCard(choice.cards[1].value, choice.cards[1].suit, stateOf(choice.cards[1].suit, choice.cards[1].value))}
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
											     it can't be dimmed the way a claimed tile is. -->
											<span class="choice-note">Both cards already yours</span>
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
													<div class="detail-card-row">
														{@render miniCard(c.value, c.suit, stateOf(c.suit, c.value))}
														<span class="detail-card-text">
															{#if !preview}
																Make a new {assetTypeLabel(c.suit)}
															{:else if expClaim}
																<!-- Claimed tiles can never be claimed again, so describe
																     where the card sits now instead of previewing a take
																     that will never happen. -->
																{#if preview.assetName}
																	<em>{preview.assetName}</em> — now held by {holderLabel(preview.ownerID)}
																{:else}
																	Now held by {holderLabel(preview.ownerID)}
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
																Takes <em>{preview.assetName}</em> from {preview.ownerName}
															{:else}
																Already held by {preview.ownerName}
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

		<section class="hands-section">
			<h3>Hands</h3>
			<div class="hands-grid">
				{#each handsByPlayer as hand (hand.player.id)}
					<div class="hand-tile" class:you={hand.player.id === currentPlayerID}>
						<div class="hand-tile-head">
							<span class="hand-tile-name">{hand.player.display_name}</span>
							<span class="hand-tile-count">{hand.cards.length}</span>
						</div>
						{#if hand.cards.length === 0}
							<span class="hand-tile-empty">No cards yet</span>
						{:else}
							<div class="hand-tile-cards">
								{#each hand.cards as c}
									{@render miniCard(c.card_value, c.card_suit)}
								{/each}
							</div>
						{/if}
					</div>
				{/each}
			</div>
		</section>

		<div class="prologue-intro">
			<h3>Starting rankings</h3>
			<p class="prologue-subtext">
				The playing cards set the initial rankings. You will choose tracks to spend your 
				<span class="heart-mark" style="color: var(--color-suit-red);">♥</span> Hearts on.
			</p>
		</div>

		<TrackBoard
			{players}
			{cards}
			{rankings}
			{committed}
			{doneFlags}
			activeTrack={null}
			{currentPlayerID}
		/>

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
			<p class="muted-text small">
				Once every player marks Done, this track resolves: hearts doing work lock in, the rest return to your hand.
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
	.prologue-view h3 { color: var(--color-accent); font-size: 1rem; margin: 0.5rem 0 0.25rem; }

	.prologue-intro {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
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
	/* Matches the steal ring on the card glyphs — blue/attention, because a
	   take is an opportunity for the reader, not a warning to them. See the
	   .card-glyph.held comment in shared/cardGlyph.css for the full reasoning. */
	.steal-color { color: var(--color-highlight); }

	/* Genuinely tabular (one row per suit, two independent readings per row),
	   so a real table — the column headers are what make the second reading
	   legible, and they carry to screen readers for free. */
	.suit-legend {
		border-collapse: collapse;
		/* Content-sized, not stretched: a full-width table pushes the three
		   columns apart until the row stops reading as one statement. Shrunk
		   to its contents, the block centres visibly and each row scans as
		   "♣ → Holding → Power". */
		width: max-content;
		max-width: 100%;
		margin-inline: auto;
		font-size: 0.9rem;
		color: var(--color-text-secondary);
		text-align: left;
	}
	.suit-legend th,
	.suit-legend td {
		padding: 0.15rem 0.75rem 0.15rem 0;
		font-weight: inherit;
		vertical-align: middle;
	}
	.suit-legend th:last-child,
	.suit-legend td:last-child { padding-right: 0; }
	.suit-legend thead th {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-muted);
		padding-bottom: 0.3rem;
	}
	.suit-legend tbody td:last-child { color: var(--color-text); }
	/* Hearts rank nothing on their own — they're declared as a suit later — so
	   the cell reads as a note, not a track name. */
	.suit-legend tbody td.wild-cell { color: var(--color-text-secondary); font-style: italic; }

	.legend-glyph {
		padding: 0.25rem 0.4rem;
		font-size: 1rem;
	}
	.legend-glyph :global(.suit) { width: 1.15em; height: 1.15em; }

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
			'name tracks count caret'
			'desc desc   desc  desc';
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
	.sheet-open-count {
		grid-area: count;
		color: var(--color-text-muted);
		font-size: 0.8rem;
		white-space: nowrap;
	}

	/* Right-aligned against the open-count, so the three headers' profiles
	   line up as a column the eye can compare down. */
	.sheet-tracks {
		grid-area: tracks;
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		align-items: center;
		gap: 0.2rem 0.55rem;
		font-size: 0.72rem;
	}
	.track-tag {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		color: var(--color-text-secondary);
		white-space: nowrap;
	}
	.bare-suit {
		display: inline-flex;
		align-items: center;
		line-height: 1;
	}
	.bare-suit[data-color='red'] { color: var(--color-suit-red); }
	.bare-suit[data-color='black'] { color: var(--color-text); }
	.bare-suit :global(.suit) { width: 0.85em; height: 0.85em; }

	.sheet-caret {
		grid-area: caret;
		color: var(--color-accent);
		font-size: 0.75rem;
		/* Points right when collapsed; rotates down to ▾ on open. */
		transform: rotate(-90deg);
		transition: transform 0.15s ease;
	}
	.sheet-panel.open .sheet-caret { transform: rotate(0); }

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
	.choice-cards { display: flex; gap: 0.3rem; flex-wrap: wrap; }
	.choice-note {
		color: var(--color-text-muted);
		font-size: 0.7rem;
		font-style: italic;
		line-height: 1.2;
	}

	.card-glyph :global(.suit) { width: 1em; height: 1em; flex: none; display: inline-block; vertical-align: middle; }

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
	.detail-card-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.detail-card-text {
		font-size: 0.85rem;
		color: var(--color-text);
	}
	.detail-card-text :global(em) { font-style: italic; }
	.detail-claim-btn {
		align-self: flex-start;
	}

	.hands-section {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.hands-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.4rem;
		max-width: 32rem;
	}
	.hand-tile {
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		padding: 0.5rem 0.6rem;
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		min-width: 0;
	}
	.hand-tile.you {
		outline: 1px solid var(--color-accent);
		outline-offset: -1px;
		background: color-mix(in srgb, var(--color-accent) 6%, transparent);
	}
	.hand-tile-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.4rem;
	}
	.hand-tile-name {
		color: var(--color-text);
		font-size: 0.85rem;
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}
	.hand-tile.you .hand-tile-name { color: var(--color-accent); }
	.hand-tile-count { color: var(--color-text-muted); font-size: 0.75rem; flex: none; }
	.hand-tile-empty { color: var(--color-text-faint); font-size: 0.8rem; font-style: italic; }
	.hand-tile-cards {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem;
	}
	.hand-tile-cards .card-glyph { font-size: 0.85rem; padding: 0.15rem 0.3rem; }
	.hand-tile-cards .card-glyph :global(.suit) { width: 1.1em; height: 1.1em; }

	.done-btn.active { background: var(--color-success); }

</style>
