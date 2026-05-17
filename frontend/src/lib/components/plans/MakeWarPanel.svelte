<!-- MakeWarPanel.svelte
  Prep + full lifecycle UI for Make War (Tier 3, Power, variable delay).

  A Make War plan creates a `war` row and a simultaneous delay reveal at prep
  time. The plan itself sits at row 0 (pending) until the reveal closes; then
  it advances to the rolled row and resolves narratively (declaration scene +
  complete). The war persists across rows and ends only via peace or full
  surrender — so the cost-of-battle picker, peace flow, and surrender-claim UI
  must remain visible long after the plan itself has resolved.

  This component therefore covers four overlapping surfaces from one mount:
   1. Delay reveal (pending plan + open reveal). Participants reveal a face;
      non-participants see Join side 1 / 2 buttons.
   2. War status panel (war.status === 'active'). Lists participants by side,
      surrender markers, current cost-of-battle pointer, and the "do my turn"
      cost picker / peace flow / surrender-claim widgets.
   3. Resolution UI (plan.status === 'resolving'). Focus player posts the
      one-time declaration scene then completes the plan.
   4. Ended summary (war.status === 'ended'). Brief "war ended (peace/etc)".
-->
<script lang="ts">
	import './planPanel.css';
	import { onMount } from 'svelte';
	import { useWindowEvents } from '$lib/useWindowEvents';
	import { WAR_EVENTS } from '$lib/ws';
	import {
		preparePlan, completePlan,
		getWarState, joinWar, postWarScene,
		payBattleCost, payWarEntry, takeSurrenderAsset,
		proposePeace, votePeace,
		type Plan, type Asset, type Player,
		type WarStateResponse, type WarParticipantInfo,
	} from '$lib/api';
	import SimultaneousRevealInput from './SimultaneousRevealInput.svelte';
	import BattleCostForm, { type BattleSubmission } from './war/BattleCostForm.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import CardPicker from './CardPicker.svelte';
	import {
		playerName, parseResolutionData,
		assetsWithIntactMarginalia, playersExcept, ownerUnleveragedAssets,
	} from './shared';

	import type { PlanPanelProps } from './types';
	import FormField from './FormField.svelte';

	let { ctx, plan = null, mode }: PlanPanelProps = $props();

	const gameID = $derived(ctx.gameID);
	const assets = $derived(ctx.assets);
	const players = $derived(ctx.players);
	const currentPlayerID = $derived(ctx.currentPlayerID);
	const isFocusPlayer = $derived(ctx.isFocusPlayer);
	const onPlansChanged = $derived(ctx.onPlansChanged);
	const onPlanPrepared = $derived(ctx.onPlanPrepared);

	// Make War uses one underlying view for both resolve and alwaysOn
	// dispatches — both fall through to "the war view". This local alias
	// keeps the existing isWarView checks below readable.
	const isWarView = $derived(mode === 'resolve' || mode === 'alwaysOn');

	// ── Prep ─────────────────────────────────────────────────────────────────
	let enemyIDs = $state<Set<number>>(new Set());
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	const otherPlayers = $derived(playersExcept(players, currentPlayerID));

	function toggleEnemy(id: number) {
		const next = new Set(enemyIDs);
		if (next.has(id)) next.delete(id); else next.add(id);
		enemyIDs = next;
	}

	async function submitPrep() {
		if (prepBusy) return;
		if (enemyIDs.size === 0) { prepError = 'Pick at least one enemy.'; return; }
		prepBusy = true; prepError = '';
		try {
			await preparePlan(gameID, {
				plan_type: 'make_war',
				enemy_player_ids: [...enemyIDs],
				preparation_notes: prepNotes.trim() || null,
			});
			enemyIDs = new Set();
			prepNotes = '';
			onPlanPrepared();
		} catch (e) {
			prepError = e instanceof Error ? e.message : 'Could not prepare plan.';
		} finally { prepBusy = false; }
	}

	// ── War-mode state ───────────────────────────────────────────────────────
	let war = $state<WarStateResponse | null>(null);
	let warError = $state('');
	let actionError = $state('');

	// Resolution data on the plan tells us about the delay reveal + scene.
	const planRD = $derived(parseResolutionData(plan));
	const delayRevealID = $derived(planRD.delay_reveal_id ?? null);
	const warScenePosted = $derived(planRD.war_scene_posted ?? false);

	async function refreshWar() {
		if (!plan) return;
		try {
			war = await getWarState(plan.id);
			warError = '';
		} catch (e) {
			// 404 just means the war row hasn't been created yet — fine.
			const msg = e instanceof Error ? e.message : '';
			if (!/no war/i.test(msg)) warError = msg || 'Could not load war state';
			war = null;
		}
	}

	function onWarEvent() { refreshWar(); }

	useWindowEvents(WAR_EVENTS, onWarEvent);
	onMount(() => { if (isWarView) refreshWar(); });

	// Re-fetch when the plan changes.
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (isWarView && plan && plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			refreshWar();
		}
	});

	// ── Derived participant views ────────────────────────────────────────────
	const myPart = $derived<WarParticipantInfo | null>(
		war && currentPlayerID != null
			? war.participants.find(p => p.player_id === currentPlayerID) ?? null
			: null,
	);
	const amParticipant = $derived(myPart != null);
	const amSurrendered = $derived(myPart?.surrendered_at_row != null);
	const amFullParticipant = $derived(
		myPart != null && myPart.entry_payment_complete && !amSurrendered,
	);

	function sideName(s: 1 | 2): string {
		return s === 1 ? 'Side 1 (declarer)' : 'Side 2 (enemies)';
	}

	// "Whose turn is it to pay cost-of-battle?" — first outstanding entry.
	const activePayerID = $derived(war?.outstanding_costs[0]?.payer_id ?? null);
	const itsMyCostTurn = $derived(activePayerID != null && activePayerID === currentPlayerID);

	// Opponents I owe THIS row (only when it's my turn).
	const myOwedOpponents = $derived(
		war?.outstanding_costs
			.filter(c => c.payer_id === currentPlayerID)
			.map(c => c.opponent_id) ?? [],
	);

	// Open peace proposal — can I vote?
	const proposal = $derived(war?.open_proposal ?? null);
	const myProposalVote = $derived(
		proposal?.votes.find(v => v.player_id === currentPlayerID) ?? null,
	);
	const canVoteProposal = $derived(
		proposal != null && amFullParticipant
		&& proposal.proposer_id !== currentPlayerID
		&& !myProposalVote,
	);

	// Open surrender claims I hold.
	const myClaims = $derived(
		war?.open_claims.filter(c => c.claimant_id === currentPlayerID) ?? [],
	);

	// Late-joiner: opponents I still owe entry to (every opposing full
	// participant minus those I've already paid entry against this row).
	const entryOpponentsOutstanding = $derived.by<number[]>(() => {
		if (!war || myPart == null || myPart.entry_payment_complete) return [];
		const mySide = myPart.side;
		const opponents = war.participants
			.filter(p => p.entry_payment_complete && p.surrendered_at_row == null && p.side !== mySide)
			.map(p => p.player_id);
		const paid = new Set(
			war.battle_costs
				.filter(bc => bc.is_entry && bc.payer_id === currentPlayerID)
				.map(bc => bc.opponent_id),
		);
		return opponents.filter(id => !paid.has(id));
	});

	// ── Sub-form inputs (shared by cost + entry pickers) ───────────────────
	// Assets owned by the current player with at least one intact marginalium.
	// BattleCostForm's marginalia-pick mode renders each card with per-line
	// checkboxes for the asset's intact lines.
	const myMarginaliaAssets = $derived(
		assetsWithIntactMarginalia(assets, currentPlayerID),
	);
	const myUnleveraged = $derived(ownerUnleveragedAssets(assets, currentPlayerID));

	async function handleCostSubmit(s: BattleSubmission) {
		if (!plan) return;
		actionError = '';
		try {
			if (s.kind === 'peace') {
				await proposePeace(plan.id, s.terms);
			} else if (s.choice === 'break_asset') {
				await payBattleCost(plan.id, {
					opponent_id: s.opponent_id, choice: 'break_asset',
					marginalia_id: s.marginalia_id, surrender: s.surrender,
				});
			} else {
				await payBattleCost(plan.id, {
					opponent_id: s.opponent_id, choice: 'leverage_two',
					asset_id_1: s.asset_id_1, asset_id_2: s.asset_id_2,
					surrender: s.surrender,
				});
			}
			await refreshWar();
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not pay cost.';
			throw e;
		}
	}

	async function handleEntrySubmit(s: BattleSubmission) {
		if (!plan || s.kind !== 'battle') return;
		actionError = '';
		try {
			if (s.choice === 'break_asset') {
				await payWarEntry(plan.id, {
					opponent_id: s.opponent_id, choice: 'break_asset',
					marginalia_id: s.marginalia_id,
				});
			} else {
				await payWarEntry(plan.id, {
					opponent_id: s.opponent_id, choice: 'leverage_two',
					asset_id_1: s.asset_id_1, asset_id_2: s.asset_id_2,
				});
			}
			await refreshWar();
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not pay entry.';
			throw e;
		}
	}

	// ── Surrender-claim form ────────────────────────────────────────────────
	let claimAssetByClaim = $state<Record<number, number | null>>({});
	let claimBusy = $state(false);

	function targetAssetsFor(surrenderedID: number): Asset[] {
		return assets.filter(a => a.owner_id === surrenderedID && !a.is_destroyed);
	}

	async function submitClaim(claimID: number, surrenderedID: number) {
		if (!plan || claimBusy) return;
		const assetID = claimAssetByClaim[claimID];
		if (assetID == null) return;
		claimBusy = true; actionError = '';
		try {
			await takeSurrenderAsset(plan.id, surrenderedID, assetID);
			claimAssetByClaim = { ...claimAssetByClaim, [claimID]: null };
			await refreshWar();
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not claim asset.';
		} finally { claimBusy = false; }
	}

	// ── Peace voting ────────────────────────────────────────────────────────
	let voteBusy = $state(false);
	async function castVote(accepted: boolean) {
		if (!plan || !proposal || voteBusy) return;
		voteBusy = true; actionError = '';
		try {
			await votePeace(plan.id, proposal.id, accepted);
			await refreshWar();
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not vote.';
		} finally { voteBusy = false; }
	}

	// ── Join war (non-participants) ─────────────────────────────────────────
	let joinBusy = $state(false);
	async function joinSide(side: 1 | 2) {
		if (!plan || joinBusy) return;
		joinBusy = true; actionError = '';
		try {
			await joinWar(plan.id, side);
			await refreshWar();
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not join war.';
		} finally { joinBusy = false; }
	}

	// ── Declaration scene + complete (focus, status=resolving) ──────────────
	let sceneBusy = $state(false);
	let completeBusy = $state(false);
	async function onPostScene() {
		if (!plan || sceneBusy) return;
		sceneBusy = true; actionError = '';
		try {
			await postWarScene(plan.id);
			onPlansChanged();
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not mark scene posted.';
		} finally { sceneBusy = false; }
	}
	async function onComplete() {
		if (!plan || completeBusy) return;
		completeBusy = true; actionError = '';
		try {
			await completePlan(plan.id);
			onPlansChanged();
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not complete plan.';
		} finally { completeBusy = false; }
	}

	// ── Reveal participants list (for SimultaneousRevealInput) ───────────────
	const revealParticipants = $derived(
		war?.participants.map(p => ({
			player_id: p.player_id,
			display_name: playerName(players, p.player_id),
		})) ?? [],
	);

	// Don't render anything if the plan is fully resolved AND the war ended.
	const shouldHide = $derived(
		isWarView
		&& plan != null
		&& plan.status === 'resolved'
		&& war != null
		&& war.status === 'ended',
	);
