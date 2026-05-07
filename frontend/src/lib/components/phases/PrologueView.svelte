<!-- PrologueView.svelte
  Structured prologue (Phase 4b). Three modes driven by game.prologue_ranking_step:

    null   →  choosing: pick boxes from the three sheets; cards make-or-take
    declare_X        →  hearts declaration for the current track
    place_set_asides_X →  rank-1 player slots zero-suit players in
    extra_peers      →  ≤3-player rule: each picks one unused title
-->
<script lang="ts">
	import {
		startMainEvent,
		getPrologueSheets,
		getPrologueCards,
		beginPrologueRanking,
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
	import { onMount, onDestroy } from 'svelte';
	import ClaimChoiceModal from './ClaimChoiceModal.svelte';
	import TrackBoard from './prologue/TrackBoard.svelte';
	import HandStrip from './prologue/HandStrip.svelte';
	import SetAsidePlacer from './prologue/SetAsidePlacer.svelte';
	import { computeBrightHearts } from '$lib/prologue/refund';

	interface Props {
		gameID: string;
		game: Game;
		players: Player[];
		assets: Asset[];
		rankings: Ranking[];
		currentPlayerID: number | null;
		isFacilitator: boolean;
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
		onResync,
	}: Props = $props();

	// ── Loaded reference data ────────────────────────────────────────────────
	let sheets = $state<PrologueSheet[]>([]);
	let claims = $state<PrologueClaim[]>([]);
	let cards = $state<PlayerCardRow[]>([]);
	let activePlayerID = $state<number | null>(null);
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

	const myTurns = $derived(claims.filter(c => c.player_id === currentPlayerID).length);
	const isMyTurn = $derived(activePlayerID != null && activePlayerID === currentPlayerID);
	const everyoneFinishedChoosing = $derived(
		players.length > 0 && players.every(p => claims.filter(c => c.player_id === p.id).length >= 3)
	);

	function playerName(id: number | null): string {
		if (id == null) return 'Dummy';
		return players.find(p => p.id === id)?.display_name ?? '?';
	}

	// ── My hand ──────────────────────────────────────────────────────────────
	const myCards = $derived(cards.filter(c => c.player_id === currentPlayerID));

	function suitColor(s: string): 'red' | 'black' {
		return s === 'H' || s === 'D' ? 'red' : 'black';
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

	// ── Begin ranking ────────────────────────────────────────────────────────
	let beginning = $state(false);
	async function onBeginRanking() {
		if (beginning) return;
		beginning = true;
		error = '';
		try {
			await beginPrologueRanking(gameID);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not begin ranking.';
		} finally {
			beginning = false;
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

	const isMyTurnForSetAsides = $derived.by(() => {
		const t = currentTrack;
		if (!t) return false;
		const r1 = rankings.find(r => r.category === t && r.rank === 1);
		return r1?.player_id === currentPlayerID;
	});

	let setAsideOrdering = $state<number[]>([]);
	$effect(() => {
		// Initialize ordering from set-asides whenever they change.
		setAsideOrdering = [...setAsidePlayers];
	});

	const rank1PlayerID = $derived.by(() => {
		const t = currentTrack;
		if (!t) return null;
		const r = rankings.find((rr) => rr.category === t && rr.rank === 1);
		return r?.player_id ?? null;
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

	$effect(() => {
		// Default the peer text to the bracketed title when a title is selected.
		if (extraPeerName && !extraPeerText) extraPeerText = `[${extraPeerName}]`;
	});

	// ── Start main event ─────────────────────────────────────────────────────
	let advancing = $state(false);
	async function advanceToMainEvent() {
		if (advancing) return;
		advancing = true;
		error = '';
		try {
			await startMainEvent(gameID);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not start main event.';
		} finally {
			advancing = false;
		}
	}

	// ── Phase classification ─────────────────────────────────────────────────
	type Mode = 'choosing' | 'declare' | 'place' | 'extra';
	const mode = $derived.by<Mode>(() => {
		const step = game.prologue_ranking_step;
		if (!step) return 'choosing';
		if (step.startsWith('declare_')) return 'declare';
		if (step.startsWith('place_set_asides_')) return 'place';
		return 'extra';
	});
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

{#snippet miniCard(value: string, suit: string)}
	<span class="mini-card" data-color={suitColor(suit)}>
		<span class="mc-value">{value}</span>
		{@render suitSvg(suit)}
	</span>
{/snippet}

<div class="prologue-view">
	<h2>Prologue</h2>

	{#if error}
		<p class="local-error">{error}</p>
	{/if}

	{#if loading}
		<p class="muted">Loading prologue…</p>

	{:else if mode === 'choosing'}
		<p class="muted">
			Each player takes three turns claiming boxes from the three sheets. Each box creates an asset
			and grants two playing cards (which create or transfer card-linked assets).
		</p>

		{#if cards.length > 0}
			<TrackBoard
				{players}
				{cards}
				{rankings}
				{committed}
				{doneFlags}
				activeTrack={'power'}
				{currentPlayerID}
			/>
		{/if}

		<div class="turn-banner" class:my-turn={isMyTurn}>
			{#if isMyTurn}
				<strong>Your turn.</strong> Claim a box from any sheet.
			{:else if activePlayerID != null}
				Waiting on <strong>{playerName(activePlayerID)}</strong>…
			{:else}
				Everyone has finished choosing.
			{/if}
		</div>

		<div class="choosing-grid">
			{#each sheets as sheet}
				<section class="sheet-panel">
					<h3>{sheet.display_name}</h3>
					<p class="muted small">Creates a {sheet.choice_asset_type}.</p>
					<div class="choice-list">
						{#each sheet.choices as choice}
							{@const existingClaim = claimMap.get(`${sheet.type}::${choice.name}`)}
							<button
								type="button"
								class="choice-btn"
								class:claimed={!!existingClaim}
								disabled={!!existingClaim || !isMyTurn || activeClaim != null}
								title={choice.description || ''}
								onclick={() => openClaimModal(sheet, choice)}
							>
								<span class="choice-name">{choice.name}</span>
								<span class="choice-cards">
									{@render miniCard(choice.cards[0].value, choice.cards[0].suit)}
									{@render miniCard(choice.cards[1].value, choice.cards[1].suit)}
								</span>
								{#if existingClaim}
									<span class="claim-by">— {playerName(existingClaim.player_id)}</span>
								{/if}
							</button>
						{/each}
					</div>
				</section>
			{/each}
		</div>

		<section class="hand-panel">
			<h3>Your Hand ({myCards.length} card{myCards.length === 1 ? '' : 's'})</h3>
			{#if myCards.length === 0}
				<p class="muted small">No cards yet — claim a box to start collecting them.</p>
			{:else}
				<div class="hand-cards">
					{#each myCards as c}
						{@render miniCard(c.card_value, c.card_suit)}
					{/each}
				</div>
			{/if}
		</section>

		<p class="muted small">
			Turns taken: {myTurns} / 3.
		</p>

		{#if isFacilitator}
			<button
				class="primary"
				onclick={onBeginRanking}
				disabled={!everyoneFinishedChoosing || beginning}
				title={!everyoneFinishedChoosing ? 'Every player must take 3 turns first' : undefined}
			>
				{beginning ? '…' : 'Begin Ranking'}
			</button>
		{/if}

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
				class="primary done-btn"
				class:active={myDoneOnTrack}
				disabled={savingDone}
				onclick={toggleDone}
			>
				{savingDone ? '…' : myDoneOnTrack ? 'Done ✓ (tap to undo)' : "I'm done"}
			</button>
			<p class="muted small">
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
			{#if rank1PlayerID != null && setAsideOrdering.length > 0}
				<SetAsidePlacer
					{players}
					{setAsideOrdering}
					openRanks={setAsideOpenRanks}
					rank1PlayerID={rank1PlayerID}
					isMyTurn={isMyTurnForSetAsides}
					busy={placing}
					onReorder={(next) => (setAsideOrdering = next)}
					onConfirm={submitSetAsides}
				/>
			{/if}
		{/if}

	{:else if mode === 'extra'}
		<p class="muted">
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
			<p class="muted small">You created your extra peer: <strong>{myExtraPeer.title_name}</strong>.</p>
		{:else}
			<div class="extra-form">
				<label>
					Title:
					<select bind:value={extraPeerName}>
						<option value="">— pick —</option>
						{#each unclaimedTitles as t}
							<option value={t.name}>{t.name}</option>
						{/each}
					</select>
				</label>
				{#if extraPeerName}
					<label>
						Peer name:
						<input
							type="text"
							bind:value={extraPeerText}
							placeholder={`[${extraPeerName}]`}
						/>
					</label>
				{/if}
				<button class="secondary" onclick={submitExtraPeer} disabled={!extraPeerName || !extraPeerText.trim() || creatingExtra}>
					{creatingExtra ? '…' : 'Create peer'}
				</button>
			</div>
		{/if}

	{/if}

	{#if isFacilitator && rankings.length >= 15}
		<button
			class="primary"
			onclick={advanceToMainEvent}
			disabled={advancing}
		>
			{advancing ? '…' : 'Start Main Event'}
		</button>
	{/if}
</div>

{#if activeClaim}
	<ClaimChoiceModal
		{gameID}
		sheet={activeClaim.sheet}
		choice={activeClaim.choice}
		cards={cards}
		onClose={() => activeClaim = null}
		onSubmitted={onClaimSubmitted}
	/>
{/if}

<style>
	.prologue-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		padding: 1rem 0;
		gap: 1rem;
		overflow-y: auto;
		min-height: 0;
	}
	.prologue-view h2 { color: #c8a96e; font-size: 1.3rem; margin: 0; }
	.prologue-view h3 { color: #c8a96e; font-size: 1rem; margin: 0.5rem 0 0.25rem; }

	.muted { color: #999; font-size: 0.9rem; margin: 0; }
	.muted.small { font-size: 0.8rem; }
	.local-error { color: #e07070; font-size: 0.85rem; margin: 0; }

	.turn-banner {
		background: #1e1e1e;
		border: 1px solid #333;
		border-left: 3px solid #555;
		border-radius: 6px;
		padding: 0.5rem 0.75rem;
		font-size: 0.9rem;
		color: #ccc;
	}
	.turn-banner.my-turn {
		border-left-color: #c8a96e;
		background: #2a2418;
		color: #e8e4d9;
	}
	.choosing-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.4rem;
	}
	@media (min-width: 600px) {
		.choosing-grid { gap: 0.75rem; }
	}

	.sheet-panel {
		background: #1e1e1e;
		border: 1px solid #333;
		border-radius: 8px;
		padding: 0.4rem;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		min-width: 0;
	}
	@media (min-width: 600px) {
		.sheet-panel { padding: 0.75rem; gap: 0.4rem; }
	}

	.choice-list {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.choice-btn {
		text-align: left;
		background: #2a2a2a;
		color: #e8e4d9;
		border: 1px solid #444;
		border-radius: 4px;
		padding: 0.3rem 0.4rem;
		font-size: 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		cursor: pointer;
		min-width: 0;
	}
	@media (min-width: 600px) {
		.choice-btn { padding: 0.4rem 0.6rem; font-size: 0.85rem; }
	}

	.choice-btn.claimed { opacity: 0.5; cursor: default; }

	.choice-name { font-weight: 600; color: #c8a96e; line-height: 1.15; word-break: break-word; }
	.choice-cards { display: flex; gap: 0.25rem; flex-wrap: wrap; }
	.claim-by { font-size: 0.7rem; color: #888; }

	.mini-card {
		display: inline-flex;
		align-items: center;
		gap: 0.15rem;
		background: #f4ecd8;
		border: 1px solid #888;
		border-radius: 3px;
		padding: 0.1rem 0.25rem;
		font-size: 0.75rem;
		font-weight: 700;
		line-height: 1;
		min-width: 1.6em;
		justify-content: center;
	}
	.mini-card[data-color='red']   { color: #b03030; }
	.mini-card[data-color='black'] { color: #1a1a1a; }
	.mini-card .mc-value { font-variant-numeric: tabular-nums; }
	.mini-card :global(.suit) { width: 1em; height: 1em; flex: none; display: inline-block; vertical-align: middle; }
	.hand-cards .mini-card { font-size: 0.85rem; padding: 0.15rem 0.3rem; }
	.hand-cards .mini-card :global(.suit) { width: 1.1em; height: 1.1em; }

	.hand-panel {
		background: #1e1e1e;
		border: 1px solid #333;
		border-radius: 8px;
		padding: 0.75rem;
	}

	.hand-cards {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		margin-top: 0.4rem;
	}

	.done-btn { align-self: flex-start; min-height: 44px; }
	.done-btn.active { background: #6cbf6c; }

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
		background: #1e1e1e;
		border: 1px solid #2a2a2a;
		border-radius: 4px;
		font-size: 0.85rem;
	}
	.extra-status li.done { border-color: #3d4d3d; }
	.extra-name { color: #e8e4d9; }
	.extra-claim { color: #6cbf6c; font-size: 0.8rem; }
	.extra-pending { color: #777; font-size: 0.8rem; font-style: italic; }

	.extra-form {
		display: flex;
		gap: 0.6rem;
		align-items: end;
		margin: 0.5rem 0;
	}
	.extra-form label {
		display: flex;
		flex-direction: column;
		font-size: 0.85rem;
		color: #aaa;
	}
	.extra-form select {
		background: #333;
		color: #e8e4d9;
		border: 1px solid #555;
		border-radius: 4px;
		padding: 0.3rem 0.5rem;
		font-size: 0.9rem;
	}

	.primary {
		background: #c8a96e;
		color: #1a1a1a;
		font-weight: 600;
		padding: 0.5rem 1rem;
		border-radius: 6px;
		align-self: flex-start;
	}
	.primary:disabled { opacity: 0.4; cursor: not-allowed; }

	.secondary {
		background: #333;
		color: #e8e4d9;
		font-weight: 600;
		padding: 0.4rem 0.8rem;
		border-radius: 6px;
		align-self: flex-start;
		border: 1px solid #555;
	}
	.secondary:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
