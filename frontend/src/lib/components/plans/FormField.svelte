<!-- FormField.svelte
  Standard label-over-content wrapper for plan forms. Replaces the
  inline pattern:

      <div class="form-label">
          <span class="form-label-text">My Label:</span>
          …chips, cards, etc.
      </div>

  Use this whenever the labelled content is more than a single native
  input (chip rows, AssetCardSelectable lists, multi-field clusters).
  For plain `<input>` / `<textarea>` fields where the native
  `<label>`-wraps-the-input pattern carries useful screen-reader
  association, keep the explicit `<label class="form-label">` pattern.

  The trailing `:` on labels is conventional in this codebase — pass
  the label without it and FormField appends one (skipped if the label
  already ends in punctuation).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		label: string;
		children: Snippet;
	}
	let { label, children }: Props = $props();

	const labelText = $derived(
		/[.:!?]$/.test(label) ? label : `${label}:`,
	);
</script>

<div class="form-label">
	<span class="form-label-text">{labelText}</span>
	{@render children()}
</div>
