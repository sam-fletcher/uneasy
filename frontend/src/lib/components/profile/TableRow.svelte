<!--
  TableRow.svelte — one of your tables, as a row of the profile's list.

  A tappable row, not a card: the arrow at its end says it opens something, a
  hairline separates it from the next, and nothing about it is boxed or
  filled. Boxes are for controls and game pieces; a row is pressed as a
  whole, and its arrow is what says so. Two lines:

    first   the table's own title ($lib/tableTitle), the same words the table
            page's title bar says, so the row tells you what you'll find when
            you open it. At its end, when chat has gone unread, the pale
            "new" dot and "N new" in plain words.
    second  who is at the table: each player as their shape and their name in
            their colour. They are not drawn as chips: the row is the control,
            and a chip shape would say each name can be pressed on its own.
            The clock sits beside whoever the game is waiting on, bright gold
            on you and grey on anyone else. Your own name stays your name, not
            "You": your seat, and so your shape and colour, can differ from
            one table to the next, and the name is what stays the same.

  No join code. Who is seated is how you recognise a table, and you only need
  the code to invite someone, which you do from the table itself.

  Two focusable things, in reading order: open the table, then (when this
  table is waiting on you) the reminder bell. A <button> cannot sit inside an
  <a>, so the row is a positioned box and the two are siblings; the link
  stretches over the whole row through ::after so the full row stays one
  tap target, and the bell sits above that overlay on its own layer. The
  arrow is drawn beneath the overlay, so a tap on it opens the table too.
-->
<script lang="ts">
	import '$lib/components/shared/tapRow.css';
	import type { MyTable } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import { tableTitle } from '$lib/tableTitle';
	import { tableBell } from '$lib/tableBell';
	import BellGlyph from '$lib/components/BellGlyph.svelte';
	import ClockMark from '$lib/components/shared/ClockMark.svelte';
	import NewDot from '$lib/components/shared/NewDot.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';
	import RowMark from '$lib/components/shared/RowMark.svelte';

	interface Props {
		table: MyTable;
		/** This table's bell request is in flight, so a second tap can't fire. */
		mutePending?: boolean;
		/** Ask the server to silence (true) or un-silence (false) this wait. */
		onMute?: (table: MyTable, setMutedTo: boolean) => void;
	}

	const { table: t, mutePending = false, onMute }: Props = $props();

	const ended = $derived(t.phase === 'ended');
	const bell = $derived(tableBell(t));
	const unread = $derived(ended ? 0 : t.unread_count);

	// "99+ new": the number hints at which table has more waiting, and stops
	// being exact past a screenful of chat.
	const unreadWords = $derived(`${unread > 99 ? '99+' : unread} new`);

	// The clock and the colour are pictures, so each player's row also carries
	// the words for a screen reader (visually hidden, in reading order).
	function stateWords(playerID: number, waited: boolean): string {
		const you = playerID === t.player_id;
		return `${you ? ' (you)' : ''}${waited ? `, the game is waiting on ${you ? 'you' : 'them'}` : ''}`;
	}
</script>

