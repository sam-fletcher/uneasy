<!-- RowPill.svelte
  Small circular badge showing a row number. Mirrors the .rail-row visual in
  PublicRecord so the same shape appears wherever we reference a row (plan
  cards, target hints), reinforcing the association.

  Variants:
    size  — 'sm' (20px) for inline use in plan cards; 'md' (32px) matches
            the PublicRecord rail.
    state — 'past' | 'current' | 'future' (defaults to 'future'). Matches
            the rail's colour scheme so the same row reads the same way in
            both places.
    kind  — 'row' (default) shows the absolute row number; 'delay' shows a
            relative "N" badge for a plan's static row delay.
-->
<script lang="ts">
	interface Props {
		row: number;
		size?: 'sm' | 'md';
		state?: 'past' | 'current' | 'future';
		kind?: 'row' | 'delay';
	}
	let { row, size = 'sm', state = 'future', kind = 'row' }: Props = $props();
	const label = $derived(kind === 'delay' ? `Resolves ${row} rows later` : `Row ${row}`);
	const text = $derived(kind === 'delay' ? `${row}` : `${row}`);
</script>

<span class="row-pill" data-size={size} data-state={state} aria-label={label} title={label}>{text}</span>

<style>
	.row-pill {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border-radius: 50%;
		font-weight: 600;
		flex-shrink: 0;
		line-height: 1;
	}
	.row-pill[data-size="sm"] { width: 20px; height: 20px; font-size: 0.7rem; }
	.row-pill[data-size="md"] { width: 32px; height: 32px; font-size: 0.78rem; }

	.row-pill[data-state="past"]    { color: var(--color-text-faint); background: transparent; border: 1px solid var(--color-border); }
	.row-pill[data-state="current"] { color: var(--color-bg); background: var(--color-accent); box-shadow: 0 0 0 2px #e0c080; }
	.row-pill[data-state="future"]  { color: var(--color-text-muted); background: transparent; border: 1px solid var(--color-border-strong); }
</style>
