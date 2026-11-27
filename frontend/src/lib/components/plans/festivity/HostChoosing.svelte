<!-- Festivity/HostChoosing.svelte
  The host's extra-makes control — their spoils for hosting (one make) plus one
  per guest who marred or opted out. Host-only (the parent renders it only for
  the host, and only while makes remain); the host picks an option and spends
  one make at a time, not tied to any guest. Everyone else reads the host's
  progress off the scorecard ("Working the room — N of M"), not here.

  The whole make list is on offer, a duel included (book p.61 gives the host the
  make list with no exception): challenging someone spends the make, and the
  target may accept (a nested Propose Duel, the host as challenger) or decline.

  The draft (the option, the rumor, the new peer, the asset or target picked)
  lives in lib/festivityDraft.svelte.ts, not here: this card is unmounted while a
  duel is pending, and a guest's challenge or the nested duel it becomes would
  take a half-written make with it. A Make Demands perform-steps winner drives
  these makes for the host, so the header names the host to them (`amHost`).
-->
<script lang="ts">
	import { untrack } from 'svelte';
	import { hostMakeDraft as draft } from '$lib/festivityDraft.svelte';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import '$lib/components/shared/actionButton.css';
	import Chip from '$lib/components/shared/Chip.svelte';
	import { hostChoice, type Asset, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import AssetCreationForm from '../../AssetCreationForm.svelte';
	import FormField from '../FormField.svelte';
	import PlayerChips from '../PlayerChips.svelte';
	import DuelTermsFields from '../duel/DuelTermsFields.svelte';
	import { playerName } from '../shared';
	import {
		MAKE_OPTS, challengeReady, earnedHostMakes, centeredPeerCards, centeredPeerPick, isDraftCard,
		type FestRes,
	} from './options';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	let { plan, fest, players, assets, amHost, blockedByOtherRoll, activeRollerID, onPlansChanged }: {
		plan: Plan;
		fest: FestRes;
		players: Player[];
		assets: Asset[];
		/** Whether the viewer is the host; false for a perform-steps winner. */
		amHost: boolean;
		/** True while a guest's roll-and-choice is in progress: the server refuses a
		 *  make until it concludes, so the picker waits too. */
		blockedByOtherRoll: boolean;
		/** The guest whose roll-and-choice is in progress, for the "waiting" note. */
		activeRollerID: number | null;
		onPlansChanged: () => void;
	} = $props();

	const earned = $derived(earnedHostMakes(fest, plan.preparer_id));
	const taken = $derived(fest.hostMakesTaken.length);
	const remaining = $derived(earned - taken);

	// Anyone but the host can be challenged.
	const duelTargets = $derived(players.filter(p => p.id !== plan.preparer_id));

	// Drafts and real centered peers as one list — see options.ts.
	const centerPeerCandidates = $derived(centeredPeerCards(fest, assets, plan.game_id));

	// Begin the draft for this plan (a different plan starts fresh). Untracked:
	// begin() reads the draft's own key.
	$effect.pre(() => {
		const planID = plan.id;
		untrack(() => draft.begin(planID));
	});

	// The draft outlives a duel, which can change the centre under it: the peer picked
	// may have been taken by someone else. Drop a pick that is no longer on offer (the
	// server would refuse it anyway). Writes only when stale, so it settles at once.
	$effect(() => {
		const onOffer = centerPeerCandidates.map(a => a.id);
		untrack(() => {
			if (draft.assetID != null && !onOffer.includes(draft.assetID)) draft.assetID = null;
		});
	});

	let hostPickerBusy = $state(false);
	let hostPickerError = $state('');

	// Whether the picked make has its required sub-input filled in — mirrors the
	// per-choice checks in submitHostChoice so the button stays disabled (the
	// server's non-empty validation is the backstop, not the first line).
	const hostChoiceReady = $derived.by(() => {
		if (!draft.picked) return false;
		switch (draft.picked) {
			case 'spread_rumor': return draft.rumor.trim().length > 0;
			case 'introduce_peer': return draft.peerName.trim().length > 0 && draft.peerMarginalia.trim().length > 0;
			case 'take_center_peer': return draft.assetID != null;
			case 'challenge_duel': return challengeReady(draft.duelTargetID, draft.duelPlace);
			default: return true;
		}
	});

	async function submitHostChoice() {
		const hostPickedChoice = draft.picked;
		if (hostPickerBusy || !hostPickedChoice) return;
		hostPickerBusy = true; hostPickerError = '';
		try {
			const body: {
				choice: string; rumor_text?: string; peer_name?: string;
				peer_marginalia?: string[]; asset_id?: number; draft_id?: string;
				target_player_id?: number; duel_type?: string; preparation_notes?: string;
			} = { choice: hostPickedChoice };
			if (draft.picked === 'challenge_duel') {
				if (draft.duelTargetID == null) { hostPickerError = 'Pick someone to challenge.'; return; }
				if (!draft.duelPlace.trim()) { hostPickerError = 'Describe the location of the duel.'; return; }
				body.target_player_id = draft.duelTargetID;
				body.duel_type = draft.duelType;
				body.preparation_notes = draft.duelPlace.trim();
			}
			if (draft.picked === 'spread_rumor') {
				if (!draft.rumor.trim()) { hostPickerError = 'Enter the rumor.'; return; }
				body.rumor_text = draft.rumor.trim();
			}
			if (draft.picked === 'introduce_peer') {
				if (!draft.peerName.trim() || !draft.peerMarginalia.trim()) {
					hostPickerError = 'Name the new peer, with one marginalia.';
					return;
				}
				body.peer_name = draft.peerName.trim();
				body.peer_marginalia = [draft.peerMarginalia.trim()];
			}
			if (draft.picked === 'take_center_peer') {
				if (draft.assetID == null) { hostPickerError = 'Pick a centered peer.'; return; }
				// A draft peer travels as draft_id; a real one as asset_id.
				const pick = centeredPeerPick(fest, draft.assetID);
				if (pick == null) { hostPickerError = 'That peer is no longer available.'; return; }
				Object.assign(body, pick);
			}
			await hostChoice(plan.id, body);
			draft.clear();
			onPlansChanged();
		} catch (e) {
			hostPickerError = e instanceof Error ? e.message : 'Could not take the make.';
		} finally { hostPickerBusy = false; }
	}
</script>

<!-- "Your" to the host; their name, in their colour, with 's to a stand-in. A snippet so
     the space after the call survives. -->
{#snippet owner()}{#if amHost}Your{:else}<PlayerName {players} id={plan.preparer_id} />'s{/if}{/snippet}

<div class="choices-section">
	<p class="choices-header">{@render owner()} extra Makes</p>
	<p class="choices-note">{taken} of {earned} taken.</p>
	{#if remaining > 0 && blockedByOtherRoll}
		<p class="choices-note muted">
			Waiting for <PlayerName {players} id={activeRollerID} /> to finish their roll…
		</p>
	{:else if remaining > 0}
		<FormField label="Take a Make — pick one">
			<div class="chip-row">
				{#each MAKE_OPTS as o}
					<Chip
						selected={draft.picked === o.key}
						onclick={() => {
							draft.picked = draft.picked === o.key ? null : o.key;
							draft.assetID = null;
							draft.duelTargetID = null;
						}}
					>{o.label}</Chip>
				{/each}
			</div>
		</FormField>
		{#if draft.picked === 'spread_rumor'}
			<label class="form-label">
				Rumor text
				<textarea rows={2} bind:value={draft.rumor} class="form-textarea" maxlength={TEXT_LIMITS.LONG_TEXT}></textarea>
			</label>
		{:else if draft.picked === 'introduce_peer'}
			<AssetCreationForm
				gameID={plan.game_id}
				assetType="peer"
				bind:name={draft.peerName}
				bind:marginalia={draft.peerMarginalia}
				disabled={hostPickerBusy}
			/>
		{:else if draft.picked === 'take_center_peer'}
			<CardPicker
				label="Peer to take from the center"
				items={centerPeerCandidates}
				{players}
				emptyMessage="No peers in the center."
				ownerLabel={(a) => isDraftCard(a)
					? 'New to court — in nobody\'s retinue'
					: `Owned by ${playerName(players, a.owner_id)}`}
				selected={draft.assetID}
				onSelect={(id) => (draft.assetID = id)}
			/>
		{:else if draft.picked === 'challenge_duel'}
			<FormField label="Challenge">
				<PlayerChips
					players={duelTargets}
					isActive={(p) => draft.duelTargetID === p.id}
					onSelect={(p) => (draft.duelTargetID = draft.duelTargetID === p.id ? null : p.id)}
				/>
				{#if draft.duelTargetID != null && fest.acceptDuels.includes(draft.duelTargetID)}
					<p class="choices-note muted">
						This challenge must be accepted.
					</p>
				{/if}
			</FormField>
			<DuelTermsFields bind:duelType={draft.duelType} bind:place={draft.duelPlace} />
		{/if}
		{#if hostPickerError}<ErrorText message={hostPickerError} variant="panel" />{/if}
		<button class="action-btn primary"
			onclick={submitHostChoice}
			disabled={hostPickerBusy || !hostChoiceReady}>
			{hostPickerBusy ? '…' : `Take this Make (${remaining} left)`}
		</button>
	{/if}
</div>
