<!-- ReAimPanel.svelte
  Shown INSTEAD of a plan's own resolve panel while the plan is paused because its
  prepare-time target is gone (resolution_data.reaim.owed — handler/plan_reaim.go).

  The book (p.12) lets a plan's target change at resolution "if the original intent
  no longer makes sense"; a destroyed peer, a rumor about an asset that no longer
  exists, a demand against a plan that has already resolved all qualify, so the
  change is forced. The preparer picks a replacement from the list the SERVER
  offers; everyone else sees what the table is waiting on.

  Nothing here decides what is valid. The options come from GET /plans/:id/reaim and
  the server accepts only a choice that matches one of them, so this component
  needs no per-rule logic beyond how to draw each plan type's kind of target:
    · Exchange Courtiers — a player, then one of their peers
    · Spread Rumors      — an asset
    · Make Demands       — another plan on the same row
-->
<script lang="ts">
	import './planPanel.css';
	import '$lib/components/shared/actionButton.css';
	import {
		getReAim, reAimPlan,
		type Plan, type Player, type Asset, type DiceRoll, type ReAimOption,
	} from '$lib/api';
	import ResolvingCard from './ResolvingCard.svelte';
	import PlayerChips from './PlayerChips.svelte';
	import CardPicker from './CardPicker.svelte';
	import { PLAN_SHORT, playerName, parseResolutionData } from './shared';

	interface Props {
		plan: Plan;
		plans: Plan[];
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		onPlansChanged: () => void;
		onRollCreated: (roll: DiceRoll) => void;
	}
	let { plan, plans, players, assets, currentPlayerID, onPlansChanged, onRollCreated }: Props = $props();

	const isPreparer = $derived(currentPlayerID != null && currentPlayerID === plan.preparer_id);
	const preparerName = $derived(playerName(players, plan.preparer_id));
	const planName = $derived(PLAN_SHORT[plan.plan_type] ?? plan.plan_type);
	const reason = $derived(parseResolutionData(plan).reaim?.reason ?? '');

	// ── Options (server-authoritative) ────────────────────────────────────────
	let options = $state<ReAimOption[]>([]);
	let loaded = $state(false);
	let loadError = $state('');
	// Drops a slow fetch for a plan that is no longer the one on screen.
	let token = 0;
	$effect(() => {
		const id = plan.id;
		const mine = ++token;
		loaded = false;
		loadError = '';
		getReAim(id)
			.then((info) => { if (mine === token) { options = info.options; loaded = true; } })
			.catch((e) => {
				if (mine !== token) return;
				loadError = e instanceof Error ? e.message : 'Could not load the choices.';
			});
	});

	// ── Selection ─────────────────────────────────────────────────────────────
	// One selected option at a time; the picker for each plan type narrows it.
	let pickedPlayerID = $state<number | null>(null);
	let pickedAssetID = $state<number | null>(null);
	let pickedPlanID = $state<number | null>(null);

	const picked = $derived.by((): ReAimOption | null => {
		return options.find((o) =>
			plan.plan_type === 'make_demands'
				? o.target_plan_id != null && o.target_plan_id === pickedPlanID
				: plan.plan_type === 'exchange_courtiers'
					? o.target_asset_id != null && o.target_asset_id === pickedAssetID
						&& o.target_player_id === pickedPlayerID
					: o.target_asset_id != null && o.target_asset_id === pickedAssetID,
		) ?? null;
	});

	// Exchange Courtiers: players who still have a peer to take, then their peers.
	const optionPlayers = $derived(
		players.filter((p) => options.some((o) => o.target_player_id === p.id)),
	);
	const peerChoices = $derived(
		assets.filter((a) => options.some(
			(o) => o.target_asset_id === a.id && o.target_player_id === pickedPlayerID,
		)),
	);
	// Spread Rumors: every living asset the server offered.
	const assetChoices = $derived(
		assets.filter((a) => options.some((o) => o.target_asset_id === a.id)),
	);
	// Make Demands: the offered plans, as readable cards.
	const planChoices = $derived(
		plans.filter((p) => options.some((o) => o.target_plan_id === p.id)),
	);

	let busy = $state(false);
	let error = $state('');
	async function submit() {
		if (!picked || busy) return;
		busy = true; error = '';
		try {
			const res = await reAimPlan(plan.id, picked);
			if (res.roll) onRollCreated(res.roll);
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not re-aim the plan.';
		} finally { busy = false; }
	}
