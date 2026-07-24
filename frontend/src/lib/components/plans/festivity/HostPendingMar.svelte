<!-- Festivity/HostPendingMar.svelte
  Renders for the host when a guest has insisted a mar that the host must resolve
  themselves — a decision about the host's OWN assets the insisting guest can't
  make for them:
    • break_self   → which marginalia on the host's main character to tear;
    • disagreement → which of the host's peers falls out (goes to the center).
  Other mars (rumor_about_you, accept_duels) apply immediately at insist time and
  never reach here. Pending mars are settled one at a time (the first in queue).
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import { resolveHostMar, type Asset, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import { breakableAssets, playerName } from '../shared';
	import { destructionWarning } from '$lib/assetRisk';

	let { plan, players, assets, pendingHostMars, onPlansChanged }: {
		plan: Plan;
		players: Player[];
		assets: Asset[];
		pendingHostMars: string[];
		onPlansChanged: () => void;
	} = $props();

	// Settle the oldest pending mar first.
	const current = $derived(pendingHostMars[0] ?? null);

	// break_self target: the host's main character, if a break can land on it —
	// an intact marginalia to tear, or a blank MC the break destroys outright.
	const hostMC = $derived(
		breakableAssets(assets.filter(a => a.owner_id === plan.preparer_id && a.is_main_character)),
	);
	const canBreak = $derived(hostMC.length > 0);
	const breakWarn = $derived(destructionWarning(hostMC[0]));

	// disagreement target: one of the host's own peers.
	const hostPeers = $derived(
		assets.filter(a =>
			a.owner_id === plan.preparer_id && a.asset_type === 'peer' && !a.is_destroyed),
	);

	let marginaliaID = $state<number | null>(null);
	let assetID = $state<number | null>(null);
	let busy = $state(false);
	let error = $state('');

	// Reset selections when the option being resolved changes.
	let lastCurrent = $state<string | null>(null);
	$effect(() => {
		if (current !== lastCurrent) {
			lastCurrent = current;
			marginaliaID = null;
			assetID = null;
			error = '';
		}
	});

	async function submit() {
		if (busy || !current) return;
		if (current === 'break_self' && canBreak && marginaliaID == null) {
			error = 'Pick what to tear.'; return;
		}
		if (current === 'disagreement' && assetID == null) {
			error = 'Pick a peer.'; return;
		}
		busy = true; error = '';
		try {
			const body: { mar_option: string; marginalia_id?: number; asset_id?: number } = {
				mar_option: current,
			};
			// For break_self with nothing left to tear, the server settles it as a
			// no-op; the placeholder id is ignored in that case.
			if (current === 'break_self') body.marginalia_id = marginaliaID ?? 0;
			if (current === 'disagreement') body.asset_id = assetID!;
			await resolveHostMar(plan.id, body);
			marginaliaID = null;
			assetID = null;
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not resolve the mar.';
		} finally { busy = false; }
	}
</script>

{#if current}
	<div class="choices-section">
		<p class="choices-header">
			{current === 'break_self' ? 'Break yourself' : 'A peer falls out with you'}
			{#if pendingHostMars.length > 1}<span class="muted" style="font-weight:400;"> · {pendingHostMars.length} mars to settle</span>{/if}
		</p>

		{#if current === 'break_self'}
			<p class="choices-note muted">
				A guest has caused you to break yourself. Choose a marginalia on your main
				character to tear.
			</p>
			{#if canBreak}
				<CardPicker
					label="Marginalium to tear"
					items={hostMC}
					{players}
					emptyMessage="You have nothing left to break."
					marginaliaMode
					selectedMarginaliaID={marginaliaID}
					selectedAssetID={hostMC[0]?.id ?? null}
					onSelectMarginalia={(mID) => (marginaliaID = mID)}
				/>
				{#if breakWarn}<p class="res-warning">{breakWarn}</p>{/if}
			{:else}
				<p class="choices-note muted">You have nothing left to tear — you can pass the break.</p>
			{/if}
			{#if error}<p class="res-error">{error}</p>{/if}
			<button class="action-btn primary" onclick={submit} disabled={busy}>
				{busy ? '…' : canBreak ? 'Tear it' : 'Pass the break'}
			</button>
		{:else}
			<p class="choices-note muted">
				A guest has caused you to fall out with one of your peers. Choose which peer
				is considering changing retinue.
			</p>
			<CardPicker
				label="Peer to set in the center"
				items={hostPeers}
				{players}
				emptyMessage="You have no peers to set in the center."
				ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
				selected={assetID}
				onSelect={(id) => (assetID = id)}
			/>
			{#if error}<p class="res-error">{error}</p>{/if}
			<button class="action-btn primary" onclick={submit} disabled={busy || assetID == null}>
				{busy ? '…' : 'Set them in the center'}
			</button>
		{/if}
	</div>
{/if}
