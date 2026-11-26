<!-- Festivity/HostPendingMar.svelte
  The host's pending-mar card. A guest with a make has insisted the host take a
  Mar (book p.61: "insist that the host choose one option from the mar list"),
  and it is the HOST who chooses which — so this card is where the choice is
  made, plus whatever that option needs:
    • rumor_about_you → the rumor's text;
    • disagreement    → which of the host's peers falls out;
    • accept_duels    → nothing;
    • break_self      → which marginalia on the host's main character to tear.
  Mars are settled one at a time, oldest insistence first.

  While a guest's roll-and-choice is open, the server refuses the settlement, so
  the card keeps its place and names the wait in place of its picker, as the
  host's Make picker does (HostChoosing). The picker's draft survives the wait,
  and a duel challenge that takes the card off the page: it lives in
  lib/festivityDraft.svelte.ts, not here, because this component is unmounted
  whenever a duel is pending.

  The mar lands on the HOST. When a Make Demands perform-steps winner drives the
  host's steps the card is theirs, so its copy names the host wherever it would
  say "you" to the host (`amHost` false); the host's own wording is unchanged.
-->
<script lang="ts">
	import { untrack } from 'svelte';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import { hostMarDraft as draft } from '$lib/festivityDraft.svelte';
	import '$lib/components/shared/actionButton.css';
	import Notice from '$lib/components/shared/Notice.svelte';
	import OptionCard from '$lib/components/shared/OptionCard.svelte';
	import { resolveHostMar, type Asset, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import { breakableAssets, playerName } from '../shared';
	import { destructionWarning } from '$lib/assetRisk';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import { MAR_OPTS, standInMarOpts } from './options';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	let { plan, players, assets, amHost, unchosenHostMars, blockedByOtherRoll, activeRollerID, onPlansChanged }: {
		plan: Plan;
		players: Player[];
		assets: Asset[];
		/** Whether the viewer is the host. False for a Make Demands perform-steps
		 *  winner, who answers the mar for the host: the copy then names the host. */
		amHost: boolean;
		/** Guests whose insistence the host hasn't answered yet, oldest first. */
		unchosenHostMars: number[];
		/** True while a guest's roll-and-choice is in progress: the server refuses a
		 *  settlement until it concludes, so the picker waits too. */
		blockedByOtherRoll: boolean;
		/** The guest whose roll-and-choice is in progress, for the "waiting" note. */
		activeRollerID: number | null;
		onPlansChanged: () => void;
	} = $props();

	// Whose insistence the card is answering: the oldest still unanswered.
	const insisterID = $derived(unchosenHostMars[0] ?? null);
	const total = $derived(unchosenHostMars.length);

	// The host, for copy that would say "you" to the host and must not say it to a stand-in.
	const hostName = $derived(playerName(players, plan.preparer_id));
	// "you" for the host, the host named for a stand-in.
	const them = $derived(amHost ? 'you' : hostName);
	const marOpts = $derived(amHost ? MAR_OPTS : standInMarOpts(hostName));

	// The draft lives outside this component (see the header); begin it for this
	// mar. Untracked: begin() reads the draft's own key, and an effect that reads
	// and writes the same state would re-trigger itself.
	$effect.pre(() => {
		const planID = plan.id;
		const insister = insisterID;
		untrack(() => draft.begin(planID, insister));
	});

	// break_self target: the host's main character, if a break can land on it —
	// an intact marginalia to tear, or a blank MC the break destroys outright.
	const hostMC = $derived(
		breakableAssets(assets.filter(a => a.owner_id === plan.preparer_id && a.is_main_character)),
	);
	const canBreak = $derived(hostMC.length > 0);
	const breakWarn = $derived(destructionWarning(hostMC[0]));

	// disagreement target: one of the host's own peers — never the main character,
	// who is the one falling out with them.
	const hostPeers = $derived(
		assets.filter(a =>
			a.owner_id === plan.preparer_id && a.asset_type === 'peer'
			&& !a.is_main_character && !a.is_destroyed),
	);
	// An option the host can't carry out is shown but locked, with the reason.
	const lockReason = (key: string): string | undefined =>
		key === 'disagreement' && hostPeers.length === 0
			? (amHost ? 'You have no peers' : `${hostName} has no peers`)
			: undefined;

	let busy = $state(false);
	let error = $state('');

	// Mirrors the checks in submit() so the button stays disabled until the chosen
	// option has what it needs (the server validates again).
	const ready = $derived.by(() => {
		switch (draft.picked) {
			case 'rumor_about_you': return draft.rumor.trim().length > 0;
			case 'disagreement': return draft.assetID != null;
			case 'break_self': return !canBreak || draft.marginaliaID != null;
			case 'accept_duels': return true;
			default: return false;
		}
	});

	async function submit() {
		const picked = draft.picked;
		if (busy || !picked || !ready || insisterID == null) return;
		busy = true; error = '';
		try {
			const body: Parameters<typeof resolveHostMar>[1] = {
				mar_option: picked,
				insister_id: insisterID,
			};
			if (picked === 'rumor_about_you') body.rumor_text = draft.rumor.trim();
			// For break_self with nothing left to tear, the server settles it as a
			// no-op; the placeholder id is ignored in that case.
			if (picked === 'break_self') body.marginalia_id = draft.marginaliaID ?? 0;
			if (picked === 'disagreement') body.asset_id = draft.assetID!;
			await resolveHostMar(plan.id, body);
			draft.clear();
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not settle the mar.';
		} finally { busy = false; }
	}
</script>

{#if total > 0}
	<div class="choices-section">
		<p class="choices-header">{amHost ? 'Your Mar' : `${hostName}'s Mar`}</p>
		<p class="ft-prompt">
			<PlayerName {players} id={insisterID} /> insists {them} take{amHost ? '' : 's'} a Mar.
			{#if total > 1}{total} mars to settle.{/if}
		</p>

		{#if blockedByOtherRoll}
			<p class="choices-note muted">
				Waiting for <PlayerName {players} id={activeRollerID} /> to finish their roll…
			</p>
		{:else}
			<p class="choices-note muted">
				{amHost ? 'You choose which one' : `You choose for ${hostName}`} — pick one from the Mar list.
			</p>
			<div class="option-list">
				{#each marOpts as o (o.key)}
					{@const reason = lockReason(o.key)}
					<OptionCard
						title={o.label}
						description={o.desc || undefined}
						{reason}
						selected={draft.picked === o.key}
						disabled={reason != null}
						onclick={() => { draft.picked = draft.picked === o.key ? null : o.key; }}
					/>
				{/each}
			</div>

			{#if draft.picked === 'rumor_about_you'}
				<label class="form-label">
					The rumor about {them}
					<textarea rows={2} bind:value={draft.rumor} class="form-textarea"
						maxlength={TEXT_LIMITS.LONG_TEXT} placeholder="What does the rumor say?"></textarea>
				</label>
			{:else if draft.picked === 'accept_duels'}
				<p class="choices-note muted">
					{amHost ? 'You' : hostName} must accept any duel challenge for the rest of the event.
				</p>
			{:else if draft.picked === 'break_self'}
				{#if canBreak}
					<CardPicker
						label="Marginalia to tear"
						items={hostMC}
						{players}
						emptyMessage="{amHost ? 'You have' : `${hostName} has`} nothing left to break."
						marginaliaMode
						selectedMarginaliaID={draft.marginaliaID}
						selectedAssetID={hostMC[0]?.id ?? null}
						onSelectMarginalia={(mID) => (draft.marginaliaID = mID)}
					/>
					{#if breakWarn}<Notice tone="warning">{breakWarn}</Notice>{/if}
				{:else}
					<p class="choices-note muted">
						{amHost ? 'You have' : `${hostName} has`} nothing left to tear — pass the break.
					</p>
				{/if}
			{:else if draft.picked === 'disagreement'}
				<CardPicker
					label="Peer to set in the center"
					items={hostPeers}
					{players}
					emptyMessage="{amHost ? 'You have' : `${hostName} has`} no peers to set in the center."
					ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
					selected={draft.assetID}
					onSelect={(id) => (draft.assetID = id)}
				/>
			{/if}

			{#if error}<ErrorText message={error} variant="panel" />{/if}
			{#if draft.picked}
				<button class="action-btn primary" onclick={submit} disabled={busy || !ready}>
					{busy ? '…'
					: draft.picked === 'break_self' && !canBreak ? 'Pass the break'
					: 'Take this Mar'}
				</button>
			{/if}
		{/if}
	</div>
{/if}