</script>

{#if mode === 'prep'}
	<div class="plan-form">
		{#if prepError}<p class="res-error">{prepError}</p>{/if}
		<FormField label="Declare war on (one or more)">
			<PlayerChips
				players={otherPlayers}
				isActive={(p) => enemyIDs.has(p.id)}
				onSelect={(p) => toggleEnemy(p.id)}
			/>
		</FormField>
		<label class="form-label">
			Notes (optional):
				<textarea rows={2} bind:value={prepNotes} class="form-textarea"
					placeholder="Casus belli, opening move, rally cry, et cetera"></textarea>
		</label>
		<p class="choices-note muted">
			Once declared, all involved players reveal a die to set the delay (average rounded up).
			Other players may join either side whenever the Public Record advances.
		</p>
		<div class="form-actions">
			<button class="action-btn primary" onclick={submitPrep}
				disabled={prepBusy || enemyIDs.size === 0}>
				{prepBusy ? '…' : 'Declare War'}
			</button>
		</div>
	</div>

{:else if plan && !shouldHide}
	<div class="plan-panel resolving">
		<div class="plan-header">
			<span class="plan-badge resolving-badge">
				{plan.status === 'resolving' ? 'War — declaration scene'
					: war?.status === 'ended' ? 'War (ended)'
					: war?.status === 'active' ? 'War — ongoing'
					: 'War — pending'}
			</span>
			<strong class="plan-title">Make War</strong>
			<span class="plan-preparer">by {playerName(players, plan.preparer_id)}</span>
		</div>
		{#if plan.preparation_notes}
			<p class="plan-notes">"{plan.preparation_notes}"</p>
		{/if}
		{#if warError}<p class="res-error">{warError}</p>{/if}
		{#if actionError}<p class="res-error">{actionError}</p>{/if}

		<!-- ── Delay reveal ────────────────────────────────────────────── -->
		{#if war && delayRevealID != null && plan.row_number == null && war.status === 'active'}
			<div class="choices-section">
				<p class="choices-header">Delay reveal</p>
				<p class="choices-note">
					Each war participant reveals a die face. Delay equals
					ceil(average). Other players may join either side below.
				</p>
				{#if amParticipant && currentPlayerID != null}
					<SimultaneousRevealInput
						revealID={delayRevealID}
						{currentPlayerID}
						participants={revealParticipants}
						prompt="Pick a die face for the war's delay"
					/>
				{:else}
					<p class="choices-note muted">
						Waiting for participants to reveal their delay dice…
					</p>
				{/if}
			</div>
		{/if}

		<!-- ── Participants by side ───────────────────────────────────── -->
		{#if war}
			<div class="choices-section">
				<p class="choices-header">Sides</p>
				{#each [1, 2] as s}
					{@const sideParts = war.participants.filter(p => p.side === s)}
					<div class="choices-note">
						<strong>{sideName(s as 1 | 2)}:</strong>
						{#if sideParts.length === 0}
							<em>(empty)</em>
						{:else}
							{#each sideParts as p, i}
								{i > 0 ? ', ' : ''}
								<span>
									{playerName(players, p.player_id)}
									{#if p.surrendered_at_row != null}
										<em>(surrendered, row {p.surrendered_at_row})</em>
									{:else if !p.entry_payment_complete}
										<em>(joining — owes entry)</em>
									{/if}
								</span>
							{/each}
						{/if}
					</div>
				{/each}

				<!-- Join buttons for non-participants -->
				{#if !amParticipant && war.status === 'active'}
					<p class="choices-note">
						Join the war (free during the delay reveal; afterwards you'll
						owe a cost-of-battle entry against every existing opposing
						participant before counting as a full member):
					</p>
					<div style="display:flex;gap:0.5rem;flex-wrap:wrap;">
						<button class="action-btn" onclick={() => joinSide(1)} disabled={joinBusy}>
							{joinBusy ? '…' : 'Join Side 1'}
						</button>
						<button class="action-btn" onclick={() => joinSide(2)} disabled={joinBusy}>
							{joinBusy ? '…' : 'Join Side 2'}
						</button>
					</div>
				{/if}
			</div>
		{/if}

		<!-- ── Cost-of-battle pointer ─────────────────────────────────── -->
		{#if war?.status === 'active' && war.outstanding_costs.length > 0}
			<div class="choices-section">
				<p class="choices-header">Cost of battle — row {war.current_row}</p>
				{#if activePayerID != null}
					<p class="choices-note">
						{itsMyCostTurn ? 'It is YOUR turn to pay cost.'
							: `Waiting on ${playerName(players, activePayerID)} to pay cost of battle.`}
					</p>
				{/if}
				<p class="choices-note muted">
					Outstanding (payer → opponent), in reverse-power order:
					{#each war.outstanding_costs as k, i}
						{i > 0 ? '; ' : ''}{playerName(players, k.payer_id)} → {playerName(players, k.opponent_id)}
					{/each}
				</p>
			</div>
		{:else if war?.status === 'active'}
			<p class="choices-note muted">No outstanding cost-of-battle this row.</p>
		{/if}

		<!-- ── My cost-of-battle picker ──────────────────────────────── -->
		{#if itsMyCostTurn && amFullParticipant}
			<div class="choices-section">
				<p class="choices-header">Your cost of battle</p>
				<BattleCostForm
					mode="cost"
					formKey={plan.id}
					opponents={myOwedOpponents}
					{players}
					marginaliaAssets={myMarginaliaAssets}
					unleveraged={myUnleveraged}
					allowPeace
					allowSurrender
					onSubmit={handleCostSubmit}
				/>
			</div>
		{/if}

		<!-- ── Late-joiner entry payments ────────────────────────────── -->
		{#if war?.status === 'active' && myPart && !myPart.entry_payment_complete}
			<div class="choices-section">
				<p class="choices-header">War entry — owed payments</p>
				{#if entryOpponentsOutstanding.length === 0}
					<p class="choices-note muted">All entry payments recorded. The server
						will mark you a full participant momentarily.</p>
				{:else}
					<p class="choices-note">
						You joined this war late. Pay one cost of battle against each
						existing opposing opponent before you count as a full
						participant. Outstanding:
						{entryOpponentsOutstanding.map(id => playerName(players, id)).join(', ')}
					</p>
					<BattleCostForm
						mode="entry"
						formKey={plan.id}
						opponents={entryOpponentsOutstanding}
						{players}
						marginaliaAssets={myMarginaliaAssets}
						unleveraged={myUnleveraged}
						allowPeace={false}
						allowSurrender={false}
						onSubmit={handleEntrySubmit}
					/>
				{/if}
			</div>
		{/if}

		<!-- ── Open peace proposal ───────────────────────────────────── -->
		{#if proposal}
			<div class="choices-section">
				<p class="choices-header">
					Peace proposal from {playerName(players, proposal.proposer_id)}
				</p>
				<p class="plan-notes">"{proposal.terms}"</p>
				<p class="choices-note">
					Accepted by:
					{#if proposal.votes.filter(v => v.accepted).length === 0}
						<em>nobody yet</em>
					{:else}
						{proposal.votes.filter(v => v.accepted)
							.map(v => playerName(players, v.player_id)).join(', ')}
					{/if}
				</p>
				{#if proposal.awaiting.length > 0}
					<p class="choices-note muted">
						Waiting on: {proposal.awaiting.map(id => playerName(players, id)).join(', ')}
					</p>
				{/if}
				{#if canVoteProposal}
					<div class="form-row">
						<button class="action-btn primary"
							onclick={() => castVote(true)} disabled={voteBusy}>
							{voteBusy ? '…' : 'Accept'}
						</button>
						<button class="action-btn"
							onclick={() => castVote(false)} disabled={voteBusy}>
							{voteBusy ? '…' : 'Reject'}
						</button>
					</div>
				{:else if myProposalVote}
					<p class="choices-note muted">
						You voted: <strong>{myProposalVote.accepted ? 'accept' : 'reject'}</strong>
					</p>
				{:else if !amFullParticipant}
					<p class="choices-note muted">Only active war participants vote on peace.</p>
				{/if}
			</div>
		{/if}

		<!-- ── Surrender claims I hold ───────────────────────────────── -->
		{#if myClaims.length > 0}
			<div class="choices-section">
				<p class="choices-header">Surrender claims</p>
				{#each myClaims as c (c.id)}
					{@const claimable = targetAssetsFor(c.surrendered_id)}
					{@const picked = claimAssetByClaim[c.id]}
					<div style="display:block;margin-bottom:0.5rem;">
						<CardPicker
							label={`${playerName(players, c.surrendered_id)} surrendered. Pick one of their assets to take`}
							items={claimable}
							{players}
							emptyMessage="No eligible assets to claim."
							selected={picked ?? null}
							onSelect={(id) => (claimAssetByClaim = { ...claimAssetByClaim, [c.id]: id })}
						/>
						<button class="action-btn primary" style="margin-top:0.4rem;"
							onclick={() => submitClaim(c.id, c.surrendered_id)}
							disabled={claimBusy || claimAssetByClaim[c.id] == null}>
							{claimBusy ? '…' : 'Claim'}
						</button>
					</div>
				{/each}
			</div>
		{/if}

		<!-- ── Resolution: declaration scene + complete ──────────────── -->
		{#if plan.status === 'resolving' && isFocusPlayer}
			<div class="choices-section">
				<p class="choices-header">Declaration scene</p>
				<p class="choices-note">
					This is the one-time scene where the war breaks open. Post the
					declaration in the scene thread above, then mark it posted here.
					(Battles between rows happen via cost-of-battle, narrated freely
					— no extra scene per row.)
				</p>
				{#if !warScenePosted}
					<button class="action-btn primary" onclick={onPostScene} disabled={sceneBusy}>
						{sceneBusy ? '…' : 'Mark declaration scene posted'}
					</button>
				{:else}
					<p class="choices-applied">Declaration scene marked posted.</p>
					<button class="action-btn primary" onclick={onComplete} disabled={completeBusy}>
						{completeBusy ? '…' : 'Complete plan'}
					</button>
					<p class="choices-note muted">
						(Completing the plan does NOT end the war — that requires peace
						or full surrender. The cost-of-battle picker above remains active
						each row until the war ends.)
					</p>
				{/if}
			</div>
		{:else if plan.status === 'resolving' && !isFocusPlayer}
			<p class="choices-note muted">
				{playerName(players, plan.preparer_id)} is posting the war's
				declaration scene…
			</p>
		{/if}

		<!-- ── Ended summary ─────────────────────────────────────────── -->
		{#if war?.status === 'ended'}
			<p class="choices-note">
				<strong>The war is over.</strong>
				{#if war.end_reason}({war.end_reason}){/if}
				{#if war.ended_at_row != null} Ended on row {war.ended_at_row}.{/if}
			</p>
		{/if}
	</div>
{/if}