</script>

<ResolvingCard {plan} {players} error={error || loadError}>
	<div class="reaim">
		<p class="reaim-lead">
			<strong>{planName}</strong> has lost its mark.
		</p>
		{#if reason}<p class="reaim-reason">{reason}</p>{/if}

		{#if !isPreparer}
			<p class="choices-note">
				Waiting for {preparerName} to choose a new target — a plan may change its
				target when the original no longer makes sense.
			</p>
		{:else if !loaded && !loadError}
			<p class="choices-note">Loading the choices…</p>
		{:else if loaded}
			<p class="choices-note">
				Choose a new target. The plan carries on from the top once you do.
			</p>

			{#if plan.plan_type === 'exchange_courtiers'}
				<div class="reaim-field">
					<span class="reaim-label">Target player</span>
					<PlayerChips
						players={optionPlayers}
						isActive={(p) => pickedPlayerID === p.id}
						onSelect={(p) => { pickedPlayerID = p.id; pickedAssetID = null; }}
					/>
				</div>
				{#if pickedPlayerID != null}
					<CardPicker
						label="Target peer"
						items={peerChoices}
						{players}
						emptyMessage="This player has no peers to exchange."
						selected={pickedAssetID}
						onSelect={(id) => (pickedAssetID = id)}
					/>
				{/if}
			{:else if plan.plan_type === 'spread_rumors'}
				<CardPicker
					label="The rumor is now about"
					items={assetChoices}
					{players}
					emptyMessage="Nothing is left to be the subject of a rumor."
					ownerLabel={(a) => `held by ${playerName(players, a.owner_id)}`}
					selected={pickedAssetID}
					onSelect={(id) => (pickedAssetID = id)}
				/>
			{:else if plan.plan_type === 'make_demands'}
				<div class="reaim-field">
					<span class="reaim-label">Demand against</span>
					<div class="choice-list">
						{#each planChoices as p (p.id)}
							<button
								type="button"
								class="choice-card"
								class:active={pickedPlanID === p.id}
								aria-pressed={pickedPlanID === p.id}
								onclick={() => (pickedPlanID = pickedPlanID === p.id ? null : p.id)}
							>
								<span class="reaim-plan-text">
									<span>{playerName(players, p.preparer_id)}'s {PLAN_SHORT[p.plan_type] ?? p.plan_type}</span>
									{#if p.preparation_notes}
										<span class="reaim-plan-notes">“{p.preparation_notes}”</span>
									{/if}
								</span>
							</button>
						{/each}
					</div>
				</div>
			{/if}

			<div class="reaim-actions">
				<button class="action-btn primary" onclick={submit} disabled={!picked || busy}>
					{busy ? '…' : 'Re-aim this plan'}
				</button>
			</div>
		{/if}
	</div>
</ResolvingCard>

<style>
	.reaim-lead { margin: 0 0 0.25rem 0; }
	.reaim-reason { margin: 0 0 0.5rem 0; font-style: italic; }
	.reaim-field { margin-top: 0.5rem; }
	.reaim-label { display: block; margin-bottom: 0.25rem; font-size: 0.9em; }
	.reaim-plan-text { display: flex; flex-direction: column; gap: 0.15rem; }
	.reaim-plan-notes { font-size: 0.85em; color: var(--color-text-faint); }
	.reaim-actions { margin-top: 0.75rem; }
</style>
