<!--
  NewDot.svelte — the pale "new" dot.

  Means one thing everywhere: new since you last looked. It sits on the Chat
  tab, on the Laws and Rumors sub-tabs (and on the Public Record tab whenever
  either has one), beside a law or rumor you haven't seen, and on a profile
  row with unread chat. Its form is what carries "new"; colour is kept for
  saying whose something is (bright gold means you), so the dot is
  deliberately not gold.

  It is a status dot, so it is the one thing allowed to be a circle. It is
  pale (the main text colour) rather than a meaning colour, and always has
  words available to a screen reader.

  A NUMBER never goes beside it in a tab: totals live in the lists, and the
  unread count lives in the chat's "new messages" divider.
-->
<script lang="ts">
	interface Props {
		/** What a screen reader says. */
		label?: string;
	}

	const { label = 'new' }: Props = $props();
</script>

<span class="new-dot" role="img" aria-label={label}></span>

<style>
	.new-dot {
		display: inline-block;
		flex: none;
		width: 8px;
		height: 8px;
		border-radius: var(--radius-dot);
		background: var(--color-text);
	}
</style>
