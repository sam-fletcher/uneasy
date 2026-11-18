<!--
  ConnectionNotice.svelte — the strip that says the table has stopped updating.

  It shows while the live connection is down (the page decides when: ws.ts waits
  out short blips first) and is the one global notice in the app. A dropped
  connection is not about any one pane or step, and the board looks perfectly
  fine while it is wrong, so it can't live inside a step: it sits across the
  whole frame, between the player row and the three panes, in the same slot as
  the page's load error.

  It is a warning Notice (a mark and orange words, no box, no tint) with the
  house secondary button beside it. Warning rather than danger because nothing
  is lost: the table is catching up the moment the connection returns. The
  words carry the meaning and the colour only repeats it. Notice's `live` makes
  a screen reader announce it as it appears.

  The button is "Try now": the retry loop is already running, and this just
  skips the wait. While that attempt is in flight it reads "Trying…" and
  ignores taps, so a player on a dead network sees that something happened
  rather than a button that looks broken. It stays aria-disabled rather than
  disabled, so it keeps focus for a keyboard user.
-->
<script lang="ts">
	import Notice from '$lib/components/shared/Notice.svelte';
	import '$lib/components/shared/actionButton.css';

	interface Props {
		/** A retry the player asked for is in flight. */
		trying: boolean;
		onTry: () => void;
	}

	const { trying, onTry }: Props = $props();
</script>

<div class="connection-notice">
	<div class="words">
		<Notice tone="warning" live>Connection lost. This table isn't updating. Reconnecting…</Notice>
	</div>
	<button
		type="button"
		class="action-btn secondary"
		aria-disabled={trying ? 'true' : undefined}
		onclick={() => {
			if (!trying) onTry();
		}}
	>{trying ? 'Trying…' : 'Try now'}</button>
</div>

<style>
	/* The words and the button are one centred group, like the title and the
	   player chips above it. The words' column is `minmax(0, max-content)`: as
	   wide as the sentence when there is room (a desktop, where the button then
	   sits right beside it) and no wider than what the button leaves when there
	   isn't (a phone, where the sentence wraps to two lines and the group fills
	   the row). The button never shrinks, and never drops to a line of its own.
	   The gutter matches the load error's, which this sits beside. */
	.connection-notice {
		display: grid;
		grid-template-columns: minmax(0, max-content) auto;
		justify-content: center;
		align-items: center;
		gap: var(--space-2) var(--space-4);
		flex: none;
		padding: var(--space-2) var(--space-4);
	}
	.words { min-width: 0; }
</style>
