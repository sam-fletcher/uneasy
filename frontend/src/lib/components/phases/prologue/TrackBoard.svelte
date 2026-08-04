<!-- TrackBoard.svelte
  Three-column board for the prologue ranking stage. All three tracks
  render side-by-side so changes to commitments are visible across
  every track at once. The current declare step is highlighted but
  the player views all three at the same time.

  See PROLOGUE_RANKING_UI_PLAN.md.
-->
<script lang="ts">
	import WeightMeter from '$lib/components/shared/WeightMeter.svelte';
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
	import { cardWeight, MAX_CARD_WEIGHT } from '$lib/prologue/choosing';

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
		// so the per-player card marks and the done-committing dot (uniformly
		// true post-resolution) are omitted. Only valid once all three tracks
		// are resolved — the set-aside badge lives in the card row and
		// unresolved projections would lose it.
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

	// `cardSuit` is a data key now, not a picture: suits are retired from the
	// game UI (adr/PROLOGUE_UX_ROUND2_PLAN.md, decision 1) and this column used
	// to draw one as its header pip. The letter survives only because it is how
	// the API spells the card rows this column filters — nothing renders it.
	const TRACKS: { id: PrologueTrack; label: string; cardSuit: 'C' | 'D' | 'S' }[] = [
		{ id: 'power', label: 'Power', cardSuit: 'C' },
		{ id: 'knowledge', label: 'Knowledge', cardSuit: 'D' },
		{ id: 'esteem', label: 'Esteem', cardSuit: 'S' }
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

	// Heaviest first, which is now literally what the reader sees: the marks
	// descend, and that descending walk IS the tie-break (rankFromContributions
	// compares the two players' lists position by position).
	function trackCardsForPlayer(pid: number, suit: string): PlayerCardRow[] {
		return cards
			.filter((c) => c.player_id === pid && c.card_suit === suit)
			.sort((a, b) => cardRank(b.card_value) - cardRank(a.card_value));
	}

	/** Wilds this player has spent on this track. (The API still calls them
	 *  hearts — the deck is the server's storage format; the UI stopped
	 *  showing it.) */
	function committedWildsForPlayer(pid: number, track: PrologueTrack): CommittedHeart[] {
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
				<!-- The track name and nothing else. The suit pip that used to sit
				     beside it taught the wrong half of the fact: ♠ heads the Esteem
				     column but makes an artifact, so the pip was an alias that
				     contradicted the word next to it (decision 1). -->
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
								<!-- One mark per card on this track (Session 3a). The mark is
								     the countable unit — how many marks a player has IS their
								     position on this track — and the meter inside it is the
								     weight that breaks a tie between two equal counts. A suited
								     card value said neither: the suit restated the column
								     header and the letter needed a rank table to mean
								     anything. -->
								<div class="chip-cards">
									{#each trackCardsForPlayer(pid, t.cardSuit) as c}
										<span class="mark">
											<WeightMeter value={c.card_value} compact />
										</span>
									{/each}
									<!-- This mark carries no visible text — a dashed frame around a
									     meter — so the aria-label is its only label, and it says
									     "any-track card" in plain language rather than the three
									     letters (Round 3, decision 6). A listener hearing "A-N-Y"
									     with no chip in front of them would be worse off than a
									     reader, which is the opposite of what the label is for. -->
									{#each committedWildsForPlayer(pid, t.id) as h}
										{@const doingWork = bright.get(pid)?.has(h.card_id) ?? false}
										<span
											class="mark wild"
											class:inert={!doingWork}
											role="img"
											aria-label={`any-track card, weight ${cardWeight(h.value)} of ${MAX_CARD_WEIGHT}, ${
												doingWork ? 'doing work' : 'wasted — would be refunded'
											}`}
											title={doingWork ? 'doing work' : 'wasted (would be refunded)'}
										>
											<WeightMeter value={h.value} compact describe={false} />
										</span>
									{/each}
									{#if row.isSetAside && committedWildsForPlayer(pid, t.id).length === 0}
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
	/*
	 * Two lines, not one. The board is three columns wide at every viewport
	 * (deliberately — the tracks have to be comparable at a glance), and the
	 * phase column caps at 440, so a name gets ~78px on a phone and ~96px on
	 * a desktop: 11-14 characters, permanently. On one line every name at
	 * the 20-char cap clipped, and since all three columns hold the SAME
	 * five players they clipped to the same strings — the board stopped
	 * naming anyone.
	 *
	 * Two lines buys ~22 characters, which covers the cap. overflow-wrap is
	 * doing the real work: player names are typically one unbroken word, so
	 * `white-space: normal` alone would never find a break point and the
	 * name would just overflow. Line-clamp keeps a pathological name from
	 * growing the row without bound (same idiom as AssetCardSelectable's
	 * expanded card).
	 */
	.chip-name {
		font-size: 0.75rem;
		line-height: 1.25;
		color: var(--color-text);
		font-weight: 500;
		overflow: hidden;
		overflow-wrap: anywhere;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
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

	/* One card, one mark. Bordered rather than a bare meter because the first
	   thing this column has to answer is "how many?" — that count is the
	   ranking — and four unframed meters in a row read as one long bar. The
	   frame makes them countable; the fill inside makes them comparable. */
	.mark {
		display: inline-flex;
		align-items: center;
		padding: 0.1rem 0.15rem;
		border: 1px solid var(--color-border);
		border-radius: 3px;
		background: var(--color-bg);
		flex: none;
	}
	/* Not a track card but a wild spent on this track, so it takes the app's
	   dashed not-yet-a-track treatment (decision 3) instead of the ♥ it used
	   to draw — the same idiom as .track-code.wild and the choosing view's
	   .tile-chip.wild, one level out on the frame rather than nested inside. */
	.mark.wild {
		border-style: dashed;
		border-color: var(--color-border-strong);
	}
	/* This wild does no work: it is committed here but the track would rank
	   the same without it, so resolution refunds it. Struck as well as dimmed —
	   dim alone reads as "quiet", and this needs to read as "cancelled". */
	.mark.inert {
		position: relative;
		opacity: 0.45;
	}
	.mark.inert::after {
		content: '';
		position: absolute;
		left: 0.1rem;
		right: 0.1rem;
		top: 50%;
		border-top: 1px solid var(--color-text);
		pointer-events: none;
	}

	/* No wide variant: the phase column is a phone-width column at every
	   viewport (≤440; docs/STYLE_GUIDE.md "Layout widths"), so the base
	   metrics are the only metrics. */
</style>
