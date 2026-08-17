<!-- TableRoster.svelte
  Who is at this table, as a list of seats.

  One component for the three lists that were drifting apart
  (adr/LOBBY_AND_CHECKLIST_PLAN.md D3): the lobby's seats, the prologue
  closing stage's ready roster, and its retinue tallies. The row vocabulary is
  borrowed from the profile page's table-card pills so all four surfaces agree:

    • colour dot     — the player's identity colour (playerColor)
    • name, or You   — the current player reads "You", never their own name.
                       Same convention as WaitingOnBar and the header chips;
                       a roster you can't find yourself in is the finding this
                       component exists to fix.
    • gold frame     — the game is waiting on them. The same set that rings the
                       header chips, so the two can never disagree, and now the
                       same TREATMENT as well (see .seat.waiting below).

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
  waiting on already wears the gold fill, which says the useful half.

  The caller owns the heading and the count ("3 seated · room for 2 more" vs
  "1 of 3 ready"), the per-row trailing content, and the lobby's invite chair.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { Player, PresenceMember } from '$lib/api';
	import { playerColor } from '$lib/playerColor';

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

	// The gold frame is pure colour, so it needs words somewhere. A tappable
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

{#snippet seatBody(p: Player)}
	<span class="seat-dot" aria-hidden="true"></span>
	<span class="seat-name">{p.id === currentPlayerID ? 'You' : p.display_name}</span>
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
					class="seat tappable"
					class:waiting
					class:mine={p.id === currentPlayerID}
					style:--seat-color={playerColor(p)}
					aria-label={seatLabel(p, online, waiting)}
					onclick={() => onSelect(p.id)}
				>
					{@render seatBody(p)}
				</button>
			{:else}
				<div
					class="seat"
					class:waiting
					class:mine={p.id === currentPlayerID}
					style:--seat-color={playerColor(p)}
				>
					{@render seatBody(p)}
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
		gap: 0.4rem;
	}
	.seat {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		/* Touch minimum, whether or not the row is tappable: a roster of
		   44px rows and a roster of 28px rows read as different objects. */
		min-height: 44px;
		padding: 0.35rem 0.6rem;
		background: var(--color-surface-2);
		border: 1px solid var(--color-border);
		border-radius: 999px;
		color: inherit;
		font: inherit;
		text-align: left;
	}
	/* Waiting-on = a gold FRAME, never a fill — the chat-bar ruling (gold
	   arrives as the label/frame) applied to the one place that still broke
	   it. The fill this replaces was `--color-chip-gold-bg`, the app's largest
	   gold slab, and it measured ΔE 4.8 from the prologue closing stage's
	   orange "last chance" row: under ADR-009's own ΔE 6 near-duplicate bar,
	   which is exactly why the two read as the same brown (owner, 2026-08-16).
	   You cannot retune your way out of that — every orange fill in the ramp
	   lands ΔE 3–5 from gold-850, because two warm hues as dark low-chroma
	   washes over near-black ARE the same brown. One of them had to stop
	   being a fill, and the ruling says which.

	   Two tiers, one hue, lifted verbatim from the header chips
	   (routes/table/[id]/+page.svelte, `.member.waiting`): a plain gold border
	   for another player (information), border + glow for yourself (act now).
	   The strip and the roster now say the same thing the same way, which is
	   the point — they already read from the same waiting set. */
	.seat.waiting { border-color: var(--color-accent); }
	.seat.waiting.mine {
		box-shadow:
			0 0 0 1px var(--color-accent),
			0 0 8px color-mix(in srgb, var(--color-accent) 45%, transparent);
	}
	.seat.tappable { cursor: pointer; }
	.seat.tappable:hover { border-color: var(--color-border-strong); }
	/* Hover is a plain-class selector too, so without this a tappable waiting
	   seat would lose its gold border under the finger — which, now that the
	   border is the ONLY signal, would erase it. No caller combines the two
	   today; this keeps the next one from finding out the hard way. */
	.seat.waiting.tappable:hover { border-color: var(--color-accent-hover); }
	.seat.tappable:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}
	.seat-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--seat-color, var(--color-text-muted));
		flex-shrink: 0;
	}
	.seat-name {
		color: var(--color-text);
		font-size: 0.9rem;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}
	/* Same recipe as TrackBoard's .sr-rank: off-screen for the eye, in the
	   reading order for a screen reader. */
	.sr-state {
		position: absolute;
		width: 1px;
		height: 1px;
		margin: -1px;
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
		gap: 0.4rem;
		min-width: 0;
	}
	.invite { display: flex; }
</style>
