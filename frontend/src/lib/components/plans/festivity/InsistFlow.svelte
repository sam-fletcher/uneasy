<!-- Festivity/InsistFlow.svelte
  Renders when the current player holds an IOU from rolling make. They
  may force a single mar option on the host (rumor_about_you /
  disagreement / accept_duels / break_self).
-->
<script lang="ts">
	import { insistHostMar, type Asset, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import FormField from '../FormField.svelte';
	import { MAR_OPTS, type FestRes } from './options';

	let { plan, fest, players, assets, onPlansChanged }: {
		plan: Plan;
		fest: FestRes;
		players: Player[];
		assets: Asset[];
		onPlansChanged: () => void;
	} = $props();

	let insistOpen = $state(false);
	let insistChoice = $state<string | null>(null);
	let insistRumor = $state('');
	let insistAssetID = $state<number | null>(null);
	let insistBusy = $state(false);
	let insistError = $state('');

	async function submitInsist() {
		if (insistBusy || !insistChoice) return;
		insistBusy = true; insistError = '';
		try {
			const body: { mar_option: string; rumor_text?: string; asset_id?: number } = {
				mar_option: insistChoice,
			};
			if (insistChoice === 'rumor_about_you') body.rumor_text = insistRumor.trim();
			if (insistChoice === 'disagreement') {
				if (insistAssetID == null) { insistError = 'Pick a peer.'; return; }
				body.asset_id = insistAssetID;
			}
			await insistHostMar(plan.id, body);
			insistOpen = false;
			insistChoice = null;
			insistRumor = '';
			insistAssetID = null;
			onPlansChanged();
		} catch (e) {
			insistError = e instanceof Error ? e.message : 'Could not insist.';
		} finally { insistBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="choices-header">You hold an IOU</p>
	<p class="choices-note">
		As a guest who rolled make, you may force the host to take one mar option.
	</p>
	{#if !insistOpen}
		<button class="action-btn" onclick={() => (insistOpen = true)}>
			Insist on a mar
		</button>
	{:else}
		<FormField label="Force a mar option on the host">
			<div class="chip-row">
				{#each MAR_OPTS as o}
					<button
						type="button"
						class="chip-btn"
						class:active={insistChoice === o.key}
						onclick={() => {
							insistChoice = insistChoice === o.key ? null : o.key;
							insistAssetID = null;
						}}
					>{o.label}</button>
				{/each}
			</div>
		</FormField>
		{#if insistChoice === 'rumor_about_you'}
			<label class="form-label">
				Rumor text (about the host):
				<textarea rows={2} bind:value={insistRumor} class="form-textarea"></textarea>
			</label>
		{:else if insistChoice === 'disagreement'}
			{@const hostPeers = assets.filter(a =>
				a.owner_id === plan.preparer_id && a.asset_type === 'peer' && !a.is_destroyed)}
			<CardPicker
				label="Host peer to set in the center"
				items={hostPeers}
				{players}
				emptyMessage="The host has no peers to set in the center."
				selected={insistAssetID}
				onSelect={(id) => (insistAssetID = id)}
			/>
		{:else if insistChoice === 'break_self'}
			<p class="choices-note muted">
				The host tears a marginalia on their own main character (they choose
				how it's marked).
			</p>
		{/if}
		{#if insistError}<p class="res-error">{insistError}</p>{/if}
		<div class="form-row">
			<button class="action-btn primary"
				onclick={submitInsist}
				disabled={insistBusy || !insistChoice}>
				{insistBusy ? '…' : 'Insist'}
			</button>
			<button class="action-btn" onclick={() => { insistOpen = false; insistChoice = null; }}>
				Cancel
			</button>
		</div>
	{/if}
	{#if fest.hostMarInsists.length > 0}
		<p class="choices-note muted">
			Forced on host so far: {fest.hostMarInsists.join(', ')}
		</p>
	{/if}
</div>
