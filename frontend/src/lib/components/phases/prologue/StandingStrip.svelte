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
	<div class="standing-head">
		<span class="standing-label">Your standing</span>
		<button type="button" class="standing-toggle" aria-expanded={boardOpen} onclick={() => (boardOpen = !boardOpen)}>
			{boardOpen ? 'hide board' : 'full board'} ▾
		</button>
	</div>

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

	{#if wildInHand > 0}
		<p class="standing-note">
			<span class="track-code wild" aria-hidden="true">WLD</span>
			{wildInHand} in hand — you assign these to tracks at the end.
		</p>
	{/if}

	{#if boardOpen}
		<TrackBoard {players} {cards} {rankings} {committed} {doneFlags} activeTrack={null} {currentPlayerID} />
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
	}
	.standing-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.standing-label {
		color: var(--color-text-muted);
		font-size: 0.68rem;
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}
	/* 0.75rem of text, but a 44px tap box: the padding grows the target and the
	   matching negative margin keeps the head row exactly as tall as the label
	   beside it (the same idiom as the header's member pills). The downward
	   bleed lands on the Esteem micro-track, which isn't interactive — and a
	   mis-tap there opens the board, i.e. the detail view of the thing tapped. */
	.standing-toggle {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		margin-block: -13px;
		padding: 0 0.5rem;
		margin-inline: -0.5rem;
		background: none;
		border: none;
		color: var(--color-accent);
		font: inherit;
		font-size: 0.75rem;
		flex: none;
		cursor: pointer;
	}
	.standing-toggle:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 2px;
	}

	.standing-row {
		display: flex;
		justify-content: space-between;
		gap: 0.4rem;
	}
	.stand-cell {
		display: inline-flex;
		flex-direction: column;
		gap: 0.2rem;
		min-width: 0;
	}
	.stand-track {
		color: var(--color-text-secondary);
		font-size: 0.72rem;
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
	/* Dummies get the same dashed not-a-real-thing treatment as WLD, so the
	   strip's five slots read as five slots — three of which nobody can take
	   in a 2-player game. */
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
