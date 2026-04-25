<!-- ClandestinelyLiaisePanel.svelte
  Prep + resolve UI for Clandestinely Liaise (Tier 2, Knowledge, variable delay).

  Unlike other plans, CL does not use a standard dice roll. Delay is set by a
  simultaneous reveal at prep time; resolution walks through four phases
  (together_at_last → secrets_we_keep → things_we_share →
  when_will_i_see_you_again → done).
-->
<script lang="ts">
	import './planPanel.css';
	import {
		preparePlan, completePlan,
		advanceLiaise, keepSecret, shareChoice,
		type Plan, type Asset, type Player,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import SimultaneousRevealInput from './SimultaneousRevealInput.svelte';
	import { playerName, parseResolutionData } from './shared';

	interface Props {
		mode: 'prep' | 'resolve' | 'delay-reveal';
		gameID: number;
		assets: Asset[];
		players: Player[];
		currentPlayerID: number | null;
		plan?: Plan | null;
		onPlansChanged?: () => void;
		onPlanPrepared?: () => void;
	}

	let {
		mode, gameID, assets, players, currentPlayerID,
		plan = null,
		onPlansChanged = () => {},
		onPlanPrepared = () => {},
	}: Props = $props();

	// ── Prep ─────────────────────────────────────────────────────────────────
	let clPartnerID = $state<number | null>(null);
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	const otherPlayers = $derived(players.filter(p => p.id !== currentPlayerID));

	async function submitPrep() {
		if (prepBusy) return;
		if (clPartnerID == null) { prepError = 'Pick a partner.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'clandestinely_liaise',
				target_player_id: clPartnerID,
				preparation_notes: prepNotes.trim() || null,
			});
			clPartnerID = null; prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	// ── Resolve state ────────────────────────────────────────────────────────
	type CLState = {
		phase: string;
		partnerID: number | null;
		delayRevealID: number | null;
		redelayRevealID: number | null;
		choices: string[];
	};
	const clState = $derived.by<CLState>(() => {
		const rd = parseResolutionData(plan);
		return {
			phase: rd.liaise_phase ?? '',
			partnerID: rd.partner_id ?? null,
			delayRevealID: rd.liaise_delay_reveal_id ?? null,
			redelayRevealID: rd.redelay_reveal_id ?? null,
			choices: rd.choices ?? [],
		};
	});

	const amPreparer = $derived(plan != null && currentPlayerID === plan.preparer_id);
	const amPartner = $derived(currentPlayerID != null && clState.partnerID === currentPlayerID);
	const amParticipant = $derived(amPreparer || amPartner);

	const participants = $derived.by(() => {
		if (!plan) return [];
		const out: { player_id: number; display_name: string }[] = [];
		out.push({
			player_id: plan.preparer_id,
			display_name: playerName(players, plan.preparer_id),
		});
		if (clState.partnerID != null) {
			out.push({
				player_id: clState.partnerID,
				display_name: playerName(players, clState.partnerID),
			});
		}
		return out;
	});

	const myUnleveragedAssets = $derived(
		currentPlayerID == null
			? []
			: assets.filter(a =>
				a.owner_id === currentPlayerID && !a.is_destroyed && !a.is_leveraged,
			),
	);
	// In share-choice sub-forms, the target is "the other participant".
	const otherParticipantID = $derived(
		amPreparer ? clState.partnerID : (plan?.preparer_id ?? null),
	);
	const otherParticipantAssets = $derived(
		otherParticipantID == null
			? []
			: assets.filter(a => a.owner_id === otherParticipantID && !a.is_destroyed),
	);

	// Has the current player submitted keep-secret?
	const iKeptSecret = $derived(
		currentPlayerID != null
		&& clState.choices.some(c => c.startsWith(`keep_secret:${currentPlayerID}:`)),
	);
	const keepSecretSubmittedIDs = $derived.by(() => {
		const ids = new Set<number>();
		for (const c of clState.choices) {
			const m = c.match(/^keep_secret:(\d+):/);
			if (m) ids.add(Number(m[1]));
		}
		return ids;
	});

	let resError = $state('');

	// ── Advance (preparer only) ──────────────────────────────────────────────
	let advanceBusy = $state(false);
	async function onAdvance(p: Plan) {
		if (advanceBusy) return;
		advanceBusy = true; resError = '';
		try {
			await advanceLiaise(p.id);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not advance phase.';
		} finally { advanceBusy = false; }
	}

	// ── Phase 2: keep secret ─────────────────────────────────────────────────
	let keepSecretAssetID = $state<number | null>(null);
	let keepBusy = $state(false);
	async function onKeepSecret(p: Plan) {
		if (keepSecretAssetID == null || keepBusy) return;
		keepBusy = true; resError = '';
		try {
			await keepSecret(p.id, keepSecretAssetID);
			keepSecretAssetID = null;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not commit secret.';
		} finally { keepBusy = false; }
	}

	// ── Phase 3: share choice ────────────────────────────────────────────────
	const SHARE_OPTIONS: { key: string; label: string; hint: string }[] = [
		{ key: 'look_at_secret',   label: "Look at partner's secrets",
			hint: "Pick one of their assets — you'll see its secrets." },
		{ key: 'update_peer',      label: 'Update your own peer',
			hint: 'Narrative only — edit the peer via its asset card later.' },
		{ key: 'break_peer',       label: 'Break your own peer',
			hint: 'Server tears one marginalia on your first intact peer.' },
		{ key: 'take_gift',        label: 'Take a gift from partner',
			hint: "Pick one of their non-peer assets — it transfers to you." },
		{ key: 'leverage_partner', label: "Leverage partner's asset, bank a die",
			hint: 'Pick one of their assets plus a die face (1–6) to bank.' },
	];
	const SHARE_NEEDS_ASSET = new Set(['look_at_secret', 'take_gift', 'leverage_partner']);
	const SHARE_NEEDS_FACE = new Set(['leverage_partner']);

	let shareChoiceKey = $state<string | null>(null);
	let shareAssetID = $state<number | null>(null);
	let shareDieFace = $state<number | null>(null);
	let shareBusy = $state(false);
	let iShared = $state(false);

	// Reset per-plan state on plan change.
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			keepSecretAssetID = null;
			shareChoiceKey = null;
			shareAssetID = null;
			shareDieFace = null;
			iShared = false;
		}
	});

	const shareSubmittable = $derived.by(() => {
		if (shareChoiceKey == null) return false;
		if (SHARE_NEEDS_ASSET.has(shareChoiceKey) && shareAssetID == null) return false;
		if (SHARE_NEEDS_FACE.has(shareChoiceKey)
			&& (shareDieFace == null || shareDieFace < 1 || shareDieFace > 6)) return false;
		return true;
	});

	async function onShare(p: Plan) {
		if (!shareSubmittable || shareBusy || shareChoiceKey == null) return;
		shareBusy = true; resError = '';
		try {
			await shareChoice(p.id, {
				choice: shareChoiceKey,
				target_asset_id: SHARE_NEEDS_ASSET.has(shareChoiceKey) ? shareAssetID : null,
				die_face: SHARE_NEEDS_FACE.has(shareChoiceKey) ? shareDieFace : null,
			});
			iShared = true;
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not submit share choice.';
		} finally { shareBusy = false; }
	}

	// ── Complete ─────────────────────────────────────────────────────────────
	let completeBusy = $state(false);
	async function onComplete(p: Plan) {
		if (completeBusy) return;
		completeBusy = true; resError = '';
		try {
			await completePlan(p.id);
			onPlansChanged();
		} catch (e) {
			resError = e instanceof Error ? e.message : 'Could not complete plan.';
		} finally { completeBusy = false; }
	}
