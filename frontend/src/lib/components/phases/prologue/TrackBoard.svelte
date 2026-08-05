<!-- TrackBoard.svelte
  Three-column board for the prologue ranking stage. All three tracks
  render side-by-side so changes to commitments are visible across
  every track at once. The current declare step is highlighted but
  the player views all three at the same time.

  What the board is FOR: the relative position of every player across the
  three tracks, with the viewer's own position emphasised. Every ruling
  below falls out of that sentence — the shared rank gutter exists because
  cross-track comparison is the whole point, and the per-card weight meters
  stay because this is the only screen where card weight matters.

  See PROLOGUE_RANKING_UI_PLAN.md.
-->
<script lang="ts">
	import WeightMeter from '$lib/components/shared/WeightMeter.svelte';
	import '$lib/components/shared/trackCode.css';
	import { playerColorByID } from '$lib/playerColor';
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
	import { cardWeight, MAX_CARD_WEIGHT, trackCode } from '$lib/prologue/choosing';

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

	/** The literal rank slots, 1–5 at every player count. Dummy tokens hold
	 *  some of them (reference_dummy_ranks), which is exactly why the gutter
	 *  keeps drawing a numeral for a row nobody occupies. */
	const RANKS = [1, 2, 3, 4, 5];

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

	type RankRow = { rank: number; playerID: number | null; isDummy: boolean; isSetAside: boolean };

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

	function rankRowsFor(p: Projection): RankRow[] {
		const dummies = new Set(p.dummyRanks);
		const byRank = new Map<number, number>();
		for (const [pid, slot] of p.slots) byRank.set(slot, pid);
		const rows: RankRow[] = [];
		for (const r of RANKS) {
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

	// Everything one column needs, computed once. The gutter has to agree with
	// all three (see dummyBand), so the projections can't stay inline in the
	// markup the way they did when each column was its own island.
	const columns = $derived(
		TRACKS.map((t) => ({
			t,
			rows: rankRowsFor(projectTrack(t.id)),
			bright: brightForTrack(t.id),
			doneSet: doneSetForTrack(t.id)
		}))
	);

	/** Ranks the dummy token holds in EVERY column, and so a band across the
	 *  whole board rather than a per-column row. `dummyRanksForCount` doesn't
	 *  vary by track, so in practice this is all of them — the intersection is
	 *  what keeps the gutter honest if a persisted ranking ever disagrees. */
	const dummyBand = $derived(
		new Set(RANKS.filter((r) => columns.every((c) => c.rows.some((row) => row.rank === r && row.isDummy))))
	);

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

	/**
	 * Wilds nobody has assigned to a track yet, one entry per player.
	 *
	 * An unspent wild can't be drawn as a mark: it will land on exactly one
	 * track and which one isn't known yet, so a ghost in all three columns
	 * would overstate it threefold. It is one fact per player, not one per
	 * player per track — hence a line under the board rather than a mark in
	 * it. Without it the board rendered nine marks and zero wilds while the
	 * viewer was holding an ANY card, i.e. a projection that read as settled.
	 */
	const unspentWilds = $derived.by(() => {
		const spent = new Map<number, number>();
		for (const h of committed) spent.set(h.player_id, (spent.get(h.player_id) ?? 0) + 1);
		const out: { id: number; label: string; n: number }[] = [];
		for (const p of players) {
			const held = cards.filter((c) => c.player_id === p.id && c.card_suit === 'H').length;
			const n = held - (spent.get(p.id) ?? 0);
			if (n > 0) out.push({ id: p.id, label: p.id === currentPlayerID ? 'you' : p.display_name, n });
		}
		// The viewer first: this line's first job is "you still have one to place".
		return out.sort((a, b) => Number(b.id === currentPlayerID) - Number(a.id === currentPlayerID));
	});

	// Only before any track is live. During declare, HandStrip already shows
	// the viewer's own ANY cards right under the board, and the marks in the
	// active column show where everyone else's went.
	const showUnspent = $derived(showCards && activeTrack === null && unspentWilds.length > 0);
</script>

<div class="track-board">
	<div class="board-grid">
		<!-- The y-axis. One numeral per rank for the whole board, not one per
		     rank per track: rank 3 is the same slot in all three columns, and
		     three copies of it only ever agreed by coincidence. -->
		<div class="gutter" aria-hidden="true">
			<span class="gutter-head"></span>
			{#each RANKS as r (r)}
				<span class="rank-num" class:dummy={dummyBand.has(r)}>{r}</span>
			{/each}
		</div>

		{#each columns as c (c.t.id)}
			<section class="column" class:active={activeTrack === c.t.id}>
				<header class="col-head">
					<!-- The track name and nothing else. The suit pip that used to sit
					     beside it taught the wrong half of the fact: ♠ heads the Esteem
					     column but makes an artifact, so the pip was an alias that
					     contradicted the word next to it (decision 1). -->
					<span class="col-label">{c.t.label}</span>
				</header>
				{#each c.rows as row (row.rank)}
					<div class="rank-row" class:dummy={row.isDummy}>
						<!-- The visible numeral lives in the gutter, which is one group
						     up front for a screen reader — so each row still carries its
						     own rank where the reading order needs it. -->
						<span class="sr-rank">rank {row.rank}{row.isDummy ? ', unclaimed' : ''}</span>
						{#if row.playerID != null}
							{@const pid = row.playerID}
							{@const isYou = pid === currentPlayerID}
							<div class="chip">
								<div class="chip-head">
									{#if isYou}
										<!-- The viewer's row needs emphasis (it is half the board's
										     job) but not gold: on this board gold means the live
										     track and nothing else, and the header strip already
										     ruled that identity isn't gold's job. This is the same
										     player-colour dot the header pills and chat bylines
										     use, and at 7px it costs a name cell ~11px where a
										     "you" label would cost ~30 and squeeze the two-line
										     clamp below. -->
										<span
											class="you-dot"
											style="background: {playerColorByID(pid, players)}"
											role="img"
											aria-label="you"
										></span>
									{/if}
									<span class="chip-name">{playerName(pid)}</span>
									{#if showCards && c.doneSet.has(pid)}
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
										{#each trackCardsForPlayer(pid, c.t.cardSuit) as card}
											<span class="mark">
												<WeightMeter value={card.card_value} compact />
											</span>
										{/each}
										<!-- This mark carries no visible text — a dashed frame around a
										     meter — so the aria-label is its only label, and it says
										     "any-track card" in plain language rather than the three
										     letters (Round 3, decision 6). A listener hearing "A-N-Y"
										     with no chip in front of them would be worse off than a
										     reader, which is the opposite of what the label is for. -->
										{#each committedWildsForPlayer(pid, c.t.id) as h}
											{@const doingWork = c.bright.get(pid)?.has(h.card_id) ?? false}
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
										<!-- No second condition: `setAside` means zero cards on this
										     track *including* committed wilds (refund.ts), so a
										     set-aside row can never also be showing a wild mark. -->
										{#if row.isSetAside}
											<span class="set-aside-badge" title="Zero cards on this track">none</span>
										{/if}
									</div>
								{/if}
							</div>
						{:else if !row.isDummy}
							<span class="empty-slot">—</span>
						{/if}
					</div>
				{/each}
			</section>
		{/each}
	</div>

	{#if showUnspent}
		<!-- "at the table" earns its words: StandingStrip's own note a few lines
		     up says how many the VIEWER holds, and without the scope the two
		     read as the same sentence twice at a 4-player table where only the
		     viewer holds one. -->
		<p class="unspent">
			<span class="track-code wild" aria-hidden="true">{trackCode('H')}</span> to be allocated at the end
			— {#each unspentWilds as u, i (u.id)}{i > 0 ? ' · ' : ''}{u.label} {u.n}{/each}.
		</p>
	{/if}
</div>

<style>
	.track-board {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	/*
	 * One grid for the whole board, not three independent columns.
	 *
	 * Rank N has to sit at the same eye level in all three tracks — comparing
	 * positions across tracks IS what the board is for — and three flex
	 * columns with content-driven row heights only lined up by coincidence:
	 * one realistic long name plus five marks in a single column drifted the
	 * rows 25px, at which point the reader is comparing rank 2 in Power
	 * against rank 3 in Knowledge. The rows are the parent's tracks now and
	 * each column subgrids onto them, so alignment is structural rather than
	 * lucky. (Subgrid also buys the active column one continuous ground: it
	 * is a single grid item spanning every row, so its tint runs behind the
	 * gaps too.)
	 *
	 * Nothing here may take padding or a border — a subgrid item's own
	 * padding shifts the tracks it inherited, which would undo the alignment
	 * this exists for. Spacing lives on the rows.
	 */
	.board-grid {
		display: grid;
		grid-template-columns: auto repeat(3, minmax(0, 1fr));
		grid-template-rows: repeat(6, auto);
		column-gap: 0.4rem;
		row-gap: 0.2rem;
	}
	.gutter,
	.column {
		display: grid;
		grid-row: 1 / -1;
		grid-template-rows: subgrid;
		min-width: 0;
	}
	.board-grid > :nth-child(1) { grid-column: 1; }
	.board-grid > :nth-child(2) { grid-column: 2; }
	.board-grid > :nth-child(3) { grid-column: 3; }
	.board-grid > :nth-child(4) { grid-column: 4; }

	/* Gold does exactly one job on this board: the live track. The full-height
	   gold outline this used to wear put a second gold rectangle around the
	   viewer's own gold-outlined name — the concentric-frame shape Round 2
	   already ruled against. The step label above already names the track
	   ("Rankings: Spend ANY on Power"), so a ground tint and a gold header is
	   the whole signal it needs. */
	.column.active {
		background: color-mix(in srgb, var(--color-accent) 7%, transparent);
		border-radius: 4px;
	}
	.column.active .rank-row {
		background: color-mix(in srgb, var(--color-accent) 12%, var(--color-surface));
	}

	.col-head {
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 0.1rem 0.05rem 0.3rem;
		border-bottom: 1px solid var(--color-surface-2);
	}
	/* Matches PrologueView's .detail-track — the same word rendered six
	   different ways across this phase was one way too many, and the board's
	   gold was the outlier (the only gold instance of a track name). Resting
	   state is now the app's ordinary track-name treatment; the ACTIVE state
	   is what changes. */
	.col-label {
		color: var(--color-text-muted);
		font-size: 0.62rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		white-space: nowrap;
	}
	.column.active .col-label {
		color: var(--color-accent);
	}

	/* A chart's y-axis tick, not a chip: muted, small, top-aligned onto the
	   name's first line, and no decoration. No ordinal suffix — rank slots are
	   literal 1–5 with dummy tokens in some of them, so "2nd" is a lie in a
	   2–3 player game (Round 2, decision 7; reference_dummy_ranks). */
	.rank-num {
		color: var(--color-text-faint);
		font-size: 0.65rem;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		line-height: 1;
		text-align: right;
		padding-top: 0.35rem;
	}
	.rank-num.dummy {
		color: var(--color-neutral);
	}

	.rank-row {
		display: flex;
		align-items: flex-start;
		padding: 0.2rem 0.3rem;
		background: var(--color-surface);
		border-radius: 3px;
		min-height: 32px;
		min-width: 0;
		position: relative;
	}
	/* Dummy rows stay — they are why the numbering has gaps, and hiding them
	   would leave a 2-player board reading "2, 4" with no account of the
	   missing slots or of the fact that rank 2 IS the top. But the diagonal
	   hatch they used to wear was the loudest texture on the board, spent on
	   its least important row. An empty recessed band (the same
	   surface-sunken as StandingStrip's .mslot.dummy) plus a dimmed numeral
	   says it quietly. */
	.rank-row.dummy,
	.column.active .rank-row.dummy {
		background: var(--color-surface-sunken);
	}

	/* The visible rank numerals sit in one gutter up front, so this is what
	   keeps each row self-describing in the reading order. */
	.sr-rank {
		position: absolute;
		width: 1px;
		height: 1px;
		margin: -1px;
		padding: 0;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
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
	/* The badge is the only encoding of set-aside now. The violet row tint and
	   the violet left border it used to sit inside were exactly co-extensive
	   with it — `setAside` means zero cards including committed wilds, so all
	   three appeared and disappeared together — and the badge is the one that
	   says WHAT rather than merely "something". Violet, not the warning
	   orange: being set aside is procedural, not a hazard, and often not even
	   actionable (you may simply hold no cards of that suit). */

	.empty-slot { color: var(--color-text-faint); font-style: italic; font-size: 0.75rem; }

	.chip {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		min-width: 0;
	}
	.chip-head {
		display: flex;
		align-items: flex-start;
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
	/* Both dots ride the name's FIRST line rather than centring against a
	   name that may wrap to two. */
	.you-dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		margin-top: 0.3rem;
		flex: none;
	}
	.done-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		margin-top: 0.32rem;
		background: var(--color-success);
		flex: none;
	}
	/* 6px between marks against 2px between the segments inside one: the marks
	   group by proximity, which is what lets the frames come off below. */
	.chip-cards {
		display: flex;
		flex-wrap: wrap;
		gap: 4px 6px;
		min-height: 1.05rem; /* matches a card row so empty/no-cards chips don't shrink */
		align-items: center;
	}

	/* One card, one mark — bare. These used to carry a 1px frame each because
	   four unframed meters in a row really did read as one long bar, but the
	   cause was spacing, not the absence of a border: the gap BETWEEN marks
	   was 2.4px and the gap between segments INSIDE one was 2px, so proximity
	   was doing nothing to bind four bars into a card. With the inter-mark gap
	   at 3x the segment gap the grouping is spatial, five frames per column
	   come off, and the box-inside-a-box reading goes with them. */
	.mark {
		display: inline-flex;
		align-items: center;
		flex: none;
	}
	/* Not a track card but a wild spent on this track, so it keeps the app's
	   dashed not-yet-a-track treatment (decision 3) — and now that the plain
	   marks are unframed, a dashed frame marks the exception rather than
	   every mark. Same idiom as .track-code.wild and the choosing view's
	   .tile-chip.wild. */
	.mark.wild {
		padding: 0.1rem 0.15rem;
		border: 1px dashed var(--color-border-strong);
		border-radius: 3px;
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
		left: 1px;
		right: 1px;
		top: 50%;
		border-top: 1px solid var(--color-text);
		pointer-events: none;
	}

	/* One line for a fact that is one per player, not one per player per
	   track — see unspentWilds. It doubles as the label that says the board
	   above is a projection. */
	/* A paragraph, not a flex row: the chip has to flow WITH the sentence.
	   As a flex item it took a line of its own and pushed the whole clause
	   down, which is the layout the STYLE_GUIDE's "a code in a chip" note is
	   trying to avoid — the chip is the sentence's first word. */
	.unspent {
		margin: 0;
		color: var(--color-text-faint);
		font-size: 0.68rem;
		line-height: 1.5;
	}
	.unspent .track-code {
		margin-right: 0.1rem;
	}

	/* No wide variant: the phase column is a phone-width column at every
	   viewport (≤440; docs/STYLE_GUIDE.md "Layout widths"), so the base
	   metrics are the only metrics. */
</style>
