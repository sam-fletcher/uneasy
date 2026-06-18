<!-- Festivity/HostChoosing.svelte
  The host's extra makes — their spoils for hosting (one make) plus one per
  guest who marred or opted out. Shown whenever the host still has makes to
  take; the host picks an option and spends one make at a time. Not tied to any
  guest. Non-hosts see a read-only count.
-->
<script lang="ts">
	import { hostChoice, getAssetSuggestions, type Asset, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import SuggestionPicker from '../../SuggestionPicker.svelte';
	import FormField from '../FormField.svelte';
	import { playerName } from '../shared';
	import { HOST_MAKE_OPTS, earnedHostMakes, type FestRes } from './options';

	let { plan, fest, players, assets, amHost, onPlansChanged }: {
		plan: Plan;
		fest: FestRes;
		players: Player[];
		assets: Asset[];
		amHost: boolean;
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
	<p class="choices-header">The host's extra Makes</p>
	<p class="choices-note muted">
		Taken {taken} of {earned} — one for hosting, plus one for each guest who
		rolled a Mar or opted out.
	</p>
	{#if remaining <= 0}
		<p class="choices-note">The host has taken all their extra Makes.</p>
	{:else if !amHost}
		<p class="choices-note muted">
			The host has {remaining} extra {remaining === 1 ? 'Make' : 'Makes'} left to take.
		</p>
	{:else}
		<p class="choices-note">
			Take one Make for yourself ({remaining} left):
		</p>
		<FormField label="Pick a make option">
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
			{hostPickerBusy ? '…' : 'Take this Make'}
		</button>
	{/if}

	{#if fest.hostMakesTaken.length > 0}
		<p class="choices-note muted" style="margin-top:0.5rem;">
			Taken so far: {fest.hostMakesTaken.join(', ')}
		</p>
	{/if}
</div>
