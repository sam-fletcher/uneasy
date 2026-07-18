<!-- TrackBoard.svelte
  Three-column board for the prologue ranking stage. All three tracks
  render side-by-side so changes to commitments are visible across
  every track at once. The current declare step is highlighted but
  the player views all three at the same time.

  See PROLOGUE_RANKING_UI_PLAN.md.
-->
<script lang="ts">
	import '$lib/components/shared/cardGlyph.css';
	import type {
		Player,
		PlayerCardRow,
		Ranking,
		CommittedHeart,
		TrackDone,
		PrologueTrack,
		RankingCategory
	} from '$lib/api';
	import {
		computeTrackRanking,
		computeBrightHearts,
		computeFinalSlots,
		cardRank
	} from '$lib/prologue/refund';

	interface Props {
		players: Player[];
		cards: PlayerCardRow[];
		rankings: Ranking[];
		committed: CommittedHeart[];
		doneFlags: TrackDone[];
		// null during box-selection: no track is being resolved yet, so no
		// column is highlighted. Set to the live track during declare/place.
		activeTrack: PrologueTrack | null;
		currentPlayerID: number | null;
		// Recap use (closing stage): rankings are persisted and cards are spent,
		// so suit glyphs, per-player card rows, and the done-committing dot
		// (uniformly true post-resolution) are omitted. Only valid once all
		// three tracks are resolved — the set-aside badge lives in the card
		// row and unresolved projections would lose it.
		showCards?: boolean;
	}

	let {
		players,
		cards,
		rankings,
		committed,
		doneFlags,
		activeTrack,
		currentPlayerID,
		showCards = true
	}: Props = $props();

	const TRACKS: { id: PrologueTrack; label: string; suit: string; suitChar: 'C' | 'D' | 'S' }[] = [
		{ id: 'power', label: 'Power', suit: '♣', suitChar: 'C' },
		{ id: 'knowledge', label: 'Knowledge', suit: '♦', suitChar: 'D' },
		{ id: 'esteem', label: 'Esteem', suit: '♠', suitChar: 'S' }
	];

	function trackToCategory(t: PrologueTrack): RankingCategory {
		return t as RankingCategory;
	}

	const allPlayerIDs = $derived(players.map((p) => p.id));

	// Per-track projection: which players land in which rank slots.
	// Set-asides (zero count) are folded inline at their default slot
	// positions (player_id ascending) — only the rank-1 player can
	// reorder them, and only in the dedicated place_set_asides step.
	type Projection = {
		slots: Map<number, number>; // player_id → slot (1..5)
		dummyRanks: number[];
		setAsideIDs: Set<number>; // players whose count on this track is zero
		resolved: boolean;
	};

	function projectTrack(track: PrologueTrack): Projection {
		const cat = trackToCategory(track);
		const persisted = rankings.filter((r) => r.category === cat);
		if (persisted.length > 0) {
			const sorted = [...persisted].sort((a, b) => a.rank - b.rank);
			const slots = new Map<number, number>();
			const dummyRanks: number[] = [];
			for (const r of sorted) {
				if (r.player_id == null) dummyRanks.push(r.rank);
				else slots.set(r.player_id, r.rank);
			}
			return { slots, dummyRanks, setAsideIDs: new Set(), resolved: true };
		}
		const slots = computeFinalSlots(track, allPlayerIDs, cards, committed);
		const r = computeTrackRanking(track, allPlayerIDs, cards, committed);
		return {
			slots,
			dummyRanks: dummyRanksForCount(players.length),
			setAsideIDs: new Set(r.setAside),
			resolved: false
		};
	}

	function dummyRanksForCount(n: number): number[] {
		switch (n) {
			case 4: return [3];
			case 3: return [1, 5];
			case 2: return [1, 3, 5];
			default: return [];
		}
	}

	function rankRowsFor(p: Projection): { rank: number; playerID: number | null; isDummy: boolean; isSetAside: boolean }[] {
		const dummies = new Set(p.dummyRanks);
		const byRank = new Map<number, number>();
		for (const [pid, slot] of p.slots) byRank.set(slot, pid);
		const rows: { rank: number; playerID: number | null; isDummy: boolean; isSetAside: boolean }[] = [];
		for (let r = 1; r <= 5; r++) {
			if (dummies.has(r)) {
				rows.push({ rank: r, playerID: null, isDummy: true, isSetAside: false });
				continue;
			}
			const pid = byRank.get(r) ?? null;
			rows.push({
				rank: r,
				playerID: pid,
				isDummy: false,
				isSetAside: pid != null && p.setAsideIDs.has(pid)
			});
		}
		return rows;
	}

	function brightForTrack(track: PrologueTrack): Map<number, Set<number>> {
		return computeBrightHearts(track, allPlayerIDs, cards, committed);
	}

	function doneSetForTrack(track: PrologueTrack): Set<number> {
		const s = new Set<number>();
		for (const d of doneFlags) {
			if (d.track === track && d.done) s.add(d.player_id);
		}
		return s;
	}

	function playerName(id: number | null): string {
		if (id == null) return '';
		return players.find((p) => p.id === id)?.display_name ?? '?';
	}

	function suitCardsForPlayer(pid: number, suit: string): PlayerCardRow[] {
		return cards
			.filter((c) => c.player_id === pid && c.card_suit === suit)
			.sort((a, b) => cardRank(b.card_value) - cardRank(a.card_value));
	}

	function committedHeartsForPlayer(pid: number, track: PrologueTrack): CommittedHeart[] {
		return committed
			.filter((h) => h.player_id === pid && h.track === track)
			.sort((a, b) => cardRank(b.value) - cardRank(a.value));
	}
