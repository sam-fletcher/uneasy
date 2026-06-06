<!-- DiceRollPanel.svelte
  Stage-driven dice roll panel. The server owns transitions
  (decide_vote → voting → leverage → resolved); clients call action
  endpoints and observe stage_changed / ready_changed / intent_set /
  vote_resolved / resolved over WS.
-->
<script lang="ts">
	import type {
		DiceRoll, DiceRollDie, VoteView, RollParticipant, RollIntent,
		Asset, Player, BankedDie,
	} from '$lib/api';
	import {
		leverageRoll, useBankedDie, callVote, skipVote, voteOnRoll,
		setRollIntent, setRollReady,
	} from '$lib/api';
	import AssetCard from './AssetCard.svelte';
	import { playerColorByID } from '$lib/playerColor';

	interface Props {
		roll: DiceRoll;
		dice: DiceRollDie[];
		votes: VoteView[];
		participants: RollParticipant[];
		bankedDice: BankedDie[];
		assets: Asset[];
		currentPlayerID: number | null;
		players: Player[];
		playerNameMap: Map<number, string>;
		/** True when the actor cannot leverage their own assets (Make Demands
		 *  control_leverage winner has authority). */
		actorLeverageBlocked?: boolean;
	}

	let {
		roll = $bindable(),
		dice = $bindable(),
		votes = $bindable(),
		participants = $bindable(),
		bankedDice = $bindable(),
		assets,
		currentPlayerID,
		players,
		playerNameMap,
		actorLeverageBlocked = false,
	}: Props = $props();

	const isActor = $derived(currentPlayerID === roll.actor_id);
	const stage = $derived(roll.stage);
	const me = $derived(participants.find(p => p.player_id === currentPlayerID) ?? null);
	const myIntent = $derived<RollIntent | 'aid' | null>(
		isActor ? 'aid' : (me?.intent ?? null)
	);
	const myReady = $derived(me?.is_ready ?? false);

	// A non-actor's intent is locked once they've committed any die for
	// this roll. Non-actors never have automatic base dice, so every die
	// belonging to them is a committed asset or banked-die spend.
	// (Intent is irrelevant for the actor — they're implicitly aiding —
	// so this check doesn't need to special-case the actor's base dice.)
	const intentLocked = $derived(
		dice.some(d => d.player_id === currentPlayerID)
	);

	const myAssets = $derived(
		assets.filter(a => a.owner_id === currentPlayerID && !a.is_destroyed)
	);
	const myUnleveragedAssets = $derived(myAssets.filter(a => !a.is_leveraged));
	const myUnspentBanked = $derived(bankedDice.filter(b => b.used_at == null));
	const canCommit = $derived(myUnleveragedAssets.length > 0 || myUnspentBanked.length > 0);

	// Split dice into pools by side.
	const actorPool = $derived(dice.filter(d => !d.is_interference));
	const intPool = $derived(dice.filter(d => d.is_interference));

	const effectiveDifficulty = $derived(roll.adjusted_difficulty ?? roll.difficulty);
	const stageLabel = $derived({
		decide_vote: 'Vote?',
		voting: 'Voting',
		leverage: 'Leverage',
		resolved: 'Resolved',
	}[stage]);

	let busy = $state(false);
	let error = $state('');
	const setErr = (e: unknown) => {
		error = e instanceof Error ? e.message : 'Action failed.';
	};
	async function run(fn: () => Promise<unknown>) {
		if (busy) return;
		busy = true; error = '';
		try { await fn(); } catch (e) { setErr(e); } finally { busy = false; }
	}

	// ── decide_vote actions ───────────────────────────────────────────────────
	const onCallVote = () => run(() => callVote(roll.id));
	const onSkipVote = () => run(() => skipVote(roll.id));

	// ── voting actions ────────────────────────────────────────────────────────
	const myVote = $derived(votes.find(v => v.player_id === currentPlayerID));
	const voteCount = $derived(votes.length);
	const onVote = (v: 1 | -1) => run(() => voteOnRoll(roll.id, v));

	// ── leverage actions ──────────────────────────────────────────────────────
	const onSetIntent = (intent: RollIntent) => run(() => setRollIntent(roll.id, intent));
	const onLeverageAsset = (asset: Asset) => run(async () => {
		const { die } = await leverageRoll(roll.id, asset.id);
		dice = [...dice, die];
	});
	const onUseBanked = (b: BankedDie) => run(async () => {
		const { die } = await useBankedDie(roll.id, b.id);
		dice = [...dice, die];
		bankedDice = bankedDice.map(x => x.id === b.id ? { ...x, used_at: new Date().toISOString(), used_roll_id: roll.id } : x);
	});
	const onToggleReady = () => run(() => setRollReady(roll.id, !myReady));

	// ── Player roster (for participants chips) ───────────────────────────────
	// A locked-ready participant (no dice to add) is called out explicitly so
	// the actor can see at a glance who's been auto-readied vs. who's still
	// thinking. The actor's "Ready" click can resolve the roll the instant
	// every other participant is ready; surfacing locked-ready prevents
	// surprise resolutions.
	function chipLabel(p: RollParticipant): string {
		const name = playerNameMap.get(p.player_id) ?? '?';
		if (p.player_id === roll.actor_id) {
			return `${name} · actor · ${p.is_ready ? 'ready' : 'not ready'}`;
		}
		const intent = p.intent ?? 'choosing…';
		if (p.is_ready && p.intent == null) {
			return `${name} · 🔒 no dice · ready`;
		}
		return `${name} · ${intent} · ${p.is_ready ? 'ready' : 'not ready'}`;
	}

	// ── Commit feed ──────────────────────────────────────────────────────────
	// Show every die EXCEPT the actor's two automatic base dice (no asset,
	// not interference). Banked-die spends and leveraged assets both show up.
	function isActorBaseDie(d: DiceRollDie): boolean {
		return d.player_id === roll.actor_id
			&& d.leveraged_asset_id == null
			&& !d.is_interference;
	}
	const commitFeed = $derived(
		dice
			.filter(d => !isActorBaseDie(d))
			.map(d => {
				const asset = d.leveraged_asset_id != null
					? assets.find(a => a.id === d.leveraged_asset_id) : undefined;
				return {
					id: d.id,
					playerID: d.player_id,
					playerName: playerNameMap.get(d.player_id) ?? '?',
					source: asset ? asset.name : 'banked die',
					isInterference: d.is_interference,
				};
			})
	);
