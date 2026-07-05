<!-- Festivity/HostChoosing.svelte
  The host's extra-makes control — their spoils for hosting (one make) plus one
  per guest who marred or opted out. Host-only (the parent renders it only for
  the host, and only while makes remain); the host picks an option and spends
  one make at a time, not tied to any guest. Everyone else reads the host's
  progress off the scorecard ("Working the room — N of M"), not here.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import { hostChoice, type Asset, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import AssetCreationForm from '../../AssetCreationForm.svelte';
	import FormField from '../FormField.svelte';
	import { playerName } from '../shared';
	import { HOST_MAKE_OPTS, earnedHostMakes, type FestRes } from './options';

	let { plan, fest, players, assets, onPlansChanged }: {
		plan: Plan;
		fest: FestRes;
		players: Player[];
		assets: Asset[];
		onPlansChanged: () => void;
	} = $props();

	const earned = $derived(earnedHostMakes(fest, plan.preparer_id));
	const taken = $derived(fest.hostMakesTaken.length);
	const remaining = $derived(earned - taken);

	const centerPeerCandidates = $derived(
		assets.filter(a => fest.centeredAssetIDs.includes(a.id) && !a.is_destroyed),
	);

	let hostPickedChoice = $state<string | null>(null);
	let hostRumor = $state('');
	let hostPeerName = $state('');
	let hostPeerMarginalia = $state('');
	let hostAssetID = $state<number | null>(null);
	let hostPickerBusy = $state(false);
	let hostPickerError = $state('');

	// Whether the picked make has its required sub-input filled in — mirrors the
	// per-choice checks in submitHostChoice so the button stays disabled (the
	// server's non-empty validation is the backstop, not the first line).
	const hostChoiceReady = $derived.by(() => {
		if (!hostPickedChoice) return false;
		switch (hostPickedChoice) {
			case 'spread_rumor': return hostRumor.trim().length > 0;
			case 'introduce_peer': return hostPeerName.trim().length > 0 && hostPeerMarginalia.trim().length > 0;
			case 'take_center_peer': return hostAssetID != null;
			default: return true;
		}
	});

	// Reset picker when plan changes.
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			hostPickedChoice = null;
			hostRumor = '';
			hostPeerName = '';
			hostPeerMarginalia = '';
			hostAssetID = null;
			hostPickerError = '';
		}
	});

	async function submitHostChoice() {
		if (hostPickerBusy || !hostPickedChoice) return;
		hostPickerBusy = true; hostPickerError = '';
		try {
			const body: {
				choice: string; rumor_text?: string; peer_name?: string;
				peer_marginalia?: string[]; asset_id?: number;
			} = { choice: hostPickedChoice };
			if (hostPickedChoice === 'spread_rumor') {
				if (!hostRumor.trim()) { hostPickerError = 'Enter the rumor.'; return; }
				body.rumor_text = hostRumor.trim();
			}
			if (hostPickedChoice === 'introduce_peer') {
				if (!hostPeerName.trim() || !hostPeerMarginalia.trim()) {
					hostPickerError = 'Name the new peer, with one marginalia.';
					return;
				}
				body.peer_name = hostPeerName.trim();
				body.peer_marginalia = [hostPeerMarginalia.trim()];
			}
			if (hostPickedChoice === 'take_center_peer') {
				if (hostAssetID == null) { hostPickerError = 'Pick a centered peer.'; return; }
				body.asset_id = hostAssetID;
			}
			await hostChoice(plan.id, body);
			hostPickedChoice = null;
			hostRumor = '';
			hostPeerName = '';
			hostPeerMarginalia = '';
			hostAssetID = null;
			onPlansChanged();
		} catch (e) {
			hostPickerError = e instanceof Error ? e.message : 'Could not take the make.';
		} finally { hostPickerBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="choices-header">
		Your extra Makes <span class="muted" style="font-weight:400;">· {taken} of {earned} taken</span>
	</p>
	{#if remaining > 0}
		<FormField label="Take a Make — pick one">
			<div class="chip-row">
				{#each HOST_MAKE_OPTS as o}
					<button
						type="button"
						class="chip-btn"
						class:active={hostPickedChoice === o.key}
						onclick={() => {
							hostPickedChoice = hostPickedChoice === o.key ? null : o.key;
							hostAssetID = null;
						}}
					>{o.label}</button>
				{/each}
			</div>
		</FormField>
		{#if hostPickedChoice === 'spread_rumor'}
			<label class="form-label">
				Rumor text:
				<textarea rows={2} bind:value={hostRumor} class="form-textarea" maxlength={5000}></textarea>
			</label>
		{:else if hostPickedChoice === 'introduce_peer'}
			<AssetCreationForm
				gameID={plan.game_id}
				assetType="peer"
				bind:name={hostPeerName}
				bind:marginalia={hostPeerMarginalia}
				disabled={hostPickerBusy}
			/>
		{:else if hostPickedChoice === 'take_center_peer'}
			<CardPicker
				label="Peer to take from the center"
				items={centerPeerCandidates}
				{players}
				emptyMessage="No peers in the center."
				ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
				selected={hostAssetID}
				onSelect={(id) => (hostAssetID = id)}
			/>
		{/if}
		{#if hostPickerError}<p class="res-error">{hostPickerError}</p>{/if}
		<button class="action-btn primary"
			onclick={submitHostChoice}
			disabled={hostPickerBusy || !hostChoiceReady}>
			{hostPickerBusy ? '…' : `Take this Make (${remaining} left)`}
		</button>
	{/if}
</div>
