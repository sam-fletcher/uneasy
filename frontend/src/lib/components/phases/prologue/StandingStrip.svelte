<!-- StandingStrip.svelte
  Where the viewer currently stands on each of the three tracks, in ~87px,
  with the full TrackBoard one tap away.

  The tiles and the ranking used to be strangers: a tile said `K♣ K♦`, the
  suit → track mapping sat in a legend ~500px up the page, and the board that
  showed the consequence was ~600px down (adr/PROLOGUE_UX_ROUND2_PLAN.md,
  Motivation). The projection was already live — this just puts a readable
  slice of it where the choice is being made.

  No ordinals (decision 7). Rank slots are literal 1–5 with dummy tokens
  occupying some of them, so "2nd" is a lie in a 2–3 player game where rank 2
  IS the top (reference_dummy_ranks). A position on the real track can't be.
-->
<script lang="ts">
	import '$lib/components/shared/trackCode.css';
	import TrackBoard from './TrackBoard.svelte';
	import type { Player, PlayerCardRow, Ranking, CommittedHeart, TrackDone, PrologueTrack } from '$lib/api';
	import { computeFinalSlots, openRanksForCount } from '$lib/prologue/refund';
	import { trackCode } from '$lib/prologue/choosing';

	interface Props {
		players: Player[];
		cards: PlayerCardRow[];
		rankings: Ranking[];
		committed: CommittedHeart[];
		doneFlags: TrackDone[];
		currentPlayerID: number | null;
	}

	let { players, cards, rankings, committed, doneFlags, currentPlayerID }: Props = $props();

	const TRACKS: { id: PrologueTrack; label: string }[] = [
		{ id: 'power', label: 'Power' },
		{ id: 'knowledge', label: 'Knowledge' },
		{ id: 'esteem', label: 'Esteem' },
	];

	let boardOpen = $state(false);

	const allPlayerIDs = $derived(players.map((p) => p.id));

	/** Which of the five slots hold dummy tokens, derived from the open ranks
	 *  rather than restating the per-count table — one switch, in refund.ts,
	 *  next to the projection that has to agree with it. */
	const dummyRanks = $derived.by(() => {
		const open = new Set(openRanksForCount(players.length));
		const out = new Set<number>();
		for (let r = 1; r <= 5; r++) if (!open.has(r)) out.add(r);
		return out;
	});

	type SlotKind = 'you' | 'other' | 'dummy' | 'empty';

	/**
	 * The literal 1–5 track for one category. Projected from the live
	 * commitment state (`computeFinalSlots`), which is what the TrackBoard
	 * below draws too, so the strip and the board can never disagree.
	 */
	function slotsFor(track: PrologueTrack): SlotKind[] {
		const projected = computeFinalSlots(track, allPlayerIDs, cards, committed);
		const occupied = new Set<number>();
		let mine: number | null = null;
		for (const [pid, slot] of projected) {
			occupied.add(slot);
			if (pid === currentPlayerID) mine = slot;
		}
		const out: SlotKind[] = [];
		for (let r = 1; r <= 5; r++) {
			if (dummyRanks.has(r)) out.push('dummy');
			else if (r === mine) out.push('you');
			else if (occupied.has(r)) out.push('other');
			else out.push('empty');
		}
		return out;
	}

	/** Wild hearts the viewer is holding. They rank nothing until the viewer
	 *  assigns them to tracks in the declare step, so they're the one part of
	 *  the projection the strip above can't show. */
	const wildInHand = $derived(
		cards.filter((c) => c.player_id === currentPlayerID && c.card_suit === 'H').length
	);
</script>

