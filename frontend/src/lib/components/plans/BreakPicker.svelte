<!-- BreakPicker.svelte
  A marginalia picker for a break, with the heads-up every break owes: tearing an
  asset's last intact marginalia destroys it (assetRisk's destructionWarning, shown
  once something is picked).

  The parent owns which marginalia is picked and passes it in, so the warning is a
  function of what is on screen and can be rendered in a test. The asset the note
  belongs to is found from the note, so the parent doesn't track it separately.
-->
<script lang="ts">
	import type { Asset, Player } from '$lib/api';
	import { destructionWarning } from '$lib/assetRisk';
	import Notice from '$lib/components/shared/Notice.svelte';
	import CardPicker from './CardPicker.svelte';

	let { label, items, players, emptyMessage, selectedMarginaliaID, onSelect }: {
		label: string;
		/** Assets with an intact marginalia to tear (shared.ts:assetsWithIntactMarginalia). */
		items: Asset[];
		players: Player[];
		emptyMessage: string;
		selectedMarginaliaID: number | null;
		onSelect: (marginaliaID: number | null) => void;
	} = $props();

	const picked = $derived(
		selectedMarginaliaID == null
			? null
			: (items.find(a => (a.marginalia ?? []).some(m => m.id === selectedMarginaliaID)) ?? null),
	);
	const warning = $derived(destructionWarning(picked));
</script>

<CardPicker
	{label}
	{items}
	{players}
	{emptyMessage}
	marginaliaMode
	{selectedMarginaliaID}
	onSelectMarginalia={(mID) => onSelect(mID)}
/>
{#if warning}<Notice>{warning}</Notice>{/if}
