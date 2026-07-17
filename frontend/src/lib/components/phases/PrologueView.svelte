<!-- PrologueView.svelte
  Structured prologue (Phase 4b). Three modes driven by game.prologue_ranking_step:

    null   →  choosing: pick boxes from the three sheets; cards make-or-take
    declare_X        →  hearts declaration for the current track
    place_set_asides_X →  top-ranked player slots zero-suit players in
    extra_peers      →  ≤3-player rule: each picks one unused title
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/cardGlyph.css';
	import '$lib/components/shared/statusText.css';
	import {
		getPrologueSheets,
		getPrologueCards,
		placePrologueSetAsides,
		createExtraPeer,
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
	} from '$lib/api';
	import { onMount, onDestroy, tick } from 'svelte';
	import ClaimChoiceModal from './ClaimChoiceModal.svelte';
	import TrackBoard from './prologue/TrackBoard.svelte';
	import HandStrip from './prologue/HandStrip.svelte';
	import SetAsidePlacer from './prologue/SetAsidePlacer.svelte';
	import { computeBrightHearts, cardRank } from '$lib/prologue/refund';
	import { openCount, heldCardSet, stealPreview } from '$lib/prologue/choosing';
	import type { WaitingOnState, Waitee } from '$lib/waitingOn';
	import { playerColorByID } from '$lib/playerColor';
	import CrownGlyph from '../CrownGlyph.svelte';
	import type { CrownMark } from '$lib/succession';
	import { TEXT_LIMITS } from '$lib/textLimits';

	interface Props {
		gameID: string;
		game: Game;
		players: Player[];
		assets: Asset[];
		rankings: Ranking[];
		currentPlayerID: number | null;
		isFacilitator: boolean;
		waitingOn: WaitingOnState;
		onResync?: () => void;
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
		onResync,
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
	let error = $state('');
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
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load prologue data.';
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

	const heldCards = $derived(heldCardSet(cards));

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

	function cardTypeLabel(suit: string): string {
		switch (suit) {
			case 'C': return 'holding';
			case 'D': return 'resource';
			case 'S': return 'artifact';
			case 'H': return 'peer';
			default:  return 'asset';
		}
	}

	// ── Choose a box ─────────────────────────────────────────────────────────
	let activeClaim = $state<{ sheet: PrologueSheet; choice: PrologueSheet['choices'][number] } | null>(null);

	function openClaimModal(sheet: PrologueSheet, choice: PrologueSheet['choices'][number]) {
		if (activeClaim) return;
		activeClaim = { sheet, choice };
	}

	async function onClaimSubmitted() {
		activeClaim = null;
		try {
			const [, assetData] = await Promise.all([reload(), listAssets(gameID)]);
			assets = assetData.assets;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not refresh.';
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
			// extra_peers or beyond — all resolved.
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
		error = '';
		try {
			let next = myCommittedOnTrack.slice();
			if (retract) {
				next = next.filter((id) => id !== cardID);
			} else if (!next.includes(cardID)) {
				next.push(cardID);
			}
			await commitTrackHearts(gameID, currentTrack as PrologueTrack, next);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update hearts.';
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
		error = '';
		try {
			await setPrologueDone(gameID, currentTrack as PrologueTrack, !myDoneOnTrack);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update done.';
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
		error = '';
		try {
			await placePrologueSetAsides(gameID, setAsideOrdering);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not place set-asides.';
		} finally {
			placing = false;
		}
	}

	// ── Extra peer (≤3 players) ──────────────────────────────────────────────
	const titlesSheet = $derived(sheets.find(s => s.type === 'titles'));
	const extraTitlesClaimed = $derived(new Set(extraPeers.map(p => p.title_name)));
	const unclaimedTitles = $derived.by(() => {
		const t = titlesSheet;
		if (!t) return [];
		return t.choices.filter(c =>
			!claimMap.has(`titles::${c.name}`) && !extraTitlesClaimed.has(c.name)
		);
	});
	const myExtraPeer = $derived(
		currentPlayerID == null ? null : extraPeers.find(p => p.player_id === currentPlayerID) ?? null
	);

	let extraPeerName = $state('');
	let extraPeerText = $state('');
	let creatingExtra = $state(false);
	async function submitExtraPeer() {
		if (!extraPeerName || !extraPeerText.trim() || creatingExtra) return;
		creatingExtra = true;
		error = '';
		try {
			const result = await createExtraPeer(gameID, extraPeerName, extraPeerText.trim());
			assets = [...assets, result.asset];
			extraPeerName = '';
			extraPeerText = '';
			reload();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not create extra peer.';
			onResync?.();
			reload();
		} finally {
			creatingExtra = false;
		}
	}

	// The peer name starts blank and is authored by the player — no bracketed
	// `[Title]` auto-fill (ADR-007 §7); the input placeholder hints instead.

	// ── Phase classification ─────────────────────────────────────────────────
	type Mode = 'choosing' | 'declare' | 'place' | 'extra';
	const mode = $derived.by<Mode>(() => {
		const step = game.prologue_ranking_step;
		if (!step) return 'choosing';
		if (step.startsWith('declare_')) return 'declare';
		if (step.startsWith('place_set_asides_')) return 'place';
		return 'extra';
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
		// extra
		const notDone = players
			.filter(p => !extraPeers.some(e => e.player_id === p.id))
			.map<Waitee>(p => ({ kind: 'player', playerID: p.id }));
		if (notDone.length === 0) return { waitees: [] };
		const waitees: Waitee[] = notDone.length === players.length
			? [{ kind: 'everyone' }]
			: notDone;
		return { waitees, stepLabel: 'Create an extra peer' };
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

{#snippet miniCard(value: string, suit: string, held = false)}
	<span class="card-glyph" class:held data-color={suitColor(suit)}>
		<span class="mc-value">{value}</span>
		{@render suitSvg(suit)}
	</span>
{/snippet}

<div class="prologue-view" bind:clientWidth={columnWidth}>
	{#if error}
		<p class="error-text">{error}</p>
	{/if}

	{#if loading}
		<p class="muted-text">Loading prologue…</p>

	{:else if mode === 'choosing'}
		<p class="prologue-lede">
			To set the stage, we create our main character's assets.
		</p>

		{#if activePlayerID == null}
			<p class="muted-text">Everyone has finished choosing.</p>
		{/if}

		<div class="prologue-intro">
			<h3>Your Retinue</h3>
			<p class="prologue-subtext">
				Each box in the 3 categories below creates an asset and grants two playing cards, and lets you create <span class="steal-color">or steal</span> another asset.
			</p>
			<p class="prologue-subtext"> 
				You can edit your assets (including your main character) at any time in your <em>Retinue</em> (top of the screen).
			</p>
			<div class="suit-legend">
				<div class="suit-legend-item">
					<span class="card-glyph legend-glyph" data-color="red">{@render suitSvg('H')}</span>
					<span>Peer</span>
				</div>
				<div class="suit-legend-item">
					<span class="card-glyph legend-glyph" data-color="black">{@render suitSvg('S')}</span>
					<span>Artifact</span>
				</div>
				<div class="suit-legend-item">
					<span class="card-glyph legend-glyph" data-color="red">{@render suitSvg('D')}</span>
					<span>Resource</span>
				</div>
				<div class="suit-legend-item">
					<span class="card-glyph legend-glyph" data-color="black">{@render suitSvg('C')}</span>
					<span>Holding</span>
				</div>
			</div>
			<div class="suit-legend-item held-legend">
				<span class="card-glyph held legend-glyph" data-color="red">{@render suitSvg('H')}</span>
				<span class="steal-color">Already held — claiming it steals that asset</span>
			</div>
		</div>

		<p class="muted-text small">Your turns: {myTurns} of 3</p>


		<div class="sheet-accordion">
			{#each sheets as sheet (sheet.type)}
				{@const isOpen = openSheets.has(sheet.type)}
				<section class="sheet-panel" class:open={isOpen}>
					<button
						type="button"
						class="sheet-header"
						aria-expanded={isOpen}
						aria-controls={isOpen ? `sheet-body-${sheet.type}` : undefined}
						onclick={(e: MouseEvent) => toggleSheetPanel(sheet.type, e.currentTarget as HTMLButtonElement)}
					>
						<span class="sheet-header-main">
							<span class="sheet-name">{sheet.display_name}</span>
							<span class="sheet-desc">{SHEET_DESCRIPTIONS[sheet.type]}</span>
						</span>
						<span class="sheet-open-count"><strong>{openCount(sheet, claims)}</strong> open</span>
						<span class="sheet-caret" aria-hidden="true">▾</span>
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
											{@render miniCard(choice.cards[0].value, choice.cards[0].suit, heldCards.has(`${choice.cards[0].suit}::${choice.cards[0].value}`))}
											{@render miniCard(choice.cards[1].value, choice.cards[1].suit, heldCards.has(`${choice.cards[1].suit}::${choice.cards[1].value}`))}
										</span>
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
														{@render miniCard(c.value, c.suit, preview != null)}
														<span class="detail-card-text">
															{#if !preview}
																Make a new {cardTypeLabel(c.suit)}
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
													Claim this box
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
		{/if}

	{:else if mode === 'extra'}
		<p class="muted-text">
			Extra peers: with three or fewer players, each player picks one unused title to flesh out the cast.
		</p>

		<ul class="extra-status">
			{#each players as p}
				{@const claim = extraPeers.find(e => e.player_id === p.id)}
				<li class:done={claim != null}>
					<span class="extra-name">{p.display_name}</span>
					{#if claim}
						<span class="extra-claim">✓ {claim.title_name}</span>
					{:else}
						<span class="extra-pending">waiting…</span>
					{/if}
				</li>
			{/each}
		</ul>

		{#if myExtraPeer}
			<p class="muted-text small">You created your extra peer: {myExtraPeer.title_name}.</p>
		{:else}
			<div class="extra-form">
				<div class="extra-title">
					<span class="extra-title-label">Title:</span>
					{#if unclaimedTitles.length === 0}
						<p class="muted-text small" style="margin:0;">No titles remain.</p>
					{:else}
						<div class="title-chip-row">
							{#each unclaimedTitles as t}
								<button
									type="button"
									class="title-chip"
									class:active={extraPeerName === t.name}
									onclick={() => (extraPeerName = extraPeerName === t.name ? '' : t.name)}
								>{t.name}</button>
							{/each}
						</div>
					{/if}
				</div>
			</div>
			<label>
				Peer name:
				<input
					type="text"
					bind:value={extraPeerText}
					class="peer-input"
					placeholder="Name your peer"
					maxlength={TEXT_LIMITS.NAME}
				/>
			</label>
			<button class="action-btn secondary" onclick={submitExtraPeer} disabled={!extraPeerName || !extraPeerText.trim() || creatingExtra}>
				{creatingExtra ? '…' : 'Create peer'}
			</button>
		{/if}

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
	/* Orange warning family, not danger red (Round 2 of the redesign plan):
	   stealing is a careful-now signal, not the at-risk/near-destruction one. */
	.steal-color { color: var(--color-warning); }

	.suit-legend {
		display: grid;
		grid-template-columns: repeat(2, auto);
		gap: 0.4rem 1.5rem;
	}
	.suit-legend-item {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		font-size: 0.9rem;
		color: var(--color-text-secondary);
	}
	.legend-glyph {
		padding: 0.25rem 0.4rem;
		font-size: 1rem;
	}
	.legend-glyph :global(.suit) { width: 1.15em; height: 1.15em; }
	.held-legend { margin-top: 0.1rem; }

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

	.sheet-header {
		display: flex;
		align-items: center;
		gap: 0.5rem;
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
	.sheet-header-main {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		flex: 1;
		min-width: 0;
	}
	.sheet-name { color: var(--color-accent); font-size: 0.95rem; }
	.sheet-desc {
		color: var(--color-text-secondary);
		font-size: 0.78rem;
		line-height: 1.3;
	}
	.sheet-open-count {
		flex: none;
		color: var(--color-text-muted);
		font-size: 0.8rem;
		white-space: nowrap;
	}
	.sheet-caret {
		flex: none;
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

	.extra-status {
		list-style: none;
		padding: 0;
		margin: 0.25rem 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		max-width: 24rem;
	}
	.extra-status li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
		padding: 0.35rem 0.5rem;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-surface-2);
		border-radius: 4px;
		font-size: 0.85rem;
	}
	.extra-status li.done { border-color: var(--color-chip-green-border); }
	.extra-name { color: var(--color-text); }
	.extra-claim { color: var(--color-success); font-size: 0.8rem; }
	.extra-pending { color: var(--color-text-faint); font-size: 0.8rem; font-style: italic; }

	.extra-form {
		display: flex;
		gap: 0.6rem;
		align-items: end;
		margin: 0.5rem 0;
	}
	.peer-input {
		max-width: 20rem;
		margin-top: 0.25rem;
	}
	.extra-title {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}
	.extra-title-label {
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}
	.title-chip-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
	}
	.title-chip {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		padding: 0.35rem 0.85rem;
		border-radius: 999px;
		border: 1px solid var(--color-neutral);
		background: var(--color-surface-2);
		color: var(--color-text);
		font-size: 0.9rem;
		cursor: pointer;
	}
	.title-chip.active {
		border-color: var(--color-accent);
		background: var(--color-chip-gold-bg);
	}
	.title-chip:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}

</style>
