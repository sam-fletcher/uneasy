<!--
  SpanHeader.svelte — the header row of a stretch of the feed that folds: a
  plan's resolution, or a scene.

  It is the only part of the span that can be pressed, so it is the only part
  drawn as a control: a tappable row (tapRow.css), 44px tall, a hairline under it,
  and the caret at its end that says "this expands in place". The span's lines sit
  indented behind a thin rule under it; there is no card and no box.

      [mark]  Name  [preparer's shape]  STATUS                [new dot]  [caret]

  - the mark is the family's own (the flag for a plan, the barbute for war, the
    quotation marks for a scene), 16px, faint unless the caller colours it;
  - the preparer's shape is there for a plan, because their colour is the only
    thing that says whose plan it is and no name sits beside it;
  - the status is a status word (small capitals, never a chip): "Resolving",
    "Make", "Mar", "Live";
  - the pale dot shows only while the span is folded and holds something unread,
    so a collapsed span still says there is news inside it;
  - a span that cannot fold (the live scene) has no caret and is disabled.

  Extra attributes (data-post-id, data-plan-id, class) pass to the <button>.
-->
<script lang="ts">
	import '$lib/components/shared/tapRow.css';
	import '$lib/components/shared/statusText.css';
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import type { Player } from '$lib/api';
	import type { ResolutionStatus } from '$lib/chatView';
	import LogMark from '$lib/components/LogMark.svelte';
	import BarbuteMark from '$lib/components/shared/BarbuteMark.svelte';
	import NewDot from '$lib/components/shared/NewDot.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';
	import RowMark from '$lib/components/shared/RowMark.svelte';

	interface Props extends Omit<HTMLButtonAttributes, 'children' | 'type'> {
		/** The LogMark family for the mark; 'war' draws the barbute in war red. */
		family: string;
		/** Paints the mark (a scene's focus player's colour). Default: faint. */
		markColor?: string;
		title: string;
		/** Whose plan it is; their shape is drawn after the name. */
		preparer?: Pick<Player, 'display_name' | 'token_color' | 'seat_order'> | null;
		status?: ResolutionStatus | null;
		expanded: boolean;
		/** The span can't be folded (the scene that is happening now): the header is
		 *  disabled and has no caret. */
		locked?: boolean;
		/** False when there is nothing inside to fold. The header can still be
		 *  pressed (it keeps its aria-expanded), it just carries no caret. */
		caret?: boolean;
		/** Something unread is inside. Shown only while folded. */
		unread?: boolean;
	}

	const {
		family,
		markColor,
		title,
		preparer = null,
		status = null,
		expanded,
		locked = false,
		caret = true,
		unread = false,
		class: extra,
		...rest
	}: Props = $props();
</script>

<button
	type="button"
	class={['tap-row', 'span-header', extra]}
	aria-expanded={expanded}
	disabled={locked}
	{...rest}
>
	<span class="span-mark" style:color={markColor} aria-hidden="true">
		{#if family === 'war'}
			<BarbuteMark />
		{:else}
			<LogMark {family} />
		{/if}
	</span>
	<span class="span-title">{title}</span>
	{#if preparer}
		<PlayerShape player={preparer} label={preparer.display_name} />
	{/if}
	{#if status}
		<span class={['status-word', 'span-status', status.tone !== 'neutral' && status.tone]}>{status.word}</span>
	{:else}
		<span class="span-status"></span>
	{/if}
	{#if unread && !expanded}
		<NewDot label="new in this span" />
	{/if}
	{#if !locked && caret}
		<RowMark kind="caret" open={expanded} />
	{/if}
</button>

<style>
	.span-header {
		font-size: var(--text-small);
		color: var(--color-text);
	}
	.span-header:disabled {
		cursor: default;
	}
	.span-header:disabled:hover {
		background: none;
	}

	/* The family mark's 16px slot. Faint, like the log lines' marks: the header
	   says what the span is in words, so the mark is a hint. */
	.span-mark {
		display: flex;
		flex: none;
		width: 16px;
		height: 16px;
		color: var(--color-text-faint);
	}

	/* A scene's banner can be a whole phrase, so the title is the one thing that
	   shrinks and wraps; the status and caret keep their own width and are never
	   pushed out of the row (a long banner once cut "Live" to "Liv"). */
	.span-title {
		min-width: 0;
		overflow-wrap: anywhere;
	}

	/* Takes the free space, so the shape sits right after the name and the
	   status right after the shape while the caret stays at the far end. It never
	   shrinks below its own words. */
	.span-status {
		flex: 1 0 auto;
		padding-left: var(--space-1);
	}
</style>