<section class="standing">
	<!-- The whole row is the control, matching the other two expandables on
	     this page (.sheet-header and the help disclosure, both full-width
	     buttons). It used to be a div whose only interactive part was the
	     83.5px toggle — 21% of a 391.6px row — with the "Your standing" label
	     sitting outside the target entirely. -->
	<button
		type="button"
		class="standing-head"
		aria-expanded={boardOpen}
		aria-controls={boardOpen ? 'standing-board' : undefined}
		onclick={() => (boardOpen = !boardOpen)}
	>
		<span class="standing-label">Your standing</span>
		<span class="standing-toggle">{boardOpen ? 'hide board' : 'full board'} ▾</span>
	</button>

	{#if wildInHand > 0}
		<p class="standing-note">
			<span class="track-code wild" aria-hidden="true">{trackCode('H')}</span>
			{wildInHand} in hand — you assign these to tracks at the end.
		</p>
	{/if}

	<div class="standing-row">
		{#each TRACKS as t (t.id)}
			{@const slots = slotsFor(t.id)}
			{@const mine = slots.indexOf('you')}
			<span class="stand-cell">
				<span class="stand-track">{t.label}</span>
				<span
					class="micro"
					aria-label={mine === -1
						? `${t.label}: you hold no slot yet`
						: `${t.label}: you are in slot ${mine + 1} of 5`}
				>
					{#each slots as s, i (i)}
						<span class="mslot" class:you={s === 'you'} class:dummy={s === 'dummy'}></span>
					{/each}
				</span>
			</span>
		{/each}
	</div>

	{#if boardOpen}
		<!-- showTrackNames={false}: the .standing-row above is the header row.
		     It sits on the board's own column grid (see --board-gutter), so the
		     three names and the viewer's micro-tracks head the three columns
		     rather than repeating them one line further down. -->
		<div id="standing-board">
			<TrackBoard
				{players}
				{cards}
				{rankings}
				{committed}
				{doneFlags}
				activeTrack={null}
				{currentPlayerID}
				showTrackNames={false}
			/>
		</div>
	{/if}
</section>

<style>
	.standing {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		border: 1px solid var(--color-border);
		border-radius: 8px;
		padding: 0.5rem 0.7rem;

		/* The board's column metrics, set here because this is the one place
		   the two have to agree: .standing-row is the board's header row, so
		   its three cells must land on the board's three columns exactly.
		   TrackBoard reads both with fallbacks — its other callers render it
		   outside any .standing and are unaffected. */
		--board-gutter: 0.6rem;
		--board-col-gap: 0.4rem;
	}
	/* 0.75rem of text, but a 44px tap box: the min-height grows the target and
	   the matching negative block margin keeps the head row exactly 18px tall,
	   unchanged from when only the toggle carried this (the same idiom as the
	   header's member pills). The negative INLINE margin is the part that makes
	   the whole row the target — it bleeds back out through the card's 0.7rem
	   padding so the tappable area runs edge to edge. The downward bleed lands
	   on the Esteem micro-track, which isn't interactive — and a mis-tap there
	   opens the board, i.e. the detail view of the thing tapped. */
	.standing-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		width: calc(100% + 1.4rem);
		min-height: 44px;
		margin-block: -13px;
		margin-inline: -0.7rem;
		padding: 0 0.7rem;
		background: none;
		border: none;
		border-radius: 8px;
		color: inherit;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.standing-head:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: -2px;
	}
	/* Parchment rather than the usual muted grey for a section label: the row
	   below is now uppercase too, and at 0.68rem/0.62rem two muted uppercase
	   rows read as one list of six labels. Brightness is what separates the
	   card's title from the column headers under it. */
	.standing-label {
		color: var(--color-text);
		font-size: 0.68rem;
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}
	.standing-toggle {
		color: var(--color-accent);
		font-size: 0.75rem;
		flex: none;
	}

	/* The board's three columns, offset by its rank gutter — so a name and the
	   viewer's five slots sit centred over the column they describe whether or
	   not the board is open. (Open or closed the row doesn't move, which is
	   what keeps the toggle from shifting the page under the reader's thumb.) */
	.standing-row {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		column-gap: var(--board-col-gap);
		padding-left: calc(var(--board-gutter) + var(--board-col-gap));
	}
	.stand-cell {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.2rem;
		min-width: 0;
	}
	/* The board's old .col-label treatment, inherited wholesale: this row IS
	   the board's header row now, and PrologueView's .detail-track is where
	   the treatment comes from (uppercase, tracked, muted). */
	.stand-track {
		color: var(--color-text-muted);
		font-size: 0.62rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		white-space: nowrap;
	}
	.micro {
		display: inline-flex;
		align-items: center;
		gap: 3px;
	}
	.mslot {
		width: 7px;
		height: 7px;
		border-radius: 2px;
		background: transparent;
		border: 1px solid var(--color-border-strong);
		flex: none;
	}
	.mslot.you {
		background: var(--color-accent);
		border-color: var(--color-accent);
	}
	/* Dummies get the same dashed not-a-real-thing treatment as a wild, so the
	   strip's five slots read as five slots — three of which nobody can take
	   in a 2-player game. (This is why the wild's class is .wild and not .any:
	   what these two share is the dash, not the label — Round 3, decision 2.) */
	.mslot.dummy {
		border-style: dashed;
		border-color: var(--color-border);
		background: var(--color-surface-sunken);
	}

	.standing-note {
		display: flex;
		align-items: center;
		gap: 0.3rem;
		margin: 0;
		color: var(--color-text-faint);
		font-size: 0.72rem;
		line-height: 1.35;
	}
</style>
