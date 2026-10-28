<!-- Festivity/HostPendingMar.svelte
  The host's pending-mar card. A guest with a make has insisted the host take a
  Mar (book p.61: "insist that the host choose one option from the mar list"),
  and it is the HOST who chooses which — so this card is where the choice is
  made, plus whatever that option needs:
    • rumor_about_you → the rumor's text;
    • disagreement    → which of the host's peers falls out;
    • accept_duels    → nothing;
    • break_self      → which marginalia on the host's main character to tear.
  Mars are settled one at a time (a legacy mar first, then the oldest insistence).

  Legacy: a game in flight across the Q13 change can hold a mar the insisting
  guest already chose (`pendingHostMars`). That card has no choice left to make
  — only the asset pick — so the option is fixed and the picker is hidden.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import { resolveHostMar, type Asset, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import { breakableAssets, playerName } from '../shared';
	import { destructionWarning } from '$lib/assetRisk';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import { MAR_OPTS } from './options';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	let { plan, players, assets, unchosenHostMars, pendingHostMars, onPlansChanged }: {
		plan: Plan;
		players: Player[];
		assets: Asset[];
		/** Guests whose insistence the host hasn't answered yet, oldest first. */
		unchosenHostMars: number[];
		/** Legacy: a mar the insisting guest chose, awaiting the host's asset pick. */
		pendingHostMars: string[];
		onPlansChanged: () => void;
	} = $props();

	const legacyOption = $derived(pendingHostMars[0] ?? null);
	// Whose insistence the card is answering (null on a legacy card).
	const insisterID = $derived(legacyOption ? null : (unchosenHostMars[0] ?? null));
	const total = $derived(pendingHostMars.length + unchosenHostMars.length);

	let picked = $state<string | null>(null);
	const current = $derived(legacyOption ?? picked);

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
		key === 'disagreement' && hostPeers.length === 0 ? 'You have no peers' : undefined;

	let rumor = $state('');
	let marginaliaID = $state<number | null>(null);
	let assetID = $state<number | null>(null);
	let busy = $state(false);
	let error = $state('');

	function reset() {
		picked = null;
		rumor = '';
		marginaliaID = null;
		assetID = null;
		error = '';
	}

	// Start fresh whenever the mar being answered changes.
	let lastKey = $state('');
	$effect(() => {
		const key = `${legacyOption ?? ''}|${insisterID ?? ''}`;
		if (key !== lastKey) {
			lastKey = key;
			reset();
		}
	});

	// Mirrors the checks in submit() so the button stays disabled until the chosen
	// option has what it needs (the server validates again).
	const ready = $derived.by(() => {
		switch (current) {
			case 'rumor_about_you': return rumor.trim().length > 0;
			case 'disagreement': return assetID != null;
			case 'break_self': return !canBreak || marginaliaID != null;
			case 'accept_duels': return true;
			default: return false;
		}
	});

	async function submit() {
		if (busy || !current || !ready) return;
		busy = true; error = '';
		try {
			const body: Parameters<typeof resolveHostMar>[1] = { mar_option: current };
			if (insisterID != null) body.insister_id = insisterID;
			if (current === 'rumor_about_you') body.rumor_text = rumor.trim();
			// For break_self with nothing left to tear, the server settles it as a
			// no-op; the placeholder id is ignored in that case.
			if (current === 'break_self') body.marginalia_id = marginaliaID ?? 0;
			if (current === 'disagreement') body.asset_id = assetID!;
			await resolveHostMar(plan.id, body);
			reset();
			onPlansChanged();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not settle the mar.';
		} finally { busy = false; }
	}
</script>

{#if total > 0}
	<div class="choices-section">
		<p class="choices-header">
			{#if insisterID != null}
				{playerName(players, insisterID)} insists you take a Mar
			{:else}
				{MAR_OPTS.find(o => o.key === legacyOption)?.label ?? 'A Mar to settle'}
			{/if}
			{#if total > 1}<span class="muted" style="font-weight:400;"> · {total} mars to settle</span>{/if}
		</p>

		{#if legacyOption == null}
			<p class="choices-note muted">You choose which one — pick one from the Mar list.</p>
			<div class="choice-list">
				{#each MAR_OPTS as o (o.key)}
					{@const reason = lockReason(o.key)}
					<button type="button" class="choice-card"
						class:active={picked === o.key} class:locked={reason != null}
						disabled={reason != null} aria-pressed={picked === o.key}
						onclick={() => { picked = picked === o.key ? null : o.key; }}>
						<span class="choice-badge">{picked === o.key ? '✓' : ''}</span>
						<span class="choice-text">
							<span class="choice-title">{o.label}</span>
							{#if o.desc}<span class="choice-desc">{o.desc}</span>{/if}
						</span>
						{#if reason}<span class="choice-lock">{reason}</span>{/if}
					</button>
				{/each}
			</div>
		{:else}
			<p class="choices-note muted">
				A guest has chosen this for you. Make the specific call below.
			</p>
		{/if}

		{#if current === 'rumor_about_you'}
			<label class="form-label">
				The rumor about you:
				<textarea rows={2} bind:value={rumor} class="form-textarea"
					maxlength={TEXT_LIMITS.LONG_TEXT} placeholder="What does the rumor say?"></textarea>
			</label>
		{:else if current === 'accept_duels'}
			<p class="choices-note muted">
				You must accept any duel challenge for the rest of the event.
			</p>
		{:else if current === 'break_self'}
			{#if canBreak}
				<CardPicker
					label="Marginalia to tear"
					items={hostMC}
					{players}
					emptyMessage="You have nothing left to break."
					marginaliaMode
					selectedMarginaliaID={marginaliaID}
					selectedAssetID={hostMC[0]?.id ?? null}
					onSelectMarginalia={(mID) => (marginaliaID = mID)}
				/>
				{#if breakWarn}<p class="res-warning">{breakWarn}</p>{/if}
			{:else}
				<p class="choices-note muted">You have nothing left to tear — you can pass the break.</p>
			{/if}
		{:else if current === 'disagreement'}
			<CardPicker
				label="Peer to set in the center"
				items={hostPeers}
				{players}
				emptyMessage="You have no peers to set in the center."
				ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
				selected={assetID}
				onSelect={(id) => (assetID = id)}
			/>
		{/if}

		{#if error}<ErrorText message={error} variant="panel" />{/if}
		{#if current}
			<button class="action-btn primary" onclick={submit} disabled={busy || !ready}>
				{busy ? '…'
					: current === 'break_self' && !canBreak ? 'Pass the break'
					: 'Take this Mar'}
			</button>
		{/if}
	</div>
{/if}
