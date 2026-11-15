<!--
  Checkbox.svelte — the house checkbox: a drawn box, the words beside it and an
  optional explanation under them, in one tap row.

  There are no plain browser checkboxes in the app. Use this for a yes-or-no
  that is SUBMITTED with the rest of a form ("Surrender after this payment",
  "Keep it secret for now"): nothing happens until the player presses the
  step's button. A setting that acts the moment it is flipped is a switch (the
  chat's "Hide bookkeeping"), and a choice among a few words is a <Chip>. The
  look is `checkbox.css` (read its header for the rules: the asset card's box,
  ticked = the gold border and a tick, locked = a hairline and faint words);
  docs/STYLE_GUIDE.md has the recipe.

      label        the words beside the box
      description  what ticking it does, in secondary text under the words, in
                   words on the screen: a `title` can't be read on a phone
      reason       why it is locked, under the description, shown only while
                   `disabled`

  Under the drawn box is a real <input type="checkbox">, hidden but focusable,
  so `bind:checked`, the keyboard (Space) and screen readers work natively: the
  label names it and the description and reason describe it. Lock it with
  `disabled`; a checkbox inside a disabled <fieldset> is locked too and reads as
  locked. Extra attributes (name, data-*, onchange) go straight to the <input>.

  The asset card draws the same box on a <span role="checkbox"> inside its
  header button, which can't hold an <input>; it shares only the look.
-->
<script lang="ts">
	import '$lib/components/shared/tapRow.css';
	import '$lib/components/shared/checkbox.css';
	import type { HTMLInputAttributes } from 'svelte/elements';
	import ToneMark from '$lib/components/shared/ToneMark.svelte';

	interface Props extends Omit<HTMLInputAttributes, 'type' | 'children' | 'title' | 'checked' | 'class'> {
		label: string;
		checked?: boolean;
		description?: string;
		reason?: string;
		class?: string;
	}

	let {
		label,
		checked = $bindable(false),
		description,
		reason,
		disabled = false,
		class: extra,
		...rest
	}: Props = $props();

	const uid = $props.id();
	const shownReason = $derived(disabled && reason ? reason : undefined);
	const describedBy = $derived(
		[description && `${uid}-desc`, shownReason && `${uid}-reason`].filter(Boolean).join(' ') || undefined,
	);
</script>

<label class={['tap-row', 'check-row', extra]}>
	<!-- The drawn box must be the input's NEXT sibling: checkbox.css styles it
	     from `.check-input:checked + .check-box`. -->
	<span class="check-lead">
		<input
			type="checkbox"
			class="check-input"
			bind:checked
			{disabled}
			aria-labelledby="{uid}-label"
			aria-describedby={describedBy}
			{...rest}
		/>
		<span class="check-box" aria-hidden="true">
			{#if checked}<ToneMark tone="success" />{/if}
		</span>
	</span>
	<span class="check-text">
		<span class="check-label" id="{uid}-label">{label}</span>
		{#if description}<span class="check-desc" id="{uid}-desc">{description}</span>{/if}
		{#if shownReason}<span class="check-desc" id="{uid}-reason">{shownReason}</span>{/if}
	</span>
</label>
