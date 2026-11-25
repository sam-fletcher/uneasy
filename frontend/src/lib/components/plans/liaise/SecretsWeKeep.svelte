<!-- Liaise/SecretsWeKeep.svelte
  Phase 2: Secrets We Keep. Each seat picks one of its own assets to hold the secret of
  the meeting and writes the meeting on its underside.

  THE MEETING'S TEXT IS A SECRET. `keepText` lives only in this component and in the
  keep-secret request. It must never be passed to ctx.emitPrepDraft or any other draft
  — a draft is relayed to every player as it is typed, and a watcher may see that a
  player is writing, never the words (DECISIONS, Architecture: a live draft never carries
  text that could become secret). The box's reset effect keys on a derived STRING and
  never on the plan object: a refetch of the plan must not wipe what is being typed
  (clandestinely_liaise_secret_text.spec.ts pins it).
-->
<script lang="ts">
	import Quote from '$lib/components/shared/Quote.svelte';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import { keepSecret, type Plan } from '$lib/api';
	import { liaiseSecretHeader } from '$lib/plans/liaiseSecret';
	import EndsNote from '../shared/EndsNote.svelte';
	import Buffet from '../shared/Buffet.svelte';
	import { LIAISE_BUFFET } from '$lib/plans/buffets';
	import CardPicker from '../CardPicker.svelte';
	import { playerName, ownerIntactAssets } from '../shared';
	import type { PlanContext } from '../types';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import { clStateOf, type Seats } from './shared';

	let { ctx, plan, seats, error = $bindable('') }: {
		ctx: PlanContext;
		plan: Plan;
		seats: Seats;
		/** The panel's error line (ResolvingCard draws it). */
		error?: string;
	} = $props();

	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const onPlansChanged = $derived(ctx.onPlansChanged);

	const clState = $derived(clStateOf(plan));
	/** The first of the viewer's seats that hasn't kept a secret yet. */
	const activeKeepSeat = $derived(seats.activeKeepSeat);

	// Keep-secret picker: the rule is "one of your assets" (THE_12_PLANS_RULES
	// §Secrets We Keep) — writing a secret on an asset's underside, which the
	// backend allows for ANY owned asset regardless of leverage. So this list
	// must NOT exclude leveraged assets (a stricter filter wrongly hid the main
	// character, which is the asset most often leveraged through play). Only
	// destroyed assets are excluded — you can't write on a torn-up asset.
	//
	// Keyed by SEAT: a stand-in nominates one of the PREPARER's assets.
	const myKeepSecretAssets = $derived(ownerIntactAssets(assets, activeKeepSeat));

	let keepSecretAssetID = $state<number | null>(null);
	let keepBusy = $state(false);
	// The meeting, as the player writes it for the underside of the asset. This is a
	// SECRET: see the note at the top of this file.
	let keepText = $state('');
	// The box starts EMPTY for both seats. The preparer's preparation note sketched this
	// meeting, but it is public (the plan card and the log carry it), so prefilling it
	// would let one tap keep a "secret" the whole table has already read. It is shown
	// just above the box as a reminder instead, and the secret is something written
	// fresh. The box starts over when the seat or plan changes (a viewer holding both
	// seats writes two meetings, one after the other) and only then: a refetch of the
	// plan must not wipe what is being typed, so this depends on the key, never on the
	// plan object itself.
	const keepStartKey = $derived(
		activeKeepSeat == null ? null : `${plan.id}:${activeKeepSeat}`,
	);
	$effect(() => {
		void keepStartKey;
		keepText = '';
	});
	// The line the server puts in front of what is written, so the hint can promise it:
	// the OTHER seat's player and the peer they brought to the meeting.
	const keepOtherPlayerID = $derived(
		activeKeepSeat == null ? null
			: activeKeepSeat === plan.preparer_id ? clState.partnerID : plan.preparer_id,
	);
	const keepOtherPeerID = $derived(
		activeKeepSeat == null ? null
			: activeKeepSeat === plan.preparer_id ? clState.partnerPeerID : clState.preparerPeerID,
	);
	const keepHeader = $derived(
		keepOtherPlayerID == null ? '' : liaiseSecretHeader(
			playerName(players, keepOtherPlayerID),
			assets.find(a => a.id === keepOtherPeerID)?.name,
		),
	);
	const keepSubmittable = $derived(keepSecretAssetID != null && keepText.trim().length > 0);
	async function onKeepSecret(p: Plan) {
		if (!keepSubmittable || keepSecretAssetID == null || keepBusy) return;
		keepBusy = true; error = '';
		try {
			await keepSecret(p.id, keepSecretAssetID, keepText.trim());
			keepSecretAssetID = null;
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not commit secret.';
		} finally { keepBusy = false; }
	}

	// Reset the pick on a change of plan — and of ACTIVE seat, for a viewer holding both:
	// the next seat's form must not inherit the last seat's half-made pick (it names the
	// other seat's asset).
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			keepSecretAssetID = null;
		}
	});
	$effect(() => {
		void activeKeepSeat;
		keepSecretAssetID = null;
	});
