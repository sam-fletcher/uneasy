<!--
  FeedDivider.svelte — a line across the chat feed that marks a boundary, with
  its words in the middle. Centring is reserved for these: it is how a reader
  tells "a boundary" from "something somebody said".

  Three weights, one meaning each (docs/STYLE_GUIDE.md "Chat"):

      day      faint, small        the calendar day changed ("Yesterday")
      unread   main parchment      where the new messages start ("4 new"); the
                                   count lives here and not on the tab
      row      deep gold           a row of the game began: the game's own colour

  The weight is the only difference, so the three read as one family and the
  eye ranks them without having to read. Not a control: nothing here is pressed.
  Extra attributes (data-post-id, data-code, aria-label) pass straight through.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'children'> {
		kind: 'day' | 'unread' | 'row';
		children: Snippet;
	}

	const { kind, children, ...rest }: Props = $props();
</script>

<div class="feed-divider" data-kind={kind} role="separator" {...rest}>
	<span class="rule" aria-hidden="true"></span>
	<span class="label">{@render children()}</span>
	<span class="rule" aria-hidden="true"></span>
</div>

<style>
	.feed-divider {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		flex: none;
		color: var(--color-text-faint);
		font-size: var(--text-label);
	}

	.rule {
		flex: 1;
		height: 1px;
		background: var(--color-border);
	}

	/* Row boundaries and boundary posts can be whole sentences (the Shake-Up's
	   banners), so the label wraps instead of stretching past the column. */
	.label {
		max-width: 75%;
		text-align: center;
		line-height: 1.4;
	}

	.feed-divider[data-kind='unread'] {
		color: var(--color-text);
	}
	.feed-divider[data-kind='unread'] .rule {
		background: var(--color-text-faint);
	}

	.feed-divider[data-kind='row'] {
		color: var(--color-accent);
		font-size: var(--text-small);
	}
</style>
