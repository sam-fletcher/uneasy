<!-- ClosingStage.svelte
  "The Stage is Set" — the prologue's closing step (game.prologue_ranking_step
  === 'closing'; adr/PROLOGUE_CLOSING_STAGE_PLAN.md). Checklist of hard items
  (must complete before Ready) and soft nudges, plus the ready roster/toggle
  that drives the all-ready auto-advance into the Main Event.

  `laws`/`rumors`/`onOpenLaws`/`onOpenRumors` are plumbed through but unused
  until Session 3 adds the recap section to this component.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { createExtraPeer, updateAsset, setClosingReady } from '$lib/api';
	import type {
		Player,
		Asset,
		PrologueSheet,
		PrologueClaim,
		ExtraPeer,
		ClosingReady,
		Law,
		Rumor,
	} from '$lib/api';
	import {
		findMainCharacter,
		isMcNamed,
		needsExtraPeer,
		findExtraPeer,
		unclaimedTitles,
		readyBlockedReason,
		isReady,
		myAtRiskCount,
	} from '$lib/prologue/closing';
	import { TEXT_LIMITS } from '$lib/textLimits';

	interface Props {
		gameID: string;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		closingReady: ClosingReady[];
		extraPeers: ExtraPeer[];
		sheets: PrologueSheet[];
		claims: PrologueClaim[];
		laws?: Law[];
		rumors?: Rumor[];
		onReload: () => void;
		onResync?: () => void;
		onOpenTones?: () => void;
		onOpenRetinue?: () => void;
		onOpenLaws?: () => void;
		onOpenRumors?: () => void;
	}

	let {
		gameID,
		players,
		assets = $bindable(),
		currentPlayerID,
		closingReady,
		extraPeers,
		sheets,
		claims,
		onReload,
		onResync,
		onOpenTones,
		onOpenRetinue,
	}: Props = $props();

	let error = $state('');

	// ── Name your main character (hard) ──────────────────────────────────────
	const myMainCharacter = $derived(findMainCharacter(assets, currentPlayerID));
	const mcNamed = $derived(isMcNamed(myMainCharacter));

	let mcRenameDraft = $state('');
	let savingMcRename = $state(false);
	async function submitMcRename() {
		const text = mcRenameDraft.trim();
		if (!myMainCharacter || !text || savingMcRename) return;
		savingMcRename = true;
		error = '';
		try {
			await updateAsset(myMainCharacter.id, { name: text });
			mcRenameDraft = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not rename your main character.';
		} finally {
			savingMcRename = false;
		}
	}

	// ── Create an extra peer (hard, ≤3p) ─────────────────────────────────────
	const needsPeer = $derived(needsExtraPeer(players.length));
	const myExtraPeer = $derived(findExtraPeer(extraPeers, currentPlayerID));
	const titlesSheet = $derived(sheets.find((s) => s.type === 'titles'));
	const openTitles = $derived(unclaimedTitles(titlesSheet, claims, extraPeers));

	let extraPeerName = $state('');
	let extraPeerText = $state('');
	let creatingExtra = $state(false);
	async function submitExtraPeer() {
		if (!extraPeerName || !extraPeerText.trim() || creatingExtra) return;
		creatingExtra = true;
		error = '';
		try {
			const result = await createExtraPeer(gameID, extraPeerName, extraPeerText.trim());
			assets = [...assets, result.asset];
			extraPeerName = '';
			extraPeerText = '';
			onReload();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not create extra peer.';
			onResync?.();
			onReload();
		} finally {
			creatingExtra = false;
		}
	}

	// ── Shore up at-risk assets (soft) ────────────────────────────────────────
	const riskCount = $derived(myAtRiskCount(assets, currentPlayerID));

	// ── Ready roster + toggle ────────────────────────────────────────────────
	const blockedReason = $derived(readyBlockedReason(mcNamed, players.length, myExtraPeer != null));
	const myReady = $derived(isReady(closingReady, currentPlayerID));

	let savingReady = $state(false);
	async function toggleReady() {
		if (savingReady || (blockedReason && !myReady)) return;
		savingReady = true;
		error = '';
		try {
			await setClosingReady(gameID, !myReady);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update readiness.';
			onResync?.();
			onReload();
		} finally {
			savingReady = false;
		}
	}
