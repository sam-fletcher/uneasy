<!--
  BarbuteMark.svelte — war's mark: an open-faced helmet with a T-shaped opening.

  One mark means war everywhere: war's lines in the log, the war band on the
  Play tab, and the title of the war's panel. It is drawn in war red
  (`--color-danger`), the colour of loss, because a war is a loss for everyone
  in it. Red here comes with a word beside it ("war", the war's name), never
  alone.

  The drawing is the `war` family in LogMark.svelte. Unlike the flag it
  replaces on war lines, nothing else in the set shares it. Use `tone="inherit"`
  only where a parent already sets a colour on purpose (a muted, finished line).
-->
<script lang="ts">
	import LogMark from '../LogMark.svelte';

	interface Props {
		tone?: 'war' | 'inherit';
		/** Edge length in px. */
		size?: number;
		/** Accessible name, when no words beside the mark say it. */
		label?: string;
	}

	const { tone = 'war', size = 16, label }: Props = $props();
</script>

<span
	class="barbute-mark"
	data-tone={tone}
	style:width="{size}px"
	style:height="{size}px"
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
>
	<LogMark family="war" />
</span>

<style>
	.barbute-mark {
		display: inline-flex;
		flex: none;
	}
	.barbute-mark[data-tone='war'] { color: var(--color-danger); }
</style>
