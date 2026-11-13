<!-- RowPill.svelte
  A row number, said in words as plain faint text: "4 rows later" on a plan
  card, "Row 6" where a row is named. Not a pill any more: a bordered or
  filled badge is a chip's shape, and nothing here can be pressed, so it is
  text. (The file keeps its name for the importers.)

    kind="delay"  a plan's static delay: "N rows later", the rows after the
                  row it was prepared on it resolves.
    kind="row"    an absolute row: "Row N".
-->
<script lang="ts">
	interface Props {
		row: number;
		kind?: 'row' | 'delay';
	}
	let { row, kind = 'row' }: Props = $props();
	const text = $derived(
		kind === 'delay' ? `${row} row${row === 1 ? '' : 's'} later` : `Row ${row}`,
	);
</script>

<span class="row-pill">{text}</span>

<style>
	.row-pill {
		font-size: var(--text-label);
		color: var(--color-text-faint);
		white-space: nowrap;
	}
</style>
