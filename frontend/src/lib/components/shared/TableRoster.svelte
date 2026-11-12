<!-- TableRoster.svelte
  Who is at this table, as a list of seats.

  One component for the three lists that were drifting apart
  (adr/LOBBY_AND_CHECKLIST_PLAN.md D3): the lobby's seats, the prologue
  closing stage's ready roster, and its retinue tallies. The row vocabulary is
  borrowed from the profile page's table-card pills so all four surfaces agree:

    • shape          — the player's identity: their seat's shape in their
                       colour (PlayerShape), which replaced the old colour dot
                       so colour is never the only signal.
    • name, or You   — the current player reads "You", never their own name,
                       in the player's colour. Same convention as the header
                       chips; a roster you can't find yourself in is the
                       finding this component exists to fix.
    • clock          — the game is waiting on them (ClockMark): bright gold on
                       your own row, secondary on anyone else's. The same set
                       that marks the header chips, so the two can never
                       disagree, and now the same TREATMENT as well. It
                       replaced a gold border (and, on your row, a glow).

  There is no box. An inert seat is a plain line; a tappable one is a button
  row marked by its arrow (it opens a sheet elsewhere), with a thin rule
  between neighbours. A player's name and shape are what the row IS, so
  nothing wears a pill.

  Presence (`members`) is still accepted and still announced to a screen
  reader, but it is no longer DRAWN (owner, 2026-08-16). It was a green ring,
  and it was the only other colour channel on a seat: a green ring around a
  gold-brown fill was the single ugliest object on the lobby and profile
  screens, and it overloaded a pill that already carries an identity colour
  and a status colour. Turns here are days apart, so "online right now" is
  weak information — if it ever comes back it should come back as words
  ("last seen 2 days ago"), the way RetinueView already does it, not as a
  second hue. The prop stays because the words below depend on it.

  There is deliberately NO facilitator tag (owner, 2026-08-16). The flag gates
  exactly one thing a player can see — Start Prologue, in the lobby — and it
  breaks ties in the row 7 → 8 ending vote; both places name the person in a
  sentence where the fact has a consequence ("alice will start the game once
  everyone has arrived", "If the vote ties, alice's side wins"), which is
  worth more than a title the app never defines anywhere. Whoever the lobby is
  waiting on already wears the clock, which says the useful half.

  The caller owns the heading and the count ("3 seated · room for 2 more" vs
  "1 of 3 ready"), the per-row trailing content, and the lobby's invite chair.
-->
<script lang="ts">
	import '$lib/components/shared/tapRow.css';
	import type { Snippet } from 'svelte';
	import type { Player, PresenceMember } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import ClockMark from './ClockMark.svelte';
	import PlayerShape from './PlayerShape.svelte';
	import RowMark from './RowMark.svelte';

	interface Props {
		players: Player[];
		/** Live presence; omit for no online state. */
		members?: PresenceMember[];
		currentPlayerID: number | null;
		/** Gold row — the same set that rings the header chips. */
		waitingPlayerIDs?: Set<number>;
		/** Right-hand content per row. */
		trailing?: Snippet<[Player]>;
		/** Makes each row a button. */
		onSelect?: (playerID: number) => void;
		/**
		 * Replaces a tappable row's accessible label. A labelled button hides
		 * its own contents from a screen reader, so a caller whose `trailing`
		 * carries meaning (the closing stage's retinue counts) has to restate
		 * it here. The words this component would have appended are handed in
		 * so they can't be dropped by accident.
		 */
		rowLabel?: (player: Player, stateWords: string) => string;
		/** Lobby only: the dashed invite chair below the seated rows. */
		inviteSeat?: Snippet;
	}

	const {
		players,
		members,
		currentPlayerID,
		waitingPlayerIDs,
		trailing,
		onSelect,
		rowLabel,
		inviteSeat,
	}: Props = $props();

	const onlineIDs = $derived(
		new Set((members ?? []).filter((m) => m.online).map((m) => m.id))
	);

	// The clock is a picture, so it needs words somewhere. A tappable
	// row spends them on its aria-label (which replaces the row's text for a
	// screen reader); a plain row appends them visually-hidden, so the reading
	// order stays "You, online".
	function stateWords(p: Player, online: boolean, waiting: boolean): string {
		// Presence is only a signal when the caller supplied any: without
		// `members` every row would otherwise announce "offline".
		const presence = members ? (online ? ', online' : ', offline') : '';
		const them = p.id === currentPlayerID ? 'you' : 'them';
		const wait = waiting ? `, the game is waiting on ${them}` : '';
		return `${presence}${wait}`;
	}

	function seatLabel(p: Player, online: boolean, waiting: boolean): string {
		const words = stateWords(p, online, waiting);
		if (rowLabel) return rowLabel(p, words);
		const you = p.id === currentPlayerID ? ' (you)' : '';
		return `${p.display_name}${you}${words}`;
	}
</script>

{#snippet seatBody(p: Player, waiting: boolean)}
	<PlayerShape player={p} />
	<span class="seat-name">{p.id === currentPlayerID ? 'You' : p.display_name}</span>
	{#if waiting}
		<ClockMark tone={p.id === currentPlayerID ? 'you' : 'other'} />
	{/if}
	{#if trailing}
		<span class="seat-trailing">{@render trailing(p)}</span>
	{/if}
{/snippet}

<ul class="table-roster">
	{#each players as p (p.id)}
		{@const online = onlineIDs.has(p.id)}
		{@const waiting = waitingPlayerIDs?.has(p.id) ?? false}
		<li>
			{#if onSelect}
				<button
					type="button"
					class="seat tap-row"
					class:mine={p.id === currentPlayerID}
					style:--seat-color={playerColor(p)}
					aria-label={seatLabel(p, online, waiting)}
					onclick={() => onSelect(p.id)}
				>
					{@render seatBody(p, waiting)}
					<RowMark kind="arrow" />
				</button>
			{:else}
				<div
					class="seat"
					class:mine={p.id === currentPlayerID}
					style:--seat-color={playerColor(p)}
				>
					{@render seatBody(p, waiting)}
					<span class="sr-state">{stateWords(p, online, waiting)}</span>
				</div>
			{/if}
		</li>
	{/each}
	{#if inviteSeat}
		<li class="invite">{@render inviteSeat()}</li>
	{/if}
</ul>

<style>
	.table-roster {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
	}
	/* A seat is a plain line, 44px at least whether or not it is tappable: a
	   roster of 44px rows and a roster of 28px rows read as different objects.
	   A tappable one also carries .tap-row (tapRow.css), which gives it the
	   rule beneath and its hover and focus; an inert one draws the same rule
	   here so the two kinds sit in one list. */
	.seat {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		width: 100%;
		min-height: var(--size-tap);
		padding: var(--space-1) 0;
		border-bottom: 1px solid var(--color-border);
		color: inherit;
		font: inherit;
		text-align: left;
	}
	.seat-name {
		color: var(--seat-color, var(--color-text));
		font-size: var(--text-small);
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}
	/* Same recipe as TrackBoard's .sr-rank: off-screen for the eye, in the
	   reading order for a screen reader. Absolutely positioned, so it takes no
	   room in the line and needs no negative margin to hide. */
	.sr-state {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}
	/* Pushes whatever the caller passed to the right-hand edge. */
	.seat-trailing {
		margin-left: auto;
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		min-width: 0;
	}
	.invite { display: flex; }
</style>
