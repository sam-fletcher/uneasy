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
		setSeats,
		getPrologueSheets,
		getPrologueCards,
		choosePrologue,
		beginPrologueRanking,
		declareHearts,
		finalizeTrackRanking,
		placePrologueSetAsides,
		createExtraPeer,
		listAssets,
	} from '$lib/api';
	import type {
		Game,
		Player,
		Asset,
		Marginalium,
		Ranking,
		RankingCategory,
		PrologueSheet,
		PrologueClaim,
		PlayerCardRow,
		PrologueSheetType,
	} from '$lib/api';
	import { tearMarginalia } from '$lib/api';
	import AssetCard from '$lib/components/AssetCard.svelte';
	import { onMount, onDestroy } from 'svelte';

	interface Props {
		gameID: string;
		game: Game;
		players: Player[];
		assets: Asset[];
		rankings: Ranking[];
		currentPlayerID: number | null;
		isFacilitator: boolean;
	}

	let {
		gameID,
		game,
		players = $bindable(),
		assets = $bindable(),
		rankings = $bindable(),
		currentPlayerID,
		isFacilitator,
	}: Props = $props();

	// ── Loaded reference data ────────────────────────────────────────────────
	let sheets = $state<PrologueSheet[]>([]);
	let claims = $state<PrologueClaim[]>([]);
	let cards = $state<PlayerCardRow[]>([]);
	let error = $state('');
	let loading = $state(true);

	async function reload() {
		try {
			const [s, c] = await Promise.all([getPrologueSheets(gameID), getPrologueCards(gameID)]);
			sheets = s.sheets;
			claims = s.claims;
			cards = c.cards;
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
		window.addEventListener('uneasy:prologue.ranking_step_changed', onStepChanged);
		window.addEventListener('uneasy:prologue.hearts_declared', onClaimEvent);
		window.addEventListener('uneasy:prologue.track_ranked', onStepChanged);
		window.addEventListener('uneasy:prologue.set_asides_placed', onStepChanged);
	});
	onDestroy(() => {
		window.removeEventListener('uneasy:prologue.choice_claimed', onClaimEvent);
		window.removeEventListener('uneasy:prologue.ranking_step_changed', onStepChanged);
		window.removeEventListener('uneasy:prologue.hearts_declared', onClaimEvent);
		window.removeEventListener('uneasy:prologue.track_ranked', onStepChanged);
		window.removeEventListener('uneasy:prologue.set_asides_placed', onStepChanged);
	});

	// ── Derived: claim lookup ────────────────────────────────────────────────
	const claimMap = $derived.by(() => {
		const m = new Map<string, PrologueClaim>();
		for (const c of claims) m.set(`${c.sheet_type}::${c.choice_name}`, c);
		return m;
	});

	const myTurns = $derived(claims.filter(c => c.player_id === currentPlayerID).length);
	const everyoneFinishedChoosing = $derived(
		players.length > 0 && players.every(p => claims.filter(c => c.player_id === p.id).length >= 3)
	);

	function playerName(id: number | null): string {
		if (id == null) return 'Dummy';
		return players.find(p => p.id === id)?.display_name ?? '?';
	}

	// ── My hand ──────────────────────────────────────────────────────────────
	const myCards = $derived(cards.filter(c => c.player_id === currentPlayerID));

	function suitGlyph(s: string): string {
		switch (s) {
			case 'C': return '♣';
			case 'D': return '♦';
			case 'S': return '♠';
			case 'H': return '♥';
		}
		return '?';
	}

	// ── Choose a box ─────────────────────────────────────────────────────────
	let claimingName = $state('');
	async function claimChoice(sheetType: PrologueSheetType, choiceName: string) {
		if (claimingName) return;
		claimingName = `${sheetType}::${choiceName}`;
		error = '';
		try {
			await choosePrologue(gameID, { sheet_type: sheetType, choice_name: choiceName });
			// Refresh assets so the retinue panel sees new card-assets and
			// transferred ones.
			const [, assetData] = await Promise.all([reload(), listAssets(gameID)]);
			assets = assetData.assets;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not claim that box.';
		} finally {
			claimingName = '';
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

	// ── Hearts declaration ───────────────────────────────────────────────────
	let heartCount = $state(0);
	let savingHearts = $state(false);
	async function submitHearts() {
		if (savingHearts) return;
		savingHearts = true;
		error = '';
		try {
			await declareHearts(gameID, heartCount);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not declare hearts.';
		} finally {
			savingHearts = false;
		}
	}

	let finalizing = $state(false);
	async function onFinalizeRanking() {
		if (finalizing) return;
		finalizing = true;
		error = '';
		try {
			await finalizeTrackRanking(gameID);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not finalize ranking.';
		} finally {
			finalizing = false;
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

	function moveSetAside(idx: number, dir: -1 | 1) {
		const tgt = idx + dir;
		if (tgt < 0 || tgt >= setAsideOrdering.length) return;
		const next = [...setAsideOrdering];
		[next[idx], next[tgt]] = [next[tgt], next[idx]];
		setAsideOrdering = next;
	}

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
	const unclaimedTitles = $derived.by(() => {
		const t = titlesSheet;
		if (!t) return [];
		return t.choices.filter(c => !claimMap.has(`titles::${c.name}`));
	});

	let extraPeerName = $state('');
	let creatingExtra = $state(false);
	async function submitExtraPeer() {
		if (!extraPeerName || creatingExtra) return;
		creatingExtra = true;
		error = '';
		try {
			const result = await createExtraPeer(gameID, extraPeerName);
			assets = [...assets, result.asset];
			extraPeerName = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not create extra peer.';
		} finally {
			creatingExtra = false;
		}
	}

	// ── Seats (unchanged from Phase 2 — still facilitator-driven) ────────────
	let draftSeats = $state<Record<number, string>>({});
	let savingSeats = $state(false);

	$effect(() => {
		const seats: Record<number, string> = {};
		for (const p of players) {
			seats[p.id] = p.seat_order != null ? String(p.seat_order) : '';
		}
		draftSeats = seats;
	});

	const seatsComplete = $derived(
		players.length > 0 && players.every(p => p.seat_order != null)
	);

	async function saveSeatsCombined() {
		const seats: Array<{ player_id: number; seat_order: number }> = [];
		for (const p of players) {
			const raw = draftSeats[p.id];
			const n = parseInt(raw, 10);
			if (!raw || isNaN(n) || n < 1) {
				error = `Enter a valid seat number for ${p.display_name}`;
				return;
			}
			seats.push({ player_id: p.id, seat_order: n });
		}
		savingSeats = true;
		error = '';
		try {
			await setSeats(gameID, seats);
			players = players.map(p => {
				const s = seats.find(s => s.player_id === p.id);
				return s ? { ...p, seat_order: s.seat_order } : p;
			});
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not save seats.';
		} finally {
			savingSeats = false;
		}
	}

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

	const myAssets = $derived(assets.filter(a => a.owner_id === currentPlayerID));

	async function onTearMarginalia(asset: Asset, m: Marginalium) {
		if (!confirm(`Tear "${m.text}"? This cannot be undone.`)) return;
		try {
			await tearMarginalia(asset.id, m.position);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not tear marginalia.';
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
								disabled={!!existingClaim || myTurns >= 3 || claimingName !== ''}
								title={choice.description || ''}
								onclick={() => claimChoice(sheet.type, choice.name)}
							>
								<span class="choice-name">{choice.name}</span>
								<span class="choice-cards">
									{choice.cards[0].value}{suitGlyph(choice.cards[0].suit)}
									·
									{choice.cards[1].value}{suitGlyph(choice.cards[1].suit)}
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
						<span class="card-chip">{c.card_value}{suitGlyph(c.card_suit)}</span>
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
		<p class="muted">
			Active track: <strong>{currentTrack}</strong>.
			Each player declares how many of their hearts to use as <strong>{currentTrack}</strong>'s suit.
			Each heart can only be used once across all three tracks.
		</p>

		<div class="declare-form">
			<label>
				Hearts to declare as {currentTrack}:
				<input type="number" min="0" bind:value={heartCount} />
			</label>
			<button class="secondary" onclick={submitHearts} disabled={savingHearts}>
				{savingHearts ? '…' : 'Submit'}
			</button>
		</div>

		<p class="muted small">
			Hearts you hold: {myCards.filter(c => c.card_suit === 'H').length}.
		</p>

		{#if isFacilitator}
			<button class="primary" onclick={onFinalizeRanking} disabled={finalizing}>
				{finalizing ? '…' : `Finalize ${currentTrack} ranking`}
			</button>
			<p class="muted small">
				Once everyone has declared their hearts (or skipped), finalize to compute the rank order.
			</p>
		{/if}

	{:else if mode === 'place'}
		<p class="muted">
			Active track: <strong>{currentTrack}</strong>. The rank-1 player places set-aside players
			(those with zero of this suit) into the remaining open ranks.
		</p>

		<div class="ranks-display">
			{#each trackRanksHere as r}
				<div class="rank-row">
					<span class="rank-num">{r.rank}.</span>
					<span>{playerName(r.player_id)}</span>
				</div>
			{/each}
		</div>

		{#if isMyTurnForSetAsides && setAsideOrdering.length > 0}
			<h3>Place set-aside players</h3>
			<p class="muted small">
				They will be slotted into the remaining open ranks in this order. Use the arrows to reorder.
			</p>
			<ul class="set-aside-list">
				{#each setAsideOrdering as pid, idx}
					<li>
						<button class="text-btn" disabled={idx === 0} onclick={() => moveSetAside(idx, -1)}>↑</button>
						<button class="text-btn" disabled={idx === setAsideOrdering.length - 1} onclick={() => moveSetAside(idx, 1)}>↓</button>
						{playerName(pid)}
					</li>
				{/each}
			</ul>
			<button class="primary" onclick={submitSetAsides} disabled={placing}>
				{placing ? '…' : 'Submit ordering'}
			</button>
		{:else if setAsideOrdering.length === 0}
			<p class="muted small">Waiting for the rank-1 player to place set-asides…</p>
		{:else}
			<p class="muted small">Waiting on {playerName(rankings.find(r => r.category === currentTrack && r.rank === 1)?.player_id ?? null)} to place the set-aside players.</p>
		{/if}

	{:else if mode === 'extra'}
		<p class="muted">
			Extra peers: with three or fewer players, each player picks one unused title to flesh out the cast.
		</p>

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
			<button class="secondary" onclick={submitExtraPeer} disabled={!extraPeerName || creatingExtra}>
				{creatingExtra ? '…' : 'Create peer'}
			</button>
		</div>

		{#if isFacilitator}
			<p class="muted small">When everyone has picked, you can start the main event.</p>
		{/if}
	{/if}

	<!-- Always-visible side panels: retinue + seats + start -->

	{#if myAssets.length > 0}
		<section class="retinue">
			<h3>Your Retinue</h3>
			<div class="asset-list">
				{#each myAssets as asset (asset.id)}
					<AssetCard {asset} onTear={onTearMarginalia} />
				{/each}
			</div>
		</section>
	{/if}

	{#if isFacilitator}
		<section class="facilitator-panel">
			<h3>Seat Order</h3>
			<p class="muted small">Clockwise seat numbers; sets focus-player ordering for the main event.</p>
			<div class="seat-grid">
				{#each players as p}
					<div class="seat-row">
						<span class="seat-name">{p.display_name}</span>
						<input
							type="number"
							min="1"
							max={players.length}
							class="seat-input"
							value={draftSeats[p.id] ?? ''}
							oninput={(e) => { draftSeats[p.id] = (e.target as HTMLInputElement).value; }}
						/>
					</div>
				{/each}
			</div>
			<button class="secondary" onclick={saveSeatsCombined} disabled={savingSeats}>
				{savingSeats ? '…' : 'Save seat order'}
			</button>

			<div class="start-section">
				<button
					class="primary"
					onclick={advanceToMainEvent}
					disabled={advancing || !seatsComplete || rankings.length < 15}
					title={
						!seatsComplete ? 'Seat order is not fully set' :
						rankings.length < 15 ? 'Rankings sub-flow is not complete' : undefined
					}
				>
					{advancing ? '…' : 'Start Main Event'}
				</button>
				{#if !seatsComplete}
					<p class="hint">Assign a seat to every player first.</p>
				{:else if rankings.length < 15}
					<p class="hint">Finish the ranking sub-flow first.</p>
				{/if}
			</div>
		</section>
	{/if}
</div>

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
	.hint { font-size: 0.8rem; color: #e0a060; margin: 0; }

	.choosing-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 1rem;
	}
	@media (max-width: 900px) {
		.choosing-grid { grid-template-columns: 1fr; }
	}

	.sheet-panel {
		background: #1e1e1e;
		border: 1px solid #333;
		border-radius: 8px;
		padding: 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.choice-list {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.choice-btn {
		text-align: left;
		background: #2a2a2a;
		color: #e8e4d9;
		border: 1px solid #444;
		border-radius: 4px;
		padding: 0.4rem 0.6rem;
		font-size: 0.85rem;
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		cursor: default;
	}

	.choice-btn.claimed { opacity: 0.5; }

	.choice-name { font-weight: 600; color: #c8a96e; }
	.choice-cards { font-size: 0.75rem; color: #aaa; }
	.claim-by { font-size: 0.7rem; color: #888; }

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

	.card-chip {
		background: #2a2a2a;
		border: 1px solid #555;
		border-radius: 4px;
		padding: 0.2rem 0.4rem;
		font-size: 0.85rem;
	}

	.declare-form, .extra-form {
		display: flex;
		gap: 0.6rem;
		align-items: end;
		margin: 0.5rem 0;
	}
	.declare-form label, .extra-form label {
		display: flex;
		flex-direction: column;
		font-size: 0.85rem;
		color: #aaa;
	}
	.declare-form input, .extra-form select {
		background: #333;
		color: #e8e4d9;
		border: 1px solid #555;
		border-radius: 4px;
		padding: 0.3rem 0.5rem;
		font-size: 0.9rem;
	}

	.ranks-display {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		background: #1e1e1e;
		border: 1px solid #333;
		border-radius: 8px;
		padding: 0.6rem;
		max-width: 24rem;
	}
	.rank-row { display: flex; gap: 0.5rem; font-size: 0.9rem; }
	.rank-num { color: #888; min-width: 1.5rem; }

	.set-aside-list {
		list-style: none;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		max-width: 18rem;
	}
	.set-aside-list li {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.9rem;
	}

	.text-btn {
		background: none;
		color: #c8a96e;
		padding: 0.1rem 0.3rem;
		font-size: 0.85rem;
		cursor: pointer;
	}
	.text-btn:disabled { opacity: 0.3; cursor: not-allowed; }

	.retinue { margin-top: 1rem; }
	.asset-list { display: flex; flex-direction: column; gap: 0.6rem; }

	.facilitator-panel {
		background: #1e1e1e;
		border: 1px solid #333;
		border-radius: 8px;
		padding: 1rem;
	}

	.seat-grid {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		margin-bottom: 0.75rem;
	}
	.seat-row { display: flex; align-items: center; gap: 0.6rem; }
	.seat-name { flex: 1; font-size: 0.9rem; }
	.seat-input {
		width: 3.5rem;
		background: #2a2a2a;
		color: #e8e4d9;
		border: 1px solid #555;
		border-radius: 4px;
		padding: 0.25rem 0.4rem;
		text-align: center;
	}

	.start-section {
		margin-top: 1rem;
		padding-top: 0.75rem;
		border-top: 1px solid #333;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
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
