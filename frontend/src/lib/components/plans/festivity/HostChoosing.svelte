<!-- Festivity/HostChoosing.svelte
  Host's per-guest make-option picker. Active during the host_choosing
  phase; the host fills in a make option for every guest who rolled mar
  or opted out. Non-hosts see a "waiting on host" note.
-->
<script lang="ts">
	import { hostChoice, getAssetSuggestions, type Asset, type Plan, type Player } from '$lib/api';
	import PlayerChips from '../PlayerChips.svelte';
	import CardPicker from '../CardPicker.svelte';
	import SuggestionPicker from '../../SuggestionPicker.svelte';
	import FormField from '../FormField.svelte';
	import { playerName } from '../shared';
	import { HOST_MAKE_OPTS, type FestRes } from './options';

	let { plan, fest, players, assets, amHost, onPlansChanged }: {
		plan: Plan;
		fest: FestRes;
		players: Player[];
		assets: Asset[];
		amHost: boolean;
		onPlansChanged: () => void;
	} = $props();

	const pendingHostGuests = $derived(
		fest.guests.filter(id => {
			const k = String(id);
			const oc = fest.outcomes[k];
			if (oc !== 'mar' && oc !== 'opt_out') return false;
			return !(k in fest.hostChoices);
		}),
	);

	const centerPeerCandidates = $derived(
		assets.filter(a => fest.centeredAssetIDs.includes(a.id) && !a.is_destroyed),
	);

	let hostPickerGuestID = $state<number | null>(null);
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
			hostPickerGuestID = null;
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
		if (hostPickerBusy || hostPickerGuestID == null || !hostPickedChoice) return;
		hostPickerBusy = true; hostPickerError = '';
		try {
			const body: { target_player_id: number; choice: string; rumor_text?: string; peer_name?: string; asset_id?: number } = {
				target_player_id: hostPickerGuestID,
				choice: hostPickedChoice,
			};
			if (hostPickedChoice === 'spread_rumor') body.rumor_text = hostRumor.trim();
			if (hostPickedChoice === 'introduce_peer') body.peer_name = hostPeerName.trim() || 'New peer';
			if (hostPickedChoice === 'take_center_peer') {
				if (hostAssetID == null) { hostPickerError = 'Pick a centered peer.'; return; }
				body.asset_id = hostAssetID;
			}
			await hostChoice(plan.id, body);
			hostPickerGuestID = null;
			hostPickedChoice = null;
			hostRumor = '';
			hostPeerName = '';
			hostAssetID = null;
			onPlansChanged();
		} catch (e) {
			hostPickerError = e instanceof Error ? e.message : 'Could not submit host choice.';
		} finally { hostPickerBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="choices-header">The host's free makes</p>
	{#if pendingHostGuests.length === 0}
		<p class="choices-note">The host has taken all their free makes.</p>
	{:else if !amHost}
		<p class="choices-note muted">
			The host takes a make for themself, one for each of:
			{pendingHostGuests.map(id => playerName(players, id)).join(', ')}
		</p>
	{:else}
		<p class="choices-note">
			Take one make for yourself for each guest who rolled a mar or opted out.
		</p>
		{@const pendingGuestPlayers = pendingHostGuests
			.map(gid => players.find(p => p.id === gid))
			.filter((p): p is typeof players[number] => p != null)}
		<FormField label="Guest">
			<PlayerChips
				players={pendingGuestPlayers}
				isActive={(p) => hostPickerGuestID === p.id}
				onSelect={(p) => {
					hostPickerGuestID = hostPickerGuestID === p.id ? null : p.id;
					hostPickedChoice = null;
					hostAssetID = null;
				}}
			/>
			{#if hostPickerGuestID != null}
				<p class="choices-note muted" style="margin:0.25rem 0 0;">
					Outcome: {fest.outcomes[String(hostPickerGuestID)]}
				</p>
			{/if}
		</FormField>
		{#if hostPickerGuestID != null}
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
		{/if}
		{#if hostPickerError}<p class="res-error">{hostPickerError}</p>{/if}
		<button class="action-btn primary"
			onclick={submitHostChoice}
			disabled={hostPickerBusy || hostPickerGuestID == null || !hostPickedChoice}>
			{hostPickerBusy ? '…' : 'Submit pick'}
		</button>
	{/if}

	{#if Object.keys(fest.hostChoices).length > 0}
		<p class="choices-note muted" style="margin-top:0.5rem;">
			Done so far:
			{Object.entries(fest.hostChoices)
				.map(([pid, c]) => `${playerName(players, Number(pid))} → ${c}`)
				.join('; ')}
		</p>
	{/if}
</div>
