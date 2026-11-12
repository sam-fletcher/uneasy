<!--
  ClockMark.svelte — the waiting glyph.

  A clock on a player's chip or row means the game is WAITING on them. The
  two tones are the whole vocabulary:

      tone="you"     bright gold: the game is waiting on YOU. It appears only
                     when that is true of the viewer, because the game is
                     nearly always waiting on someone.
      tone="other"   secondary grey: the game is waiting on this other player.

  Bright gold is reserved for exactly three things, your picks, your focus
  ring and this glyph on your own chip, so don't borrow it for anything else
  and don't draw this mark for a deadline (it means "waiting").

  The drawing is the `clock` family in LogMark.svelte, so the house marks
  have one home. This wrapper adds the size, the colour rule and the label.
  Colour is never the only signal: a row that shows the clock also says the
  same in words somewhere (a screen-reader label, the open step's heading).
  Pass `label` when the clock stands alone, leave it off when words are beside it.
-->
<script lang="ts">
	import LogMark from '../LogMark.svelte';

	interface Props {
		tone?: 'you' | 'other' | 'inherit';
		/** Edge length in px. */
		size?: number;
		/** Accessible name, when no words beside the mark say it. */
		label?: string;
	}

	const { tone = 'inherit', size = 16, label }: Props = $props();
</script>

<span
	class="clock-mark"
	data-tone={tone}
	style:width="{size}px"
	style:height="{size}px"
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
>
	<LogMark family="clock" />
</span>

<style>
	.clock-mark {
		display: inline-flex;
		flex: none;
	}
	.clock-mark[data-tone='you'] { color: var(--color-you); }
	.clock-mark[data-tone='other'] { color: var(--color-text-secondary); }
</style>