<li class="table-row tap-row">
	<a class="table-link" href={`/table/${t.game_id}`}>
		<span class="title-line">
			<span class="title">{tableTitle(t)}</span>
			{#if unread > 0}
				<!-- The words say it, so the dot is decoration for a screen reader. -->
				<span class="unread">
					<span class="unread-dot" aria-hidden="true"><NewDot /></span>
					<span class="unread-words">{unreadWords}</span>
				</span>
			{/if}
		</span>
		<span class="players">
			{#each t.players as p (p.id)}
				{@const waited = !ended && t.waiting_on_player_ids.includes(p.id)}
				<span class="player" style:--player-color={playerColor(p)}>
					<PlayerShape player={p} />
					<span class="player-name">{p.display_name}</span>
					{#if waited}
						<ClockMark tone={p.id === t.player_id ? 'you' : 'other'} />
					{/if}
					<span class="sr-only">{stateWords(p.id, waited)}</span>
				</span>
			{/each}
		</span>
	</a>
	{#if bell}
		{#if bell.setMutedTo === null}
			<!-- Reminders stopped on their own, so this is a status, not a
			     control: a button here would offer to silence something
			     already silent. -->
			<span class="bell bell-static" role="img" title={bell.title} aria-label={bell.label}>
				<BellGlyph size={20} struck={bell.struck} />
			</span>
		{:else}
			<button
				class="bell"
				type="button"
				aria-pressed={bell.struck}
				title={bell.title}
				aria-label={bell.label}
				disabled={mutePending}
				onclick={() => onMute?.(t, bell.setMutedTo as boolean)}
			>
				<BellGlyph size={20} struck={bell.struck} />
			</button>
		{/if}
	{/if}
	<RowMark kind="arrow" />
</li>

<style>
	/* The row is the positioned box: it holds the link, the bell and the arrow
	   side by side. The shared .tap-row recipe (shared/tapRow.css) supplies its
	   layout, 44px minimum height and the hairline below. What it can't supply
	   is the pressable part: that recipe styles the <a> or <button> itself
	   (a:hover, a:focus-visible), but here the <a> can't hold the bell, so it is
	   the li that carries the hover, and the link's ::after carries the focus
	   ring (below). Only the extra padding of a two-line row is overridden. */
	.table-row {
		position: relative;
		padding: var(--space-3) 0;
		/* The bell's strike is cut in the colour it sits on. */
		--bell-strike-bg: var(--color-bg);
	}
	@media (hover: hover) {
		.table-row:hover {
			background: var(--color-surface);
			--bell-strike-bg: var(--color-surface);
		}
	}

	.table-link {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		color: inherit;
		text-decoration: none;
	}
	.table-link::after {
		content: '';
		position: absolute;
		inset: 0;
	}
	/* The ring is drawn on the overlay, so it traces the whole row, and INSIDE
	   it so the list's edges never clip it. Bright gold: it is yours, the
	   keyboard user's. */
	.table-link:focus-visible {
		outline: none;
	}
	.table-link:focus-visible::after {
		outline: 2px solid var(--color-you);
		outline-offset: -2px;
	}

	.title-line {
		min-height: 1.5rem;
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	.title {
		min-width: 0;
		font-size: var(--text-body);
		color: var(--color-text);
	}
	/* "4 new" sits at the end of the first line, in plain main text. The dot
	   marks that something is unread and the number says how much. */
	.unread {
		margin-left: auto;
		flex: none;
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-label);
		color: var(--color-text);
		white-space: nowrap;
	}
	.unread-dot {
		display: inline-flex;
	}

	/* The players wrap onto more lines rather than scrolling: this is a list to
	   scan, and the longest roster is five names. */
	.players {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-1) var(--space-3);
	}
	.player {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		font-size: var(--text-small);
		color: var(--player-color);
		min-width: 0;
	}
	/* A long name costs an ellipsis, not a row: 26ch is the backstop the header
	   chips use for a pathological run of wide glyphs. */
	.player-name {
		max-width: 26ch;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}
	/* Off-screen for the eye, in the reading order for a screen reader. */
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}

	/* 44px of tap target around a 20px glyph, the house minimum; the weight
	   comes from the glyph, not the hit area. Above the link's overlay, so a tap
	   on it never also opens the table. */
	.bell {
		position: relative;
		z-index: 1;
		flex: none;
		display: flex;
		align-items: center;
		justify-content: center;
		width: var(--size-tap);
		height: var(--size-tap);
		padding: 0;
		border: none;
		border-radius: var(--radius-control);
		background: none;
		/* Plain text, not a de-emphasised grey: a greyed bell would read as
		   "reminders are off", the opposite of what an un-struck bell means.
		   Not gold either: gold on this row already means you (the clock). */
		color: var(--color-text);
	}
	button.bell {
		cursor: pointer;
	}
	button.bell:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: -2px;
	}
	button.bell:disabled {
		opacity: 0.5;
		cursor: default;
	}
	/* The two struck states are told apart by colour, and colour alone has to do
	   it: the other difference is the title attribute, which a touch device
	   never shows. Which register each takes follows agency, not severity:
	     * silenced by the player is their own undoable choice with nothing
	       wrong about it, so it takes the quiet secondary text;
	     * given up on is the system's doing and the genuinely new information
	       (nobody is being nudged here any more), so it takes the warning
	       orange, as the Retinue's reminder line does.
	   Not red: that is spoken for by errors and things at risk. */
	button.bell[aria-pressed='true'] {
		color: var(--color-text-secondary);
	}
	.bell-static {
		color: var(--color-warning);
	}
</style>
