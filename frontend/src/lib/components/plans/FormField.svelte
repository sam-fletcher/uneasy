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

  A label is a label, not a heading, and carries no trailing punctuation: it names
  what the field takes ("Rows until you meet again, or 0 to part ways") and never
  repeats the sentence above it (Round 6, "Inside a step"). FormField used to append
  a colon; it prints the label as written.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		label: string;
		children: Snippet;
	}
	let { label, children }: Props = $props();

</script>

<div class="form-label">
	<span class="form-label-text">{label}</span>
	{@render children()}
</div>