</script>

<div class="track-board">
	{#each TRACKS as t}
		{@const proj = projectTrack(t.id)}
		{@const bright = brightForTrack(t.id)}
		{@const doneSet = doneSetForTrack(t.id)}
		<section class="column" class:active={activeTrack === t.id}>
			<header class="col-head">
				{#if showCards}
					<span class="col-suit" data-color={t.id === 'knowledge' ? 'red' : 'black'}>{t.suit}</span>
				{/if}
				<span class="col-label">{t.label}</span>
			</header>
			{#each rankRowsFor(proj) as row}
				<div
					class="rank-row"
					class:dummy={row.isDummy}
					class:set-aside={row.isSetAside}
				>
					<span class="rank-num">{row.rank}</span>
					{#if row.isDummy}
						<span class="dummy-slot"></span>
					{:else if row.playerID != null}
						{@const pid = row.playerID}
						{@const isYou = pid === currentPlayerID}
						<div class="chip" class:you={isYou}>
							<div class="chip-head">
								<span class="chip-name">{playerName(pid)}</span>
								{#if showCards && doneSet.has(pid)}
									<span class="done-dot" title="Done"></span>
								{/if}
							</div>
							{#if showCards}
								<div class="chip-cards">
									{#each suitCardsForPlayer(pid, t.suitChar) as c}
										<span class="card-glyph small" data-color={t.suitChar === 'D' ? 'red' : 'black'}>
											{c.card_value}
										</span>
									{/each}
									{#each committedHeartsForPlayer(pid, t.id) as h}
										<span
											class="card-glyph small heart"
											class:grey={!(bright.get(pid)?.has(h.card_id) ?? false)}
											data-color="red"
											title={(bright.get(pid)?.has(h.card_id) ?? false)
												? 'doing work'
												: 'wasted (would be refunded)'}
										>
											{h.value}♥
										</span>
									{/each}
									{#if row.isSetAside && committedHeartsForPlayer(pid, t.id).length === 0}
										<span class="set-aside-badge" title="Zero cards on this track">no cards</span>
									{/if}
								</div>
							{/if}
						</div>
					{:else}
						<span class="empty-slot">—</span>
					{/if}
				</div>
			{/each}
		</section>
	{/each}
</div>

<style>
	.track-board {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.4rem;
	}

	.column {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		background: var(--color-bg);
		border: 1px solid var(--color-surface-2);
		border-radius: 6px;
		padding: 0.3rem;
		min-width: 0;
	}
	.column.active {
		border-color: var(--color-accent);
		box-shadow: 0 0 0 1px color-mix(in srgb, var(--color-accent) 25%, transparent) inset;
	}

	.col-head {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.2rem;
		padding: 0.1rem 0.05rem 0.3rem;
		border-bottom: 1px solid var(--color-surface-2);
	}
	/* Fixed line-height so the upsized diamond doesn't make the Knowledge
	   header taller than the other two (the label's line box sets the
	   header height instead). */
	.col-suit { font-size: 0.95rem; line-height: 1rem; }
	/* The diamond glyph's ink is ~34% narrower than the clubs' at equal font
	   size, and it's the only red-on-dark suit; upsizing evens out its
	   apparent size against the white suits. */
	.col-suit[data-color='red'] { color: var(--color-suit-red); font-size: 1.1rem; }
	.col-suit[data-color='black'] { color: var(--color-text); }
	.col-label {
		color: var(--color-accent);
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0;
		white-space: nowrap;
	}
	.rank-row {
		display: flex;
		align-items: flex-start;
		gap: 0.25rem;
		padding: 0.2rem 0.25rem;
		background: var(--color-surface);
		border-radius: 3px;
		min-height: 32px;
	}
	.rank-row.dummy {
		background: var(--color-bg);
		opacity: 0.5;
	}
	.rank-row.set-aside {
		background: color-mix(in srgb, var(--color-chip-violet-border) 12%, var(--color-surface));
		border-left: 2px solid var(--color-chip-violet-border);
	}
	.set-aside-badge {
		background: var(--color-chip-violet-bg);
		border: 1px solid var(--color-chip-violet-border);
		color: var(--color-chip-violet-text);
		font-size: 0.55rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 0.05rem 0.3rem;
		border-radius: 2px;
		flex: none;
	}
	.rank-num {
		color: var(--color-text-muted);
		font-size: 0.7rem;
		font-weight: 600;
		min-width: 0.9rem;
		padding-top: 0.1rem;
	}
	.dummy-slot {
		flex: 1;
		height: 1.2rem;
		background: repeating-linear-gradient(
			45deg,
			var(--color-surface),
			var(--color-surface) 4px,
			var(--color-bg) 4px,
			var(--color-bg) 8px
		);
		border-radius: 3px;
	}
	.empty-slot { color: var(--color-text-faint); font-style: italic; font-size: 0.75rem; }

	.chip {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		min-width: 0;
	}
	.chip.you {
		outline: 1px solid var(--color-accent);
		outline-offset: 1px;
		border-radius: 3px;
		background: color-mix(in srgb, var(--color-accent) 6%, transparent);
	}
	.chip-head {
		display: flex;
		align-items: center;
		gap: 0.25rem;
	}
	.chip-name {
		font-size: 0.75rem;
		color: var(--color-text);
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex: 1;
		min-width: 0;
	}
	.done-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--color-success);
		flex: none;
	}
	.chip-cards {
		display: flex;
		flex-wrap: wrap;
		gap: 0.15rem;
		min-height: 1.05rem; /* matches a card row so empty/no-cards chips don't shrink */
		align-items: center;
	}
	.card-glyph.grey {
		opacity: 0.45;
		background: var(--color-card-spent);
		text-decoration: line-through;
	}

	/* No wide variant: the phase column is a phone-width column at every
	   viewport (≤440; docs/STYLE_GUIDE.md "Layout widths"), so the base
	   metrics are the only metrics. */
</style>
