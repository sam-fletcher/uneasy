<!-- Festivity/HostChoosing.svelte
  The host's extra-makes control — their spoils for hosting (one make) plus one
  per guest who marred or opted out. Host-only (the parent renders it only for
  the host, and only while makes remain); the host picks an option and spends
  one make at a time, not tied to any guest. Everyone else reads the host's
  progress off the scorecard ("Working the room — N of M"), not here.
-->
<script lang="ts">
	import { hostChoice, getAssetSuggestions, type Asset, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import SuggestionPicker from '../../SuggestionPicker.svelte';
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
	let hostAssetID = $state<number | null>(null);
	let hostPickerBusy = $state(false);
	let hostPickerError = $state('');

	// Peer-name suggestions, fetched lazily the first time introduce_peer is
	// chosen (re-fetched per plan).
	let peerNameSuggestions = $state<string[]>([]);
	let peerNameSuggLoading = $state(false);
	let peerNameSuggFetched = false;

	// Reset picker when plan changes.
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			hostPickedChoice = null;
			hostRumor = '';
			hostPeerName = '';
			hostAssetID = null;
			hostPickerError = '';
			peerNameSuggestions = [];
			peerNameSuggFetched = false;
		}
	});

	$effect(() => {
		if (hostPickedChoice !== 'introduce_peer' || peerNameSuggFetched) return;
		peerNameSuggFetched = true;
		peerNameSuggLoading = true;
		getAssetSuggestions(plan.game_id, 'peer', 'name')
			.then(res => { peerNameSuggestions = res.suggestions; })
			.catch(() => { peerNameSuggestions = []; })
			.finally(() => { peerNameSuggLoading = false; });
	});

	async function submitHostChoice() {
		if (hostPickerBusy || !hostPickedChoice) return;
		hostPickerBusy = true; hostPickerError = '';
		try {
			const body: { choice: string; rumor_text?: string; peer_name?: string; asset_id?: number } = {
				choice: hostPickedChoice,
			};
			if (hostPickedChoice === 'spread_rumor') body.rumor_text = hostRumor.trim();
			if (hostPickedChoice === 'introduce_peer') body.peer_name = hostPeerName.trim() || 'New peer';
			if (hostPickedChoice === 'take_center_peer') {
				if (hostAssetID == null) { hostPickerError = 'Pick a centered peer.'; return; }
				body.asset_id = hostAssetID;
			}
			await hostChoice(plan.id, body);
			hostPickedChoice = null;
			hostRumor = '';
			hostPeerName = '';
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
				<textarea rows={2} bind:value={hostRumor} class="form-textarea"></textarea>
			</label>
		{:else if hostPickedChoice === 'introduce_peer'}
			<div class="form-label">
				<span>New peer's name:</span>
				<SuggestionPicker
					suggestions={peerNameSuggestions}
					bind:value={hostPeerName}
					loading={peerNameSuggLoading}
					customPlaceholder="Name of the new peer"
					maxlength={120}
				/>
			</div>
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
			disabled={hostPickerBusy || !hostPickedChoice}>
			{hostPickerBusy ? '…' : `Take this Make (${remaining} left)`}
		</button>
	{/if}
</div>