</script>

<div class="choices-section">
	<p class="ft-prompt">
		Each of you picks one of your own assets to hold the secret of this
		meeting, and writes the meeting on its underside.
	</p>
	{#if seats.amLockedOut}
		<!-- A demand handed this seat to someone else; say so rather than
		     offering a form the server would refuse. -->
		<p class="choices-note">
			<PlayerName {players} id={seats.preparerSeatHolder} /> is nominating on your behalf.
		</p>
	{:else if seats.iKeptSecret}
		<!-- Who is still owed is the step heading's who-line to say. -->
		<p class="choices-note">
			{seats.participants.some(p => !seats.keepSecretSubmittedIDs.has(p.player_id))
				? "You've committed your secret."
				: 'Both secrets are committed.'}
		</p>
	{:else}
		{#if seats.actingForPreparerSeat}
			<p class="ft-prompt">
				You hold <PlayerName {players} id={plan.preparer_id} />'s side of this meeting —
				pick one of <PlayerName {players} id={plan.preparer_id} />'s assets.
			</p>
		{/if}
		<CardPicker
			label="Assets that can hold it"
			items={myKeepSecretAssets}
			{players}
			emptyMessage="{seats.actingForPreparerSeat ? playerName(players, plan.preparer_id) + ' has' : 'You have'} no assets to bear the secret."
			selected={keepSecretAssetID}
			onSelect={(id) => (keepSecretAssetID = id)}
		/>
		{#if myKeepSecretAssets.length > 0}
			<!-- The preparer's note, as a reminder: it is public, so it is not the
			     box's starting text. The partner wrote no note and sees no reminder. -->
			{#if activeKeepSeat === plan.preparer_id && plan.preparation_notes}
				<p class="choices-note muted">
					{seats.actingForPreparerSeat ? `${playerName(players, plan.preparer_id)}'s` : 'Your'}
					preparation note: <Quote text={plan.preparation_notes} />
				</p>
			{/if}
			<label class="form-label">
				<span class="form-label-text">The meeting</span>
				<textarea
					rows={4}
					class="form-textarea"
					maxlength={TEXT_LIMITS.NARRATIVE}
					placeholder="What happened when you met? Write it as you would on the underside."
					bind:value={keepText}
				></textarea>
			</label>
			<p class="choices-note muted">
				“{keepHeader} —” goes in front of what you write, so you needn't say who
				it was with.
			</p>
			<p class="choices-note muted">
				{seats.actingForPreparerSeat
					? `Only ${playerName(players, plan.preparer_id)} can read it`
					: 'Only you can read it'},
				unless someone looks at, takes or breaks this asset.
				The table sees which asset holds a secret, never what it says.
			</p>
			<button class="action-btn primary"
				onclick={() => onKeepSecret(plan)}
				disabled={keepBusy || !keepSubmittable}>
				{keepBusy ? '…' : 'Keep this secret'}
			</button>
			<EndsNote text="When you have both kept a secret, Things We Share begins." />
		{/if}
	{/if}
</div>
<Buffet tabs={LIAISE_BUFFET} />
