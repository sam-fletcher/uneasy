<!--
  PlayerName.svelte — a player's name in text. The one component that owns how
  a name looks (the look itself is inlineText.css; the table of exceptions is
  in the style guide under "Inline text").

  In their colour wherever it sits: sentences, finished lines, headings, faint
  notes. No shape inside a sentence: the words around it say who it is. Pass
  `label` only where the name stands alone as a label (a byline, the who-line,
  a chip, the player row): then the player's shape comes first, drawn by the
  same <PlayerShape> as everywhere else.

  Inside a control the control's text colour wins, by inheritance (see
  inlineText.css), so a caller never passes anything to say so. `fixed` is for
  a player chip, whose name keeps the colour because the chip is the player.

  Three ways to say who:
      <PlayerName player={p} />               a Player in hand
      <PlayerName {players} id={someID} />    an id, looked up
      <PlayerName player={null} name="Unknown" />  nobody you can see: grey,
                                              upright, never a shape

  `name` overrides the text (a stand-in's label, "Unknown"). "you" and "your"
  are plain words and aren't wrapped in this; the possessive 's stays outside
  too, so only the name itself is coloured.
-->
<script lang="ts">
	import '$lib/components/shared/inlineText.css';
	import type { Player } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';

	type Who = Pick<Player, 'id' | 'display_name' | 'token_color' | 'seat_order'>;

	interface Props {
		player?: Who | null;
		/** With `id`: the roster to find the player in. */
		players?: Who[];
		id?: number | null;
		/** Text to show instead of the player's display name. */
		name?: string;
		/** The name stands alone as a label: the shape goes first. */
		label?: boolean;
		/** Keeps the player's colour inside a chip (a player chip). */
		fixed?: boolean;
		/** Shape size, when labelled. */
		size?: number;
	}

	const { player, players, id, name, label = false, fixed = false, size = 12 }: Props = $props();

	const who = $derived<Who | null>(
		player !== undefined
			? player
			: id != null && players
				? (players.find((p) => p.id === id) ?? null)
				: null,
	);
	const text = $derived(name ?? who?.display_name ?? '?');
	// Only a resolved player has a colour; anyone else falls to the CSS grey.
	const color = $derived(who ? playerColor(who) : undefined);
</script>

<span
	class={['player-name', label && who && 'labelled', fixed && 'fixed', !who && 'unknown']}
	style:--player-name-color={color}
>{#if label && who}<PlayerShape player={who} {size} /><span class="player-name-text">{text}</span>{:else}{text}{/if}</span>
