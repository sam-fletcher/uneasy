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
	import { onMount, onDestroy } from 'svelte';
	import {
		preparePlan, completePlan,
		getWarState, joinWar, postWarScene,
		payBattleCost, payWarEntry, takeSurrenderAsset,
		proposePeace, votePeace,
		type Plan, type Asset, type Player,
		type WarStateResponse, type WarParticipantInfo,
	} from '$lib/api';
	import SimultaneousRevealInput from './SimultaneousRevealInput.svelte';
	import { playerName, parseResolutionData } from './shared';

	interface Props {
		mode: 'prep' | 'war';
		gameID: number;
		assets: Asset[];
		players: Player[];
		currentPlayerID: number | null;
		plan?: Plan | null;
		isFocusPlayer?: boolean;
		onPlansChanged?: () => void;
		onPlanPrepared?: () => void;
	}

	let {
		mode, gameID, assets, players, currentPlayerID,
		plan = null, isFocusPlayer = false,
		onPlansChanged = () => {},
		onPlanPrepared = () => {},
	}: Props = $props();

	// ── Prep ─────────────────────────────────────────────────────────────────
	let enemyIDs = $state<Set<number>>(new Set());
	let prepNotes = $state('');
	let prepBusy = $state(false);
	let prepError = $state('');

	const otherPlayers = $derived(players.filter(p => p.id !== currentPlayerID));

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

	onMount(() => {
		if (mode === 'war') {
			refreshWar();
			for (const t of [
				'war.declared', 'war.player_joined', 'war.battle_cost_due',
				'war.battle_cost_paid', 'war.player_surrendered', 'war.asset_seized',
				'war.entry_completed', 'war.peace_proposed', 'war.peace_vote',
				'war.ended',
			]) window.addEventListener(`uneasy:${t}`, onWarEvent);
		}
	});
	onDestroy(() => {
		for (const t of [
			'war.declared', 'war.player_joined', 'war.battle_cost_due',
			'war.battle_cost_paid', 'war.player_surrendered', 'war.asset_seized',
			'war.entry_completed', 'war.peace_proposed', 'war.peace_vote',
			'war.ended',
		]) window.removeEventListener(`uneasy:${t}`, onWarEvent);
	});

	// Re-fetch when the plan changes.
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (mode === 'war' && plan && plan.id !== lastPlanID) {
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

	// ── Cost picker form state ──────────────────────────────────────────────
	type CostKind = 'break_asset' | 'leverage_two' | 'propose_peace';
	let costOpponentID = $state<number | null>(null);
	let costKind = $state<CostKind>('break_asset');
	let costMarginaliaID = $state<number | null>(null);
	let costAsset1 = $state<number | null>(null);
	let costAsset2 = $state<number | null>(null);
	let costSurrender = $state(false);
	let peaceTerms = $state('');
	let costBusy = $state(false);

	const myMarginalia = $derived(
		currentPlayerID == null ? [] :
		assets
			.filter(a => a.owner_id === currentPlayerID && !a.is_destroyed)
			.flatMap(a => (a.marginalia ?? [])
				.filter(m => !m.is_torn)
				.map(m => ({ id: m.id, label: `${a.name} — "${m.text}"` }))),
	);
	const myUnleveraged = $derived(
		currentPlayerID == null ? [] :
		assets.filter(a =>
			a.owner_id === currentPlayerID && !a.is_destroyed && !a.is_leveraged,
		),
	);

	const costSubmittable = $derived.by(() => {
		if (costKind === 'propose_peace') return peaceTerms.trim().length > 0;
		if (costOpponentID == null) return false;
		if (costKind === 'break_asset') return costMarginaliaID != null;
		// leverage_two
		return costAsset1 != null && costAsset2 != null && costAsset1 !== costAsset2;
	});

	function resetCostForm() {
		costMarginaliaID = null;
		costAsset1 = null;
		costAsset2 = null;
		costSurrender = false;
		peaceTerms = '';
	}

	async function submitCost() {
		if (!plan || !costSubmittable || costBusy) return;
		costBusy = true; actionError = '';
		try {
			if (costKind === 'propose_peace') {
				await proposePeace(plan.id, peaceTerms.trim());
			} else if (costKind === 'break_asset') {
				await payBattleCost(plan.id, {
					opponent_id: costOpponentID!,
					choice: 'break_asset',
					marginalia_id: costMarginaliaID!,
					surrender: costSurrender,
				});
			} else {
				await payBattleCost(plan.id, {
					opponent_id: costOpponentID!,
					choice: 'leverage_two',
					asset_id_1: costAsset1!,
					asset_id_2: costAsset2!,
					surrender: costSurrender,
				});
			}
			resetCostForm();
			await refreshWar();
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not pay cost.';
		} finally { costBusy = false; }
	}

	// ── War-entry (late joiner) form ────────────────────────────────────────
	let entryOpponentID = $state<number | null>(null);
	let entryKind = $state<'break_asset' | 'leverage_two'>('break_asset');
	let entryMarginaliaID = $state<number | null>(null);
	let entryAsset1 = $state<number | null>(null);
	let entryAsset2 = $state<number | null>(null);
	let entryBusy = $state(false);

	const entrySubmittable = $derived.by(() => {
		if (entryOpponentID == null) return false;
		if (entryKind === 'break_asset') return entryMarginaliaID != null;
		return entryAsset1 != null && entryAsset2 != null && entryAsset1 !== entryAsset2;
	});

	async function submitEntry() {
		if (!plan || !entrySubmittable || entryBusy) return;
		entryBusy = true; actionError = '';
		try {
			if (entryKind === 'break_asset') {
				await payWarEntry(plan.id, {
					opponent_id: entryOpponentID!,
					choice: 'break_asset',
					marginalia_id: entryMarginaliaID!,
				});
			} else {
				await payWarEntry(plan.id, {
					opponent_id: entryOpponentID!,
					choice: 'leverage_two',
					asset_id_1: entryAsset1!,
					asset_id_2: entryAsset2!,
				});
			}
			entryMarginaliaID = null; entryAsset1 = null; entryAsset2 = null;
			await refreshWar();
		} catch (e) {
			actionError = e instanceof Error ? e.message : 'Could not pay entry.';
		} finally { entryBusy = false; }
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
		mode === 'war'
		&& plan != null
		&& plan.status === 'resolved'
		&& war != null
		&& war.status === 'ended',
	);
</script>

{#if mode === 'prep'}
	<div class="plan-form">
		{#if prepError}<p class="res-error">{prepError}</p>{/if}
		<p class="form-label">Declare war on (pick at least one):</p>
		<div class="choice-list">
			{#each otherPlayers as p}
				<label class="choice-item" style="display:flex;align-items:center;gap:0.5rem;">
					<input type="checkbox" checked={enemyIDs.has(p.id)}
						onchange={() => toggleEnemy(p.id)} />
					<span>{p.display_name}</span>
				</label>
			{/each}
		</div>
		<label class="form-label">
			Notes (optional):
				<textarea rows={2} bind:value={prepNotes} class="form-textarea"
					placeholder="Casus belli, opening moves, anything to set the scene…"></textarea>
		</label>
		<p class="choices-note muted">
			Once prepared, you and every named enemy each reveal a die face;
			delay = ceil(average). Other players may join either side during
			that window.
		</p>
		<button class="action-btn primary" onclick={submitPrep} disabled={prepBusy}>
			{prepBusy ? '…' : 'Declare war'}
		</button>
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
		{#if war && delayRevealID != null && plan.row_number === 0 && war.status === 'active'}
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
				<label class="form-label">
					Opponent:
					<select bind:value={costOpponentID} class="form-textarea" style="height:auto;">
						<option value={null}>— pick the opponent —</option>
						{#each myOwedOpponents as id}
							<option value={id}>{playerName(players, id)}</option>
						{/each}
					</select>
				</label>

				<div class="choice-list">
					<label class="choice-item">
						<input type="radio" name="cost-{plan.id}" value="break_asset"
							checked={costKind === 'break_asset'}
							onchange={() => { costKind = 'break_asset'; resetCostForm(); }} />
						<strong>Break an asset</strong> — tear one of your marginalia
					</label>
					<label class="choice-item">
						<input type="radio" name="cost-{plan.id}" value="leverage_two"
							checked={costKind === 'leverage_two'}
							onchange={() => { costKind = 'leverage_two'; resetCostForm(); }} />
						<strong>Leverage two</strong> — leverage two of your un-leveraged assets
					</label>
					<label class="choice-item">
						<input type="radio" name="cost-{plan.id}" value="propose_peace"
							checked={costKind === 'propose_peace'}
							onchange={() => { costKind = 'propose_peace'; resetCostForm(); }} />
						<strong>Propose peace</strong> — open a vote on terms; if it doesn't
						pass unanimously you'll still need to pay using one of the options above
					</label>
				</div>

				{#if costKind === 'break_asset'}
					<label class="form-label">
						Marginalia to tear:
						<select bind:value={costMarginaliaID} class="form-textarea" style="height:auto;">
							<option value={null}>— pick a marginalium —</option>
							{#each myMarginalia as m}
								<option value={m.id}>{m.label}</option>
							{/each}
						</select>
					</label>
				{:else if costKind === 'leverage_two'}
					<label class="form-label">
						First asset to leverage:
						<select bind:value={costAsset1} class="form-textarea" style="height:auto;">
							<option value={null}>— pick an asset —</option>
							{#each myUnleveraged as a}
								<option value={a.id} disabled={a.id === costAsset2}>{a.name}</option>
							{/each}
						</select>
					</label>
					<label class="form-label">
						Second asset to leverage:
						<select bind:value={costAsset2} class="form-textarea" style="height:auto;">
							<option value={null}>— pick an asset —</option>
							{#each myUnleveraged as a}
								<option value={a.id} disabled={a.id === costAsset1}>{a.name}</option>
							{/each}
						</select>
					</label>
				{:else}
					<label class="form-label">
						Peace terms:
						<textarea rows={3} bind:value={peaceTerms} class="form-textarea"
							placeholder="Describe the terms you propose…"></textarea>
					</label>
				{/if}

				<label class="form-label" style="display:flex;align-items:center;gap:0.5rem;"
					title={costKind === 'propose_peace'
						? "Doesn't apply when proposing peace."
						: 'After this payment is recorded you will be marked surrendered. ' +
						  'Each opposing non-surrendered opponent will get to claim one of your assets.'}>
					<input type="checkbox" bind:checked={costSurrender}
						disabled={costKind === 'propose_peace'} />
					<span class:muted={costKind === 'propose_peace'}>
						Surrender after this payment
					</span>
				</label>

				<button class="action-btn primary" onclick={submitCost}
					disabled={costBusy || !costSubmittable}>
					{costBusy ? '…' : (costKind === 'propose_peace' ? 'Propose peace' : 'Pay cost')}
				</button>
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
					<label class="form-label">
						Opponent:
						<select bind:value={entryOpponentID} class="form-textarea" style="height:auto;">
							<option value={null}>— pick an opponent —</option>
							{#each entryOpponentsOutstanding as id}
								<option value={id}>{playerName(players, id)}</option>
							{/each}
						</select>
					</label>
					<div class="choice-list">
						<label class="choice-item">
							<input type="radio" name="entry-{plan.id}" value="break_asset"
								checked={entryKind === 'break_asset'}
								onchange={() => { entryKind = 'break_asset'; entryMarginaliaID = null; entryAsset1 = null; entryAsset2 = null; }} />
							Break an asset
						</label>
						<label class="choice-item">
							<input type="radio" name="entry-{plan.id}" value="leverage_two"
								checked={entryKind === 'leverage_two'}
								onchange={() => { entryKind = 'leverage_two'; entryMarginaliaID = null; entryAsset1 = null; entryAsset2 = null; }} />
							Leverage two
						</label>
					</div>
					{#if entryKind === 'break_asset'}
						<label class="form-label">
							Marginalia to tear:
							<select bind:value={entryMarginaliaID} class="form-textarea" style="height:auto;">
								<option value={null}>— pick a marginalium —</option>
								{#each myMarginalia as m}
									<option value={m.id}>{m.label}</option>
								{/each}
							</select>
						</label>
					{:else}
						<label class="form-label">
							First asset:
							<select bind:value={entryAsset1} class="form-textarea" style="height:auto;">
								<option value={null}>— pick —</option>
								{#each myUnleveraged as a}
									<option value={a.id} disabled={a.id === entryAsset2}>{a.name}</option>
								{/each}
							</select>
						</label>
						<label class="form-label">
							Second asset:
							<select bind:value={entryAsset2} class="form-textarea" style="height:auto;">
								<option value={null}>— pick —</option>
								{#each myUnleveraged as a}
									<option value={a.id} disabled={a.id === entryAsset1}>{a.name}</option>
								{/each}
							</select>
						</label>
					{/if}
					<button class="action-btn primary" onclick={submitEntry}
						disabled={entryBusy || !entrySubmittable}>
						{entryBusy ? '…' : 'Pay entry against this opponent'}
					</button>
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
					<div style="display:flex;gap:0.5rem;">
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
				{#each myClaims as c}
					<div class="choice-item" style="display:block;margin-bottom:0.5rem;">
						<p>
							{playerName(players, c.surrendered_id)} surrendered. Pick one
							of their assets to take:
						</p>
						<select bind:value={claimAssetByClaim[c.id]} class="form-textarea" style="height:auto;">
							<option value={undefined}>— pick an asset —</option>
							{#each targetAssetsFor(c.surrendered_id) as a}
								<option value={a.id}>{a.name} ({a.asset_type})</option>
							{/each}
						</select>
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