</script>

<div class="roll-panel">
	<div class="roll-header">
		<span class="roll-title">Dice Roll</span>
		<span class="roll-meta">
			Difficulty <strong>{roll.difficulty}</strong>
			{#if roll.adjusted_difficulty != null && roll.adjusted_difficulty !== roll.difficulty}
				→ <strong class="adjusted">{roll.adjusted_difficulty}</strong>
			{/if}
		</span>
		<span class="roll-actor">Actor: {playerNameMap.get(roll.actor_id) ?? '?'}</span>
		<span class="stage-chip" data-stage={stage}>{stageLabel}</span>
	</div>

	{#if error}
		<p class="roll-error">{error}</p>
	{/if}

	<!-- Dice pools (always visible) -->
	<div class="dice-section">
		<div class="dice-group">
			<span class="dice-label">Actor pool</span>
			<div class="dice-row">
				{#each actorPool as die (die.id)}
					<div
						class="die"
						class:cancelled={die.is_cancelled}
						class:unrolled={die.face == null}
						style:border-color={playerColorByID(die.player_id, players)}
						title={`${playerNameMap.get(die.player_id) ?? '?'} · ${
							die.leveraged_asset_id
								? assets.find(a => a.id === die.leveraged_asset_id)?.name ?? 'asset'
								: 'base/banked die'
						}`}
					>{die.face ?? '🎲'}</div>
				{/each}
			</div>
		</div>
		{#if intPool.length > 0}
			<div class="dice-group interference">
				<span class="dice-label">Interference</span>
				<div class="dice-row">
					{#each intPool as die (die.id)}
						<div
							class="die int"
							class:unrolled={die.face == null}
							style:border-color={playerColorByID(die.player_id, players)}
							title={playerNameMap.get(die.player_id) ?? '?'}
						>{die.face ?? '🎲'}</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>

	<!-- ── Stage: decide_vote ──────────────────────────────────────────────── -->
	{#if stage === 'decide_vote'}
		{#if isActor}
			<div class="stage-actions">
				<button class="action-btn primary" onclick={onCallVote} disabled={busy}>
					Call difficulty vote
				</button>
				<button class="action-btn secondary" onclick={onSkipVote} disabled={busy}>
					Skip vote
				</button>
			</div>
		{:else}
			<p class="stage-hint">Waiting for {playerNameMap.get(roll.actor_id) ?? 'the actor'} to decide about a difficulty vote…</p>
		{/if}
	{/if}

	<!-- ── Stage: voting ───────────────────────────────────────────────────── -->
	{#if stage === 'voting'}
		<div class="stage-actions">
			<p class="stage-hint">
				Each vote shifts difficulty by ±1. {voteCount} of {players.length} have voted.
			</p>
			{#if myVote}
				<p class="stage-hint">
					You voted <strong>{myVote.vote === 1 ? '+1 (harder)' : '−1 (easier)'}</strong>.
					Waiting on others…
				</p>
			{:else}
				<div class="vote-buttons">
					<button class="vote-btn easier" onclick={() => onVote(-1)} disabled={busy}>
						−1 (easier)
					</button>
					<button class="vote-btn harder" onclick={() => onVote(1)} disabled={busy}>
						+1 (harder)
					</button>
				</div>
			{/if}
		</div>
	{/if}

	<!-- ── Stage: leverage ─────────────────────────────────────────────────── -->
	{#if stage === 'leverage'}
		<!-- Intent + ready row -->
		<div class="intent-row">
			{#if !isActor && !myIntent}
				<button class="intent-btn aid" onclick={() => onSetIntent('aid')} disabled={busy}>
					Aid
				</button>
				<button class="intent-btn interfere" onclick={() => onSetIntent('interfere')} disabled={busy}>
					Interfere
				</button>
			{:else if !isActor}
				<span class="intent-badge" class:locked={intentLocked}>
					{myIntent === 'aid' ? "You're aiding" : "You're interfering"}
				</span>
			{/if}
			<button
				class="ready-btn"
				class:ready={myReady}
				onclick={onToggleReady}
				disabled={busy || (myReady && !canCommit)}
				title={myReady && !canCommit ? 'You have no dice left to add — automatically ready.' : ''}
			>
				{myReady ? (canCommit ? 'Unready (add more dice)' : 'Ready (locked)') : 'Ready'}
			</button>
		</div>

		<!-- My assets (only when intent set or I'm the actor) -->
		{#if (isActor || myIntent) && myAssets.length > 0}
			<div class="my-assets">
				<span class="section-label">Your assets</span>
				{#each myAssets as asset (asset.id)}
					<AssetCard
						{asset}
						compact
						mode="roll-leverage"
						onRollLeverage={onLeverageAsset}
						rollLeverageDisabled={myReady || (isActor && actorLeverageBlocked)}
						onTear={() => {}}
					/>
				{/each}
			</div>
		{/if}

		<!-- Banked dice (only when intent set or I'm the actor) -->
		{#if (isActor || myIntent) && myUnspentBanked.length > 0}
			<div class="banked-section">
				<span class="section-label">Banked dice ({myUnspentBanked.length})</span>
				<div class="banked-list">
					{#each myUnspentBanked as b (b.id)}
						<button
							class="banked-btn"
							onclick={() => onUseBanked(b)}
							disabled={busy || myReady}
							title="Spend this banked die (random face at resolution)"
						>
							🎲 Spend (+1 die)
						</button>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Public commit feed -->
		{#if commitFeed.length > 0}
			<div class="commit-feed">
				<span class="section-label">Commits this roll</span>
				<ul>
					{#each commitFeed as c (c.id)}
						<li>
							<span class="dot" style:background={playerColorByID(c.playerID, players)}></span>
							<span class="cf-name">{c.playerName}</span>
							<span class="cf-source">· {c.source}</span>
							<span class="cf-side" class:interfere={c.isInterference}>
								· {c.isInterference ? 'interfere' : 'aid'}
							</span>
						</li>
					{/each}
				</ul>
			</div>
		{/if}

		<!-- Player roster -->
		<div class="roster">
			{#each participants as p (p.player_id)}
				<span class="chip" style:border-color={playerColorByID(p.player_id, players)}>
					{chipLabel(p)}
				</span>
			{/each}
		</div>

		<!-- Footer summary -->
		<p class="footer-summary">
			{actorPool.length} aid · {intPool.length} interfere ·
			{participants.filter(p => p.is_ready).length} of {participants.length} ready
		</p>
	{/if}

	<!-- ── Stage: resolved ─────────────────────────────────────────────────── -->
	{#if stage === 'resolved'}
		<div class="result-banner" class:make={roll.outcome === 'make'} class:mar={roll.outcome === 'mar'}>
			<span class="result-label">{roll.outcome === 'make' ? 'Make' : 'Mar'}</span>
			<span class="result-score">{roll.result} distinct face{roll.result === 1 ? '' : 's'} vs. difficulty {effectiveDifficulty}</span>
		</div>
	{/if}
</div>

<style>
	.roll-panel {
		border: 1px solid #4a3a20;
		border-radius: 6px;
		padding: 0.75rem;
		background: #1e1a10;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		flex-shrink: 0;
	}

	.roll-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.roll-title { font-weight: 700; color: var(--color-accent); font-size: 0.9rem; }
	.roll-meta { font-size: 0.85rem; color: var(--color-text-muted); }
	.adjusted { color: #e0c070; }
	.roll-actor { font-size: 0.78rem; color: var(--color-text-muted); }

	.stage-chip {
		margin-left: auto;
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 0.15rem 0.5rem;
		border-radius: 3px;
		background: var(--color-border-warm);
		color: var(--color-accent);
	}
	.stage-chip[data-stage="resolved"] { background: #1a3a1a; color: var(--color-success); }
	.stage-chip[data-stage="voting"] { background: #3a2a3a; color: #c890e0; }

	.roll-error { color: var(--color-danger); font-size: 0.82rem; margin: 0; }

	/* Dice */
	.dice-section { display: flex; flex-direction: column; gap: 0.4rem; }
	.dice-group { display: flex; align-items: center; gap: 0.5rem; }
	.dice-group.interference .dice-label { color: var(--color-danger); }
	.dice-label { font-size: 0.75rem; color: var(--color-text-muted); min-width: 90px; flex-shrink: 0; }
	.dice-row { display: flex; gap: 0.35rem; flex-wrap: wrap; }
	.die {
		width: 32px; height: 32px; border-radius: 5px;
		border: 2px solid var(--color-accent); background: #2a2010;
		color: var(--color-text); font-weight: 700; font-size: 1rem;
		display: flex; align-items: center; justify-content: center;
	}
	.die.int { background: #2a1010; color: #f0b0b0; }
	.die.cancelled { opacity: 0.3; text-decoration: line-through; border-style: dashed; }
	.die.unrolled { color: var(--color-text-muted); border-style: dashed; font-size: 1.1rem; }

	/* Stage hints / actions */
	.stage-actions { display: flex; flex-direction: column; gap: 0.5rem; }
	.stage-hint { font-size: 0.85rem; color: var(--color-text-muted); margin: 0; }

	.action-btn {
		min-height: 44px;
		padding: 0.5rem 0.8rem;
		border-radius: 5px;
		font-size: 0.9rem;
		font-weight: 600;
		cursor: pointer;
	}
	.action-btn.primary { background: var(--color-accent); color: var(--color-bg); }
	.action-btn.secondary { background: var(--color-border); color: var(--color-accent); border: 1px solid #4a4030; }
	.action-btn:disabled { opacity: 0.4; cursor: not-allowed; }

	/* Vote buttons */
	.vote-buttons { display: flex; gap: 0.5rem; }
	.vote-btn {
		flex: 1;
		min-height: 44px;
		padding: 0.5rem;
		border-radius: 5px;
		font-size: 0.9rem;
		font-weight: 700;
		cursor: pointer;
		border: 1px solid;
	}
	.vote-btn.easier { background: #0a2a0a; border-color: var(--color-success); color: var(--color-success); }
	.vote-btn.harder { background: #2a0a0a; border-color: var(--color-danger); color: var(--color-danger); }
	.vote-btn:disabled { opacity: 0.4; cursor: not-allowed; }

	/* Intent + ready row */
	.intent-row {
		display: flex;
		gap: 0.5rem;
		align-items: center;
		flex-wrap: wrap;
	}
	.intent-btn {
		flex: 1;
		min-height: 44px;
		padding: 0.5rem;
		border-radius: 5px;
		font-size: 0.9rem;
		font-weight: 700;
		border: 1px solid;
	}
	.intent-btn.aid { background: #0a2a1a; border-color: var(--color-success); color: var(--color-success); }
	.intent-btn.interfere { background: #2a0a0a; border-color: var(--color-danger); color: var(--color-danger); }
	.intent-btn:disabled { opacity: 0.4; cursor: not-allowed; }
	.intent-badge {
		font-size: 0.85rem;
		color: #e0c070;
		padding: 0.35rem 0.6rem;
		border: 1px solid #4a3a20;
		border-radius: 4px;
	}
	.intent-badge.locked { opacity: 0.6; }
	.ready-btn {
		min-height: 44px;
		padding: 0.5rem 0.8rem;
		margin-left: auto;
		border-radius: 5px;
		font-size: 0.9rem;
		font-weight: 600;
		background: var(--color-border);
		color: var(--color-text);
		border: 1px solid #555;
		cursor: pointer;
	}
	.ready-btn.ready {
		background: #1a3a1a;
		color: var(--color-success);
		border-color: var(--color-success);
	}
	.ready-btn:disabled { opacity: 0.5; cursor: not-allowed; }

	/* Sections */
	.section-label {
		font-size: 0.78rem;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	.my-assets { display: flex; flex-direction: column; gap: 0.4rem; }
	.banked-section { display: flex; flex-direction: column; gap: 0.4rem; }
	.banked-list { display: flex; flex-wrap: wrap; gap: 0.4rem; }
	.banked-btn {
		min-height: 44px;
		padding: 0.4rem 0.7rem;
		border: 1px solid var(--color-accent);
		border-radius: 4px;
		background: #2a2010;
		color: var(--color-text);
		font-weight: 600;
	}
	.banked-btn:disabled { opacity: 0.4; cursor: not-allowed; }

	/* Commit feed */
	.commit-feed { display: flex; flex-direction: column; gap: 0.3rem; }
	.commit-feed ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.2rem; }
	.commit-feed li { display: flex; align-items: center; gap: 0.4rem; font-size: 0.82rem; color: #ccc; }
	.dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
	.cf-source { color: var(--color-text-muted); }
	.cf-side { color: var(--color-success); }
	.cf-side.interfere { color: var(--color-danger); }

	/* Roster + footer */
	.roster { display: flex; flex-wrap: wrap; gap: 0.35rem; }
	.chip {
		font-size: 0.75rem;
		padding: 0.2rem 0.5rem;
		border: 1px solid #555;
		border-radius: 12px;
		color: #ccc;
	}
	.footer-summary { font-size: 0.82rem; color: var(--color-text-muted); margin: 0; }

	/* Result */
	.result-banner {
		display: flex; align-items: center; gap: 0.75rem;
		padding: 0.5rem 0.75rem; border-radius: 5px; border: 1px solid;
	}
	.result-banner.make { border-color: var(--color-success); background: #0a1f0a; }
	.result-banner.mar  { border-color: var(--color-danger); background: #1f0a0a; }
	.result-label { font-size: 1.1rem; font-weight: 800; text-transform: uppercase; letter-spacing: 0.06em; }
	.result-banner.make .result-label { color: var(--color-success); }
	.result-banner.mar  .result-label { color: var(--color-danger); }
	.result-score { font-size: 0.82rem; color: var(--color-text-muted); }
</style>
