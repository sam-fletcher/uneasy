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
    • green ring     — online right now (needs `members`; omit for no presence)
    • gold fill      — the game is waiting on them. The same set that rings the
                       header chips, so the two can never disagree.

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
		inviteSeat,
	}: Props = $props();

	const onlineIDs = $derived(
		new Set((members ?? []).filter((m) => m.online).map((m) => m.id))
	);

	// The ring and the gold fill are pure colour, so they need words somewhere.
	// A tappable row spends them on its aria-label (which replaces the row's
	// text for a screen reader); a plain row appends them visually-hidden, so
	// the reading order stays "You, facilitator, online".
	function stateWords(p: Player, online: boolean, waiting: boolean): string {
		// Presence is only a signal when the caller supplied any: without
		// `members` every row would otherwise announce "offline".
		const presence = members ? (online ? ', online' : ', offline') : '';
		const them = p.id === currentPlayerID ? 'you' : 'them';
		const wait = waiting ? `, the game is waiting on ${them}` : '';
		return `${presence}${wait}`;
	}

	function seatLabel(p: Player, online: boolean, waiting: boolean): string {
		const you = p.id === currentPlayerID ? ' (you)' : '';
		const facilitator = p.is_facilitator ? ', facilitator' : '';
		return `${p.display_name}${you}${facilitator}${stateWords(p, online, waiting)}`;
	}
</script>

{#snippet seatBody(p: Player)}
	<span class="seat-dot" aria-hidden="true"></span>
	<span class="seat-name">{p.id === currentPlayerID ? 'You' : p.display_name}</span>
	{#if p.is_facilitator}
		<span class="seat-tag">facilitator</span>
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
					class="seat tappable"
					class:online
					class:waiting
					style:--seat-color={playerColor(p)}
					aria-label={seatLabel(p, online, waiting)}
					onclick={() => onSelect(p.id)}
				>
					{@render seatBody(p)}
				</button>
			{:else}
				<div class="seat" class:online class:waiting style:--seat-color={playerColor(p)}>
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
	/* Online = a ring around the seat, exactly as the profile card's pills do
	   it; its PRESENCE is the signal (it survives colour-blindness) and the
	   muted green echoes the retinue's online dot. */
	.seat.online { box-shadow: 0 0 0 1px var(--color-online); }
	.seat.waiting {
		background: var(--color-chip-gold-bg);
		border-color: var(--color-chip-gold-border);
	}
	.seat.tappable { cursor: pointer; }
	.seat.tappable:hover { border-color: var(--color-border-strong); }
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
	.seat-tag {
		flex: none;
		font-size: 0.65rem;
		background: var(--color-chip-violet-bg);
		border: 1px solid var(--color-chip-violet-border);
		color: var(--color-chip-violet-text);
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
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
