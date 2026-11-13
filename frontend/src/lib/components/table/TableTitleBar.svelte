<!--
  TableTitleBar.svelte — the top row of the table page: Home, the table's title,
  and Help. Nothing else.

  The title names the middle level of detail ("Row 6 · Seek Answers",
  "Prologue · Choosing"): the phase alone is too coarse to earn the space, and
  the step belongs to the Play pane. It is built by tableTitle() from the game
  and the row state's `title_plan_type`, the same words the profile page shows
  for this table. The bar spans the whole window, above the columns, because it
  is about the whole table and not one pane.
-->
<script lang="ts">
	import HelpButton from '$lib/components/HelpButton.svelte';

	interface Props {
		title: string;
		gameId: string;
		route: string;
		phase?: string;
	}

	const { title, gameId, route, phase }: Props = $props();
</script>

<header class="title-bar">
	<a class="home" href="/profile" aria-label="Home">
		<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
			<path d="M3.5 11L12 4l8.5 7M6 9.5V20h12V9.5" />
		</svg>
	</a>
	<h1 class="title">{title}</h1>
	<HelpButton {gameId} {route} {phase} />
</header>

<style>
	.title-bar {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		flex: none;
		min-height: var(--layout-title-bar);
		padding: 0 var(--space-1);
	}

	.home {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: var(--size-tap);
		height: var(--size-tap);
		flex: none;
		color: var(--color-text-secondary);
		border-radius: var(--radius-control);
		text-decoration: none;
	}
	.home:hover { color: var(--color-text); background: var(--color-surface); }
	.home:focus-visible { outline: 2px solid var(--color-you); outline-offset: 2px; }

	/* The game's colour, at the heading size, centred between the two buttons.
	   One line: a long title shortens rather than growing the bar. */
	.title {
		flex: 1;
		min-width: 0;
		margin: 0;
		text-align: center;
		font-size: var(--text-heading);
		font-weight: 400;
		color: var(--color-accent);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
</style>
