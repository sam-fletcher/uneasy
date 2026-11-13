<!--
  PlayerRow.svelte — who is at the table, in one line under the title bar.

  Each player is a chip: their shape and their name in their colour, so the key
  from colour to shape is always on screen. Tapping one opens that player's
  retinue. Two small marks say the only two things worth saying here:

      a clock       the game is waiting on this player. Bright gold on you,
                    secondary on everyone else.
      a warning     one or more of this player's assets is one tear from
                    destruction but still has an empty marginalia slot. No
                    number: the count is in the retinue, where it can be acted on.

  The row spans the full width and never wraps: it scrolls sideways, with no
  scrollbar and a fade on whichever edge has more. At two and three columns
  every chip normally fits without scrolling. Your own chip reads "You" (it
  costs less width than any name and can't be mistaken for another).

  Ranks and the bright-gold ring that used to be here are gone: the clock says
  whose move it is, and the ranks are in the retinue.
-->
<script lang="ts">
	import '$lib/components/shared/chip.css';
	import { tick } from 'svelte';
	import type { Player } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import ClockMark from '$lib/components/shared/ClockMark.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';

	interface Props {
		players: Player[];
		currentPlayerID: number | null;
		/** Every player the game is waiting on. */
		waitingPlayerIDs: Set<number>;
		/** Player id → how many of their assets are needlessly at risk. Only
		 *  whether it is above zero is drawn. */
		atRiskByPlayer: Map<number, number>;
		onSelect: (playerID: number) => void;
	}

	const { players, currentPlayerID, waitingPlayerIDs, atRiskByPlayer, onSelect }: Props = $props();

	// ── Sideways scroll, with a fade on the edge that is clipped ───────────
	// A few px of tolerance absorbs sub-pixel rounding between scrollWidth and
	// clientWidth, so the fade doesn't flicker on for a row that in fact fits.
	const FADE_SLOP_PX = 4;
	let rowEl = $state<HTMLElement | null>(null);
	let fadeLeft = $state(false);
	let fadeRight = $state(false);
	let ownChipShown = false;

	function updateFade() {
		if (!rowEl) return;
		const { scrollLeft, scrollWidth, clientWidth } = rowEl;
		fadeLeft = scrollLeft > FADE_SLOP_PX;
		fadeRight = scrollLeft + clientWidth < scrollWidth - FADE_SLOP_PX;
	}

	$effect(() => {
		void players.length;
		if (!rowEl) return;
		void tick().then(updateFade);
		const observer = new ResizeObserver(updateFade);
		observer.observe(rowEl);
		return () => observer.disconnect();
	});

	// The viewer's own chip is scrolled into view once on load. The row never
	// follows whoever's move it is afterwards, so the order stays predictable.
	$effect(() => {
		if (ownChipShown || !rowEl || currentPlayerID == null || players.length === 0) return;
		ownChipShown = true;
		void tick().then(() => {
			rowEl
				?.querySelector<HTMLElement>(`[data-player-id="${currentPlayerID}"]`)
				?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
		});
	});
</script>

<div class="player-row" class:fade-left={fadeLeft} class:fade-right={fadeRight}>
	<nav class="players" aria-label="Players" bind:this={rowEl} onscroll={updateFade}>
		{#each players as player (player.id)}
			{@const isMe = player.id === currentPlayerID}
			{@const waiting = waitingPlayerIDs.has(player.id)}
			{@const atRisk = (atRiskByPlayer.get(player.id) ?? 0) > 0}
			<button
				type="button"
				class="chip compact row-chip"
				data-player-id={player.id}
				style:color={playerColor(player)}
				title={player.display_name}
				aria-label={`View ${player.display_name}'s retinue${isMe ? ' (you)' : ''}${waiting ? (isMe ? ' — your move' : ' — their move') : ''}${atRisk ? ' — some assets need marginalia' : ''}`}
				onclick={() => onSelect(player.id)}
			>
				<PlayerShape {player} />
				<span class="name">{isMe ? 'You' : player.display_name}</span>
				{#if waiting}
					<ClockMark tone={isMe ? 'you' : 'other'} />
				{/if}
				{#if atRisk}
					<!-- A copy of the warning mark in Notice.svelte (same path), in the
					     at-risk red. It stays a copy only until the marks in Notice
					     become one shared component; this then imports that. -->
					<svg class="risk" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
						<path d="M12 3.5L21.5 20H2.5zM12 10v4.5M12 17.5v0.01" />
					</svg>
				{/if}
			</button>
		{/each}
	</nav>
</div>

<style>
	/* Wraps the scroller so the edge fades can sit over it without being
	   scrolled away with it. */
	.player-row {
		position: relative;
		flex: none;
		border-bottom: 1px solid var(--color-border);
	}
	.player-row::before,
	.player-row::after {
		content: '';
		position: absolute;
		top: 0;
		bottom: 0;
		width: var(--space-5);
		pointer-events: none;
		opacity: 0;
		transition: opacity 0.15s ease;
		z-index: 1;
	}
	.player-row::before { left: 0; background: linear-gradient(to right, var(--color-bg), transparent); }
	.player-row::after { right: 0; background: linear-gradient(to left, var(--color-bg), transparent); }
	.player-row.fade-left::before { opacity: 1; }
	.player-row.fade-right::after { opacity: 1; }
	/* The hint still appears; it just doesn't ease in. */
	@media (prefers-reduced-motion: reduce) {
		.player-row::before,
		.player-row::after { transition: none; }
	}

	.players {
		display: flex;
		gap: var(--space-2);
		overflow-x: auto;
		scrollbar-width: none;
		-webkit-overflow-scrolling: touch;
		padding: var(--space-1) var(--space-4) var(--space-3);
	}
	.players::-webkit-scrollbar { display: none; }

	/* From two columns up every chip normally fits, so the row centres under
	   the centred columns; when it does overflow it starts at the left edge. */
	@media (min-width: 744px) {
		.players { justify-content: safe center; }
	}

	/* chip.css gives the shape of a chip; the player's colour is the text's. */
	.row-chip {
		flex: none;
		gap: var(--space-2);
	}

	/* Names shorten to a set width: a scrolled name is one gesture away, and the
	   full name is on the chip's title and in the retinue it opens. */
	.name {
		max-width: 12ch;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}
	@media (min-width: 744px) {
		.name { max-width: 20ch; }
	}

	.risk {
		flex: none;
		color: var(--color-danger);
	}
</style>
