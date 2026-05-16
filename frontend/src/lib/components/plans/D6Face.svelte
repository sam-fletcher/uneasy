<!-- D6Face.svelte
  Single die-face icon (values 0–6). Used wherever the user is picking or
  reading a die face — e.g. SimultaneousRevealInput, anywhere we display
  a chosen face for a reveal. Callers wrap this inside their own button
  (`.chip-btn`) for selection.

  The 0 face renders blank (no pips) for flows that allow "no submit"
  (currently: liaise cancel).
-->
<script lang="ts">
	interface Props {
		value: number;          // 0–6
		size?: number;          // px; default 24
		ariaLabel?: string;
	}
	let { value, size = 24, ariaLabel }: Props = $props();

	// Pip positions on a 20×20 viewbox (cx, cy).
	const TL: [number, number] = [6.5,  6.5];
	const TR: [number, number] = [13.5, 6.5];
	const ML: [number, number] = [6.5,  10];
	const MM: [number, number] = [10,   10];
	const MR: [number, number] = [13.5, 10];
	const BL: [number, number] = [6.5,  13.5];
	const BR: [number, number] = [13.5, 13.5];

	const PIPS: Record<number, [number, number][]> = {
		0: [],
		1: [MM],
		2: [TL, BR],
		3: [TL, MM, BR],
		4: [TL, TR, BL, BR],
		5: [TL, TR, MM, BL, BR],
		6: [TL, TR, ML, MR, BL, BR],
	};
	const pips = $derived(PIPS[value] ?? []);
</script>

<svg
	viewBox="0 0 20 20"
	width={size}
	height={size}
	role="img"
	aria-label={ariaLabel ?? `Die face ${value}`}
	class="d6-face"
>
	<rect x="2.5" y="2.5" width="15" height="15" rx="2.5" ry="2.5"
		fill="none" stroke="currentColor" stroke-width="1.2" />
	{#each pips as [cx, cy]}
		<circle {cx} {cy} r="1.3" fill="currentColor" />
	{/each}
</svg>

<style>
	.d6-face {
		display: inline-block;
		vertical-align: middle;
	}
</style>