</script>

<div class="closing-stage">
	{#if error}
		<p class="error-text">{error}</p>
	{/if}

	<h2 class="closing-title">The Stage is Set</h2>
	<p class="closing-lede">
		The prologue draws to a close. Before the Main Event begins, put your house in order.
	</p>

	<div class="checklist">
		<section class="check-card" class:done={mcNamed}>
			<div class="check-head">
				<span class="check-mark" aria-hidden="true">{mcNamed ? '✓' : '○'}</span>
				<h3 class="check-title">Name your main character</h3>
			</div>
			{#if mcNamed}
				<p class="check-body">{myMainCharacter?.name}</p>
			{:else}
				<p class="check-body">Currently: {myMainCharacter?.name ?? '—'}</p>
				<div class="check-form">
					<input
						type="text"
						bind:value={mcRenameDraft}
						placeholder="Name your main character"
						maxlength={TEXT_LIMITS.NAME}
						aria-label="New main character name"
					/>
					<button
						class="action-btn secondary small"
						onclick={submitMcRename}
						disabled={!mcRenameDraft.trim() || savingMcRename}
					>
						{savingMcRename ? '…' : 'Save name'}
					</button>
				</div>
			{/if}
		</section>

		{#if needsPeer}
			<section class="check-card" class:done={myExtraPeer != null}>
				<div class="check-head">
					<span class="check-mark" aria-hidden="true">{myExtraPeer ? '✓' : '○'}</span>
					<h3 class="check-title">Create an extra peer</h3>
				</div>
				{#if myExtraPeer}
					<p class="check-body">You created your extra peer: {myExtraPeer.title_name}.</p>
				{:else}
					<div class="extra-form">
						<div class="extra-title">
							<span class="extra-title-label">Title:</span>
							{#if openTitles.length === 0}
								<p class="muted-text small" style="margin:0;">No titles remain.</p>
							{:else}
								<div class="title-chip-row">
									{#each openTitles as t}
										<button
											type="button"
											class="title-chip"
											class:active={extraPeerName === t.name}
											onclick={() => (extraPeerName = extraPeerName === t.name ? '' : t.name)}
										>{t.name}</button>
									{/each}
								</div>
							{/if}
						</div>
					</div>
					<label>
						Peer name:
						<input
							type="text"
							bind:value={extraPeerText}
							class="peer-input"
							placeholder="Name your peer"
							maxlength={TEXT_LIMITS.NAME}
						/>
					</label>
					<button
						class="action-btn secondary small"
						onclick={submitExtraPeer}
						disabled={!extraPeerName || !extraPeerText.trim() || creatingExtra}
					>
						{creatingExtra ? '…' : 'Create peer'}
					</button>
				{/if}
			</section>
		{/if}

		{#if riskCount > 0}
			<section class="check-card soft at-risk">
				<div class="check-head">
					<h3 class="check-title-soft">Shore up at-risk assets</h3>
				</div>
				<p class="check-body">
					{riskCount} of your assets {riskCount === 1 ? 'is' : 'are'} one tear from destruction.
				</p>
				<button class="action-btn secondary small" onclick={onOpenRetinue}>Open Retinue</button>
			</section>
		{/if}

		<section class="check-card soft tones">
			<div class="check-head">
				<h3 class="check-title-soft">Tones — last chance</h3>
			</div>
			<p class="check-body">Tones lock when the Main Event begins.</p>
			<button class="action-btn secondary small" onclick={onOpenTones}>Open Tones</button>
		</section>
	</div>

	<h3 class="ready-heading">Ready roster</h3>
	<ul class="ready-roster">
		{#each players as p}
			{@const ready = isReady(closingReady, p.id)}
			<li class:ready>
				<span class="ready-roster-name">{p.display_name}</span>
				{#if ready}
					<span class="ready-roster-status done">✓ ready</span>
				{:else}
					<span class="ready-roster-status pending">waiting…</span>
				{/if}
			</li>
		{/each}
	</ul>

	<button
		class="action-btn primary done-btn"
		class:active={myReady}
		disabled={savingReady || (!myReady && blockedReason != null)}
		title={!myReady ? (blockedReason ?? undefined) : undefined}
		onclick={toggleReady}
	>
		{savingReady ? '…' : myReady ? 'Ready ✓ (tap to undo)' : "I'm ready"}
	</button>
	{#if !myReady && blockedReason}
		<p class="muted-text small">{blockedReason}</p>
	{/if}
</div>

<style>
	.closing-stage {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.closing-title {
		margin: 0;
		color: var(--color-text);
		font-size: 1.05rem;
		line-height: 1.45;
	}
	.closing-lede {
		margin: 0;
		color: var(--color-text-secondary);
		font-size: 0.9rem;
		line-height: 1.4;
	}

	.checklist {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	.check-card {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		padding: 0.65rem 0.7rem;
	}
	/* Hard items (name your MC / extra peer): default border until complete,
	   then the same success-green the declare-track done-btn uses. */
	.check-card.done { border-color: var(--color-chip-green-border); }

	/* Soft items: red for the at-risk nudge (style guide — "red is danger,
	   which includes the at-risk game state"), orange for Tones (the one
	   "careful now, this locks soon" family). */
	.check-card.soft.at-risk {
		border-color: var(--color-danger-muted);
		background: color-mix(in srgb, var(--color-danger-muted) 12%, var(--color-surface-sunken));
	}
	.check-card.soft.tones {
		border-color: var(--color-warning-border);
		background: color-mix(in srgb, var(--color-warning-border) 12%, var(--color-surface-sunken));
	}

	.check-head {
		display: flex;
		align-items: baseline;
		gap: 0.45rem;
	}
	.check-mark {
		color: var(--color-text-muted);
		font-size: 0.95rem;
		line-height: 1;
	}
	.check-card.done .check-mark { color: var(--color-success); }
	.check-title {
		margin: 0;
		color: var(--color-accent);
		font-size: 0.95rem;
	}
	.check-title-soft {
		margin: 0;
		color: var(--color-text);
		font-size: 0.95rem;
	}
	.check-body {
		margin: 0;
		color: var(--color-text-secondary);
		font-size: 0.85rem;
		line-height: 1.4;
	}
	.check-form {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		align-items: end;
	}
	.check-form input {
		flex: 1 1 12rem;
		min-width: 0;
	}

	.ready-heading { color: var(--color-accent); font-size: 1rem; margin: 0; }

	.ready-roster {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		max-width: 24rem;
	}
	.ready-roster li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
		padding: 0.35rem 0.5rem;
		min-height: 44px;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-surface-2);
		border-radius: 4px;
		font-size: 0.85rem;
	}
	.ready-roster li.ready { border-color: var(--color-chip-green-border); }
	.ready-roster-name { color: var(--color-text); }
	.ready-roster-status.done { color: var(--color-success); font-size: 0.8rem; }
	.ready-roster-status.pending { color: var(--color-text-faint); font-size: 0.8rem; font-style: italic; }

	.done-btn.active { background: var(--color-success); }

	.extra-form {
		display: flex;
		gap: 0.6rem;
		align-items: end;
	}
	.peer-input {
		max-width: 20rem;
		margin-top: 0.25rem;
	}
	.extra-title {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}
	.extra-title-label {
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}
	.title-chip-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
	}
	.title-chip {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		padding: 0.35rem 0.85rem;
		border-radius: 999px;
		border: 1px solid var(--color-neutral);
		background: var(--color-surface-2);
		color: var(--color-text);
		font-size: 0.9rem;
		cursor: pointer;
	}
	.title-chip.active {
		border-color: var(--color-accent);
		background: var(--color-chip-gold-bg);
	}
	.title-chip:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}
</style>