</script>

{#if mode === 'prep'}
	<div class="plan-form">
		{#if prepError}<p class="res-error">{prepError}</p>{/if}
		<label class="form-label">
			Partner:
			<select bind:value={clPartnerID} class="form-textarea" style="height:auto;">
				<option value={null}>— pick a partner —</option>
				{#each otherPlayers as p}
					<option value={p.id}>{p.display_name}</option>
				{/each}
			</select>
		</label>
		<label class="form-label">
			Notes (optional):
			<textarea rows={2} bind:value={prepNotes} class="form-textarea"
				placeholder="Where are you meeting? In what capacity?"></textarea>
		</label>
		<p class="choices-note muted">
			Once prepared, you and your partner will each reveal a die face to set
			the delay (delay = ceil(average)).
		</p>
		<button class="action-btn primary" onclick={submitPrep} disabled={prepBusy}>
			{prepBusy ? '…' : 'Prepare Clandestinely Liaise'}
		</button>
	</div>

{:else if mode === 'delay-reveal' && plan}
	<!-- Delay reveal — the plan is pending at row 0 until both faces are in. -->
	<div class="plan-panel pending">
		<div class="plan-header">
			<span class="plan-badge pending-badge">Pending — delay reveal</span>
			<strong class="plan-title">Clandestinely Liaise</strong>
			<span class="plan-preparer">by {playerName(players, plan.preparer_id)}</span>
		</div>
		{#if plan.preparation_notes}
			<p class="plan-notes">"{plan.preparation_notes}"</p>
		{/if}
		{#if amParticipant && clState.delayRevealID != null && currentPlayerID != null}
			<SimultaneousRevealInput
				revealID={clState.delayRevealID}
				{currentPlayerID}
				{participants}
				prompt="Pick a die face to set the delay"
			/>
		{:else}
			<p class="choices-note muted">
				{playerName(players, plan.preparer_id)} and
				{playerName(players, clState.partnerID)} are settling on a time…
			</p>
		{/if}
	</div>

{:else if plan}
	<ResolvingCard {plan} {players} error={resError}>
		{#if !amParticipant}
			<p class="ft-prompt muted">A clandestine liaison is underway.</p>

		<!-- Phase 1: Together At Last ─────────────────────────────── -->
		{:else if clState.phase === 'together_at_last'}
			<div class="choices-section">
				<p class="choices-header">Together at last</p>
				<p class="choices-note">
					Set the scene of your meeting in the posts below. When you're ready
					to move on, the preparer advances the liaison.
				</p>
				{#if amPreparer}
					<button class="action-btn primary"
						onclick={() => onAdvance(plan)} disabled={advanceBusy}>
						{advanceBusy ? '…' : 'Advance to Secrets We Keep'}
					</button>
				{:else}
					<p class="choices-note muted">
						Waiting for {playerName(players, plan.preparer_id)} to advance…
					</p>
				{/if}
			</div>

		<!-- Phase 2: Secrets We Keep ──────────────────────────────── -->
		{:else if clState.phase === 'secrets_we_keep'}
			<div class="choices-section">
				<p class="choices-header">Secrets we keep</p>
				<p class="choices-note">
					Each of you nominates one of your own assets to hold the secret of
					this meeting. Picks are revealed only once both have submitted.
				</p>
				{#if iKeptSecret}
					<p class="choices-note">
						You've committed your secret. Waiting for
						{#each participants.filter(p => !keepSecretSubmittedIDs.has(p.player_id)) as p, i}
							{i > 0 ? ', ' : ''}{p.display_name}
						{/each}…
					</p>
				{:else if myUnleveragedAssets.length === 0}
					<p class="choices-note muted">
						You have no un-leveraged assets to bear the secret.
					</p>
				{:else}
					<div class="choice-list">
						{#each myUnleveragedAssets as a}
							<label class="choice-item" style="display:flex;align-items:center;gap:0.5rem;">
								<input type="radio" name="keep-secret-{plan.id}"
									value={a.id}
									checked={keepSecretAssetID === a.id}
									onchange={() => (keepSecretAssetID = a.id)} />
								<span>{a.name}</span>
							</label>
						{/each}
					</div>
					<button class="action-btn primary"
						onclick={() => onKeepSecret(plan)}
						disabled={keepBusy || keepSecretAssetID == null}>
						{keepBusy ? '…' : 'Keep this secret'}
					</button>
				{/if}

				{#if amPreparer && keepSecretSubmittedIDs.size >= 2}
					<button class="action-btn primary" style="margin-top:0.5rem;"
						onclick={() => onAdvance(plan)} disabled={advanceBusy}>
						{advanceBusy ? '…' : 'Advance to Things We Share'}
					</button>
				{/if}
			</div>

		<!-- Phase 3: Things We Share ──────────────────────────────── -->
		{:else if clState.phase === 'things_we_share'}
			<div class="choices-section">
				<p class="choices-header">Things we share</p>
				<p class="choices-note">
					Pick one option. Both picks are revealed once both have submitted.
				</p>
				{#if iShared}
					<p class="choices-note">You've submitted. Waiting for your partner…</p>
				{:else}
					<div class="choice-list">
						{#each SHARE_OPTIONS as opt}
							<label class="choice-item" style="display:block;">
								<input type="radio" name="share-{plan.id}"
									value={opt.key}
									checked={shareChoiceKey === opt.key}
									onchange={() => {
										shareChoiceKey = opt.key;
										shareAssetID = null;
										shareDieFace = null;
									}} />
								<strong>{opt.label}</strong>
								<div class="choices-note muted" style="margin-left:1.5rem;">{opt.hint}</div>
							</label>
						{/each}
					</div>

					{#if shareChoiceKey && SHARE_NEEDS_ASSET.has(shareChoiceKey)}
						<label class="form-label" style="margin-top:0.5rem;">
							{shareChoiceKey === 'take_gift' ? "Partner's gift" : "Partner's asset"}:
							<select bind:value={shareAssetID} class="form-textarea" style="height:auto;">
								<option value={null}>— pick an asset —</option>
								{#each otherParticipantAssets.filter(a =>
									shareChoiceKey === 'take_gift' ? a.asset_type !== 'peer' : true,
								) as a}
									<option value={a.id}>{a.name} ({a.asset_type})</option>
								{/each}
							</select>
						</label>
					{/if}

					{#if shareChoiceKey && SHARE_NEEDS_FACE.has(shareChoiceKey)}
						<label class="form-label">
							Die face to bank (1–6):
							<input type="number" min="1" max="6"
								bind:value={shareDieFace} class="form-textarea"
								style="width:4rem;height:auto;" />
						</label>
					{/if}

					<button class="action-btn primary"
						onclick={() => onShare(plan)}
						disabled={shareBusy || !shareSubmittable}>
						{shareBusy ? '…' : 'Submit share choice'}
					</button>
				{/if}

				{#if amPreparer}
					<button class="action-btn" style="margin-top:0.5rem;"
						onclick={() => onAdvance(plan)} disabled={advanceBusy}>
						{advanceBusy ? '…' : 'Advance to "When will I see you again?"'}
					</button>
					<p class="choices-note muted">
						(Advance once both players have submitted above.)
					</p>
				{/if}
			</div>

		<!-- Phase 4: When Will I See You Again ────────────────────── -->
		{:else if clState.phase === 'when_will_i_see_you_again'}
			<div class="choices-section">
				<p class="choices-header">When will I see you again?</p>
				<p class="choices-note">
					Reveal a die face (1–6) to schedule another liaison, or 0 to
					part ways. If either of you picks 0, no follow-up is scheduled.
				</p>
				{#if clState.redelayRevealID != null && currentPlayerID != null}
					<SimultaneousRevealInput
						revealID={clState.redelayRevealID}
						{currentPlayerID}
						{participants}
						allowZero={true}
						prompt="Pick a face (0 = cancel)"
					/>
				{/if}
			</div>

		<!-- Done ──────────────────────────────────────────────────── -->
		{:else if clState.phase === 'done'}
			<div class="complete-section">
				<p class="choices-applied">The liaison is complete.</p>
				{#if amPreparer}
					<button class="action-btn primary"
						onclick={() => onComplete(plan)} disabled={completeBusy}>
						{completeBusy ? '…' : 'Complete plan'}
					</button>
				{:else}
					<p class="choices-note muted">
						Waiting for {playerName(players, plan.preparer_id)} to close the plan…
					</p>
				{/if}
			</div>

		{:else}
			<p class="ft-prompt muted">Phase: {clState.phase || '(unknown)'}</p>
		{/if}
	</ResolvingCard>
{/if}
