<!--
  Retinue view for any player. Shows the player's assets as tiles with a
  2x2 marginalia sub-grid. The tile is the primary interactive surface;
  owner-only mutations (add/edit marginalia, rename, etc.) are hooked
  directly onto the tile parts.
-->
<script lang="ts">
	import { addMarginalia, updateMarginalia, updateAsset, leverageAsset, writeSecret, getAssetSuggestions } from '$lib/api';
	import type { Asset, Player, PresenceMember, Marginalium, Secret, Ranking } from '$lib/api';
	import SuggestionPicker from './SuggestionPicker.svelte';

	let {
		playerId,
		players,
		members,
		assets,
		secrets = [],
		rankings = [],
		viewerPlayerId,
		leverageActive = false,
		onSecretsChanged,
	}: {
		playerId: number;
		players: Player[];
		members: PresenceMember[];
		assets: Asset[];
		secrets?: Secret[];
		rankings?: Ranking[];
		viewerPlayerId: number | null;
		/** True when there's an unresolved active dice roll — leverage is actionable. */
		leverageActive?: boolean;
		/** Called after the viewer writes a secret so the parent can refetch. */
		onSecretsChanged?: () => void;
	} = $props();

	function rankFor(category: 'power' | 'knowledge' | 'esteem'): number | null {
		const r = rankings.find(r => r.category === category && r.player_id === playerId);
		return r ? r.rank : null;
	}
	const playerRanks = $derived({
		power: rankFor('power'),
		knowledge: rankFor('knowledge'),
		esteem: rankFor('esteem'),
	});
	// Status is the inverse of rank (status = 6 − rank); it's the difficulty
	// others face when targeting this player, where rank is their own difficulty
	// when acting. The rules use both, so we surface both.
	function statusFromRank(rank: number | null): number | null {
		return rank == null ? null : 6 - rank;
	}
	const playerStatuses = $derived({
		power: statusFromRank(playerRanks.power),
		knowledge: statusFromRank(playerRanks.knowledge),
		esteem: statusFromRank(playerRanks.esteem),
	});
	const hasRanks = $derived(
		playerRanks.power != null || playerRanks.knowledge != null || playerRanks.esteem != null
	);

	const player = $derived(players.find(p => p.id === playerId) ?? null);
	const presence = $derived(members.find(m => m.id === playerId) ?? null);
	const isSelf = $derived(viewerPlayerId === playerId);

	const ownedAssets = $derived(
		assets.filter(a => a.owner_id === playerId && !a.is_destroyed)
	);

	const assetTypeLabels: Record<Asset['asset_type'], string> = {
		peer: 'Peer',
		holding: 'Holding',
		artifact: 'Artifact',
		resource: 'Resource',
	};

	// Build a 4-slot marginalia array (filled by 1-indexed position, padded with null).
	function slotsFor(asset: Asset): (Marginalium | null)[] {
		const slots: (Marginalium | null)[] = [null, null, null, null];
		for (const m of asset.marginalia) {
			if (m.position >= 1 && m.position <= 4) slots[m.position - 1] = m;
		}
		return slots;
	}

	// ── Add / edit marginalia (owner only) ──────────────────────────────────
	// When set, the matching asset's marginalia grid is replaced by an editor.
	// editingPosition === null → adding new; otherwise editing that position.
	let editingAssetId = $state<number | null>(null);
	let editingPosition = $state<number | null>(null);
	let draftText = $state('');
	let saving = $state(false);
	let editError = $state<string | null>(null);

	// Type-keyed marginalia suggestions, fetched when the add editor opens.
	let addSuggestions = $state<string[]>([]);
	let suggestionsLoading = $state(false);

	async function startAdd(asset: Asset) {
		editingAssetId = asset.id;
		editingPosition = null;
		draftText = '';
		editError = null;
		addSuggestions = [];
		suggestionsLoading = true;
		try {
			const res = await getAssetSuggestions(asset.game_id, asset.asset_type, 'marginalia');
			// Ignore if the editor was closed or moved on while we were loading.
			if (editingAssetId === asset.id && editingPosition == null) {
				addSuggestions = res.suggestions;
			}
		} catch {
			addSuggestions = [];
		} finally {
			if (editingAssetId === asset.id) suggestionsLoading = false;
		}
	}

	function startEdit(asset: Asset, m: Marginalium) {
		editingAssetId = asset.id;
		editingPosition = m.position;
		draftText = m.text;
		editError = null;
	}

	function cancelEdit() {
		editingAssetId = null;
		editingPosition = null;
		draftText = '';
		editError = null;
	}

	async function saveEdit() {
		const assetId = editingAssetId;
		const text = draftText.trim();
		if (assetId == null || !text || saving) return;
		saving = true;
		editError = null;
		try {
			if (editingPosition == null) {
				await addMarginalia(assetId, text);
			} else {
				await updateMarginalia(assetId, editingPosition, text);
			}
			// WS events (MarginaliaAdded / MarginaliaUpdated) update the asset prop.
			cancelEdit();
		} catch (e) {
			editError = e instanceof Error ? e.message : 'Could not save.';
			saving = false;
			return;
		}
		saving = false;
	}

	// ── Rename asset (owner only) ───────────────────────────────────────────
	let renamingAssetId = $state<number | null>(null);
	let renameDraft = $state('');
	let renameSaving = $state(false);
	let renameError = $state<string | null>(null);

	function startRename(asset: Asset) {
		renamingAssetId = asset.id;
		renameDraft = asset.name;
		renameError = null;
	}

	function cancelRename() {
		renamingAssetId = null;
		renameDraft = '';
		renameError = null;
	}

	async function saveRename(asset: Asset) {
		const text = renameDraft.trim();
		if (renamingAssetId !== asset.id || !text || renameSaving) return;
		if (text === asset.name) { cancelRename(); return; }
		renameSaving = true;
		renameError = null;
		try {
			await updateAsset(asset.id, { name: text });
			cancelRename();
		} catch (e) {
			renameError = e instanceof Error ? e.message : 'Could not rename.';
		} finally {
			renameSaving = false;
		}
	}

	// ── Set as main character (owner, peers only) ──────────────────────────
	// Two-step flow: tap ☆ on a non-main peer → if old MC has untorn marginalia,
	// that tile shows a picker; tapping a marginalium fires the atomic swap.
	let mcSwapTo = $state<number | null>(null); // new MC asset id (target of swap)
	let mcSwapSaving = $state(false);
	let mcSwapError = $state<string | null>(null);

	const currentMC = $derived(ownedAssets.find(a => a.is_main_character) ?? null);

	function cancelMcSwap() {
		mcSwapTo = null;
		mcSwapSaving = false;
		mcSwapError = null;
	}

	async function startMcSwap(target: Asset) {
		if (mcSwapSaving) return;
		mcSwapError = null;
		// Edge case: no current MC, or current MC has no untorn marginalia → no
		// picker needed, fire directly.
		const old = currentMC;
		const untorn = old ? old.marginalia.filter(m => !m.is_torn) : [];
		if (!old || untorn.length === 0) {
			mcSwapSaving = true;
			try {
				await updateAsset(target.id, { is_main_character: true });
				cancelMcSwap();
			} catch (e) {
				mcSwapError = e instanceof Error ? e.message : 'Could not set main character.';
				mcSwapSaving = false;
			}
			return;
		}
		mcSwapTo = target.id;
	}

	async function confirmMcSwap(tearPosition: number) {
		if (mcSwapTo == null || mcSwapSaving) return;
		mcSwapSaving = true;
		mcSwapError = null;
		try {
			await updateAsset(mcSwapTo, { is_main_character: true, tear_position: tearPosition });
			cancelMcSwap();
		} catch (e) {
			mcSwapError = e instanceof Error ? e.message : 'Could not swap main character.';
			mcSwapSaving = false;
		}
	}

	// ── Secrets (any viewer, any asset) ─────────────────────────────────────
	let secretsOpenForAssetId = $state<number | null>(null);
	let newSecretText = $state('');
	let secretSaving = $state(false);
	let secretError = $state<string | null>(null);

	function secretsForAsset(assetId: number): Secret[] {
		return secrets.filter(s => s.asset_id === assetId);
	}

	function toggleSecrets(assetId: number) {
		if (secretsOpenForAssetId === assetId) {
			secretsOpenForAssetId = null;
			newSecretText = '';
			secretError = null;
		} else {
			secretsOpenForAssetId = assetId;
			newSecretText = '';
			secretError = null;
		}
	}

	async function saveSecret(asset: Asset) {
		const text = newSecretText.trim();
		if (!text || secretSaving) return;
		secretSaving = true;
		secretError = null;
		try {
			await writeSecret(asset.id, text);
			newSecretText = '';
			onSecretsChanged?.();
		} catch (e) {
			secretError = e instanceof Error ? e.message : 'Could not write secret.';
		} finally {
			secretSaving = false;
		}
	}

	// ── Leverage (owner only, when a roll is active) ────────────────────────
	let leveragingId = $state<number | null>(null);
	let leverageError = $state<string | null>(null);

	async function doLeverage(asset: Asset) {
		if (leveragingId != null) return;
		leveragingId = asset.id;
		leverageError = null;
		try {
			await leverageAsset(asset.id);
			// AssetLeveraged WS event updates the prop.
		} catch (e) {
			leverageError = e instanceof Error ? e.message : 'Could not leverage.';
		} finally {
			leveragingId = null;
		}
	}

	// Reset edit state when the player being viewed changes.
	$effect(() => {
		void playerId;
		cancelEdit();
		cancelRename();
		cancelMcSwap();
		leverageError = null;
	});

	// Action: focus the textarea when it mounts (replaces autofocus attribute,
	// which Svelte's a11y rules discourage).
	function focusOnMount(node: HTMLElement) {
		node.focus();
	}
</script>

<div class="retinue-view">
	{#if player}
		<header class="retinue-header">
			<h2>{isSelf ? 'Your Retinue' : `${player.display_name}'s Retinue`}</h2>
			<div class="meta">
				<span class="dot" class:online={presence?.online}></span>
				<span class="status">{presence?.online ? 'online' : 'offline'}</span>
				{#if player.is_facilitator}
					<span class="tag">facilitator</span>
				{/if}
			</div>
		</header>

		{#if hasRanks}
			<section class="rank-strip" aria-label="Track rankings">
				{#each [
					{ label: 'Power', rank: playerRanks.power, status: playerStatuses.power },
					{ label: 'Knowledge', rank: playerRanks.knowledge, status: playerStatuses.knowledge },
					{ label: 'Esteem', rank: playerRanks.esteem, status: playerStatuses.esteem },
				] as track (track.label)}
					<div class="rank-cell">
						<span class="rank-label">{track.label}</span>
						<div class="rank-pair">
							<span class="rank-stat">
								<span class="rank-num">{track.rank ?? '—'}</span>
								<span class="rank-sublabel">Rank</span>
							</span>
							<span class="rank-stat">
								<span class="rank-num">{track.status ?? '—'}</span>
								<span class="rank-sublabel">Status</span>
							</span>
						</div>
					</div>
				{/each}
			</section>
		{/if}

		{#if ownedAssets.length === 0}
			<p class="empty">No assets yet.</p>
		{:else}
			<ul class="asset-grid">
				{#each ownedAssets as asset (asset.id)}
					<li
						class="asset-tile"
						class:main-char={asset.is_main_character}
						class:leveraged={asset.is_leveraged}
					>
						<div class="tile-head">
							{#if renamingAssetId === asset.id}
								<input
									class="rename-input"
									type="text"
									bind:value={renameDraft}
									disabled={renameSaving}
									maxlength={80}
									onblur={() => saveRename(asset)}
									onkeydown={(e) => {
										if (e.key === 'Escape') { e.preventDefault(); cancelRename(); }
										else if (e.key === 'Enter') { e.preventDefault(); saveRename(asset); }
									}}
									use:focusOnMount
								/>
							{:else if isSelf}
								<button type="button" class="asset-name editable" onclick={() => startRename(asset)} aria-label={`Rename ${asset.name}`}>
									{asset.name}
									{#if asset.is_main_character}<span class="main-badge">★</span>{/if}
								</button>
							{:else}
								<span class="asset-name">
									{asset.name}
									{#if asset.is_main_character}<span class="main-badge">★</span>{/if}
								</span>
							{/if}
							{#if isSelf && asset.asset_type === 'peer' && !asset.is_main_character && renamingAssetId !== asset.id && mcSwapTo == null}
								<button
									type="button"
									class="mc-toggle"
									onclick={() => startMcSwap(asset)}
									disabled={mcSwapSaving}
									aria-label={`Make ${asset.name} the main character`}
									title="Make main character"
								>☆</button>
							{/if}
							{#if isSelf && leverageActive && !asset.is_leveraged && renamingAssetId !== asset.id && mcSwapTo == null}
								<button
									type="button"
									class="lev-btn"
									onclick={() => doLeverage(asset)}
									disabled={leveragingId === asset.id}
									title="Commit this asset to the active roll"
								>{leveragingId === asset.id ? '…' : 'Leverage'}</button>
							{/if}
							<button
								type="button"
								class="secrets-btn"
								class:has-secrets={secretsForAsset(asset.id).length > 0}
								class:active={secretsOpenForAssetId === asset.id}
								onclick={() => toggleSecrets(asset.id)}
								aria-label={secretsForAsset(asset.id).length > 0 ? `View ${secretsForAsset(asset.id).length} secret(s)` : 'Write a secret'}
								title="Secrets"
							>
								<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
									<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
									<circle cx="12" cy="12" r="3" />
								</svg>
								{#if secretsForAsset(asset.id).length > 0}<span class="secrets-badge">{secretsForAsset(asset.id).length}</span>{/if}
							</button>
							<span class="asset-type">{assetTypeLabels[asset.asset_type]}</span>
						</div>
						{#if isSelf && leverageError && leveragingId == null}
							<p class="m-editor-error">{leverageError}</p>
						{/if}
						{#if renamingAssetId === asset.id && renameError}
							<p class="m-editor-error">{renameError}</p>
						{/if}
						{#if mcSwapTo != null && currentMC && asset.id === currentMC.id}
							<div class="mc-picker">
								<p class="m-editor-label">
									Pick a marginalium to break on {asset.name}
								</p>
								<div class="mc-picker-list">
									{#each slotsFor(asset) as slot, i (i)}
										{#if slot && !slot.is_torn}
											<button type="button" class="mc-picker-item" disabled={mcSwapSaving} onclick={() => confirmMcSwap(slot.position)}>
												<span class="m-pos">{i + 1}.</span>
												<span class="m-tile-text">{slot.text}</span>
											</button>
										{/if}
									{/each}
								</div>
								{#if mcSwapError}<p class="m-editor-error">{mcSwapError}</p>{/if}
								<div class="m-editor-actions">
									<button type="button" class="m-btn secondary" onclick={cancelMcSwap} disabled={mcSwapSaving}>Cancel</button>
								</div>
							</div>
						{:else if editingAssetId === asset.id}
							<div class="m-editor">
								<p class="m-editor-label">
									{editingPosition == null ? 'Adding marginalium' : `Editing marginalium ${editingPosition} of 4`}
								</p>
								{#if editingPosition == null}
									<!-- Add: offer type-keyed examples, or write your own. -->
									<SuggestionPicker
										suggestions={addSuggestions}
										bind:value={draftText}
										loading={suggestionsLoading}
										customPlaceholder="Write a marginalium…"
										multiline
										disabled={saving}
									/>
								{:else}
									<textarea
										class="m-editor-input"
										placeholder="Write a marginalium…"
										bind:value={draftText}
										disabled={saving}
										rows={3}
										maxlength={280}
										onkeydown={(e) => {
											if (e.key === 'Escape') { e.preventDefault(); cancelEdit(); }
											else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); saveEdit(); }
										}}
										use:focusOnMount
									></textarea>
								{/if}
								{#if editError}<p class="m-editor-error">{editError}</p>{/if}
								<div class="m-editor-actions">
									<button type="button" class="m-btn secondary" onclick={cancelEdit} disabled={saving}>Cancel</button>
									<button type="button" class="m-btn primary" onclick={saveEdit} disabled={saving || !draftText.trim()}>
										{saving ? '…' : 'Save'}
									</button>
								</div>
							</div>
						{:else}
							<div class="m-grid">
								{#each slotsFor(asset) as slot, i (i)}
									{#if slot}
										{#if isSelf && !slot.is_torn}
											<button type="button" class="m-tile filled" onclick={() => startEdit(asset, slot)} aria-label={`Edit marginalium ${slot.position}`}>
												<span class="m-tile-text">{slot.text}</span>
											</button>
										{:else}
											<div class="m-tile" class:torn={slot.is_torn}>
												<span class="m-tile-text">{slot.text}</span>
											</div>
										{/if}
									{:else if isSelf}
										<button type="button" class="m-tile empty add" onclick={() => startAdd(asset)} aria-label="Add marginalia">
											<span class="add-plus" aria-hidden="true">+</span>
										</button>
									{:else}
										<div class="m-tile empty" aria-label="empty marginalia slot"></div>
									{/if}
								{/each}
							</div>
						{/if}
						{#if secretsOpenForAssetId === asset.id}
							<div class="secrets-panel">
								<p class="m-editor-label">Secrets</p>
								{#if secretsForAsset(asset.id).length === 0}
									<p class="empty small">No secrets visible to you.</p>
								{:else}
									<ul class="secrets-list">
										{#each secretsForAsset(asset.id) as s (s.id)}
											<li class="secret-row" class:authored={s.author_id === viewerPlayerId}>
												<span class="secret-text">{s.text}</span>
												{#if s.author_id === viewerPlayerId}<span class="secret-mine">yours</span>{/if}
											</li>
										{/each}
									</ul>
								{/if}
								{#if isSelf}
									<textarea
										class="m-editor-input"
										placeholder="Write a secret on this asset…"
										bind:value={newSecretText}
										disabled={secretSaving}
										rows={2}
										maxlength={500}
										onkeydown={(e) => {
											if (e.key === 'Escape') { e.preventDefault(); toggleSecrets(asset.id); }
											else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); saveSecret(asset); }
										}}
									></textarea>
									{#if secretError}<p class="m-editor-error">{secretError}</p>{/if}
								{/if}
								<div class="m-editor-actions">
									<button type="button" class="m-btn secondary" onclick={() => toggleSecrets(asset.id)} disabled={secretSaving}>Close</button>
									{#if isSelf}
										<button type="button" class="m-btn primary" onclick={() => saveSecret(asset)} disabled={secretSaving || !newSecretText.trim()}>
											{secretSaving ? '…' : 'Add secret'}
										</button>
									{/if}
								</div>
							</div>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	{:else}
		<p class="empty">Player not found.</p>
	{/if}
</div>

<style>
	.retinue-view {
		display: flex;
		flex-direction: column;
		gap: 0.85rem;
	}

	.retinue-header h2 {
		color: var(--color-accent);
		font-size: 1.1rem;
		margin: 0 0 0.3rem;
	}

	.rank-strip {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.4rem;
		background: #161614;
		border: 1px solid var(--color-border-subtle);
		border-radius: 8px;
		padding: 0.5rem 0.6rem;
	}
	.rank-cell {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.15rem;
	}
	.rank-label {
		font-size: 0.7rem;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	.rank-pair {
		display: flex;
		gap: 0.7rem;
	}
	.rank-stat {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.05rem;
	}
	.rank-num {
		font-size: 1.1rem;
		font-weight: 600;
		color: var(--color-text);
		font-variant-numeric: tabular-nums;
		line-height: 1.1;
	}
	.rank-sublabel {
		font-size: 0.7rem;
		color: var(--color-text-faint);
	}

	.meta {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.8rem;
		color: var(--color-text-muted);
	}

	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: #555;
	}
	.dot.online { background: var(--color-success); }

	.tag {
		font-size: 0.7rem;
		background: var(--color-border-warm);
		color: var(--color-accent);
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.empty {
		color: var(--color-text-faint);
		font-size: 0.9rem;
		font-style: italic;
		margin: 0;
	}

	/* ── Asset tiles ─────────────────────────────────────────────────────── */

	.asset-grid {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	.asset-tile {
		background: #242420;
		border: 1px solid var(--color-border-strong);
		border-radius: 8px;
		padding: 0.6rem 0.7rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.asset-tile.main-char { border-color: var(--color-accent); }
	.asset-tile.leveraged { border-color: var(--color-info); opacity: 0.78; }

	.tile-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 0.5rem;
	}

	.asset-name {
		font-weight: 600;
		font-size: 0.95rem;
		color: var(--color-text);
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	button.asset-name.editable {
		background: none;
		border: 1px solid transparent;
		padding: 0.15rem 0.35rem;
		margin: -0.15rem -0.35rem;
		border-radius: 4px;
		font-family: inherit;
		text-align: left;
		cursor: pointer;
		max-width: 100%;
	}
	button.asset-name.editable:hover { border-color: #5a5a52; background: #232320; }
	button.asset-name.editable:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	.rename-input {
		flex: 1;
		min-width: 0;
		font-family: inherit;
		font-size: 0.95rem;
		font-weight: 600;
		color: var(--color-text);
		background: #1d1d1a;
		border: 1px solid #5a5a52;
		border-radius: 4px;
		padding: 0.25rem 0.4rem;
	}
	.rename-input:focus { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	/* "Make main character" toggle (☆ on non-main peer) */
	.mc-toggle {
		flex-shrink: 0;
		min-width: 32px;
		height: 32px;
		padding: 0 0.4rem;
		background: none;
		border: 1px solid transparent;
		border-radius: 4px;
		color: #8a7a52;
		font-size: 1.1rem;
		line-height: 1;
		cursor: pointer;
	}
	.mc-toggle:hover { color: var(--color-accent); border-color: #5a4d2c; background: #2a2418; }
	.mc-toggle:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
	.mc-toggle:disabled { opacity: 0.45; cursor: not-allowed; }

	/* Leverage button (visible during an active roll, on non-leveraged owner assets) */
	.lev-btn {
		flex-shrink: 0;
		min-height: 32px;
		padding: 0.25rem 0.6rem;
		background: #1f3045;
		color: #8fb6df;
		border: 1px solid #34587a;
		border-radius: 4px;
		font-family: inherit;
		font-size: 0.72rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		cursor: pointer;
	}
	.lev-btn:hover { background: #243a55; color: #b1cdec; }
	.lev-btn:focus-visible { outline: 2px solid var(--color-info); outline-offset: 1px; }
	.lev-btn:disabled { opacity: 0.5; cursor: not-allowed; }

	/* Secrets eye button + count badge */
	.secrets-btn {
		position: relative;
		flex-shrink: 0;
		min-width: 36px;
		height: 32px;
		padding: 0 0.4rem;
		background: none;
		border: 1px solid transparent;
		border-radius: 4px;
		color: var(--color-text-faint);
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}
	.secrets-btn:hover { color: var(--color-accent); border-color: #5a5a52; background: #232320; }
	.secrets-btn:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
	.secrets-btn.has-secrets { color: var(--color-accent); }
	.secrets-btn.active { color: var(--color-text); border-color: #5a5a52; background: #2a2a26; }

	.secrets-badge {
		position: absolute;
		top: -4px;
		right: -4px;
		min-width: 16px;
		height: 16px;
		padding: 0 4px;
		background: var(--color-accent);
		color: var(--color-bg);
		border-radius: 8px;
		font-size: 0.65rem;
		font-weight: 700;
		line-height: 16px;
		text-align: center;
	}

	/* Secrets panel (expanded under the marginalia area) */
	.secrets-panel {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		padding-top: 0.5rem;
		border-top: 1px solid var(--color-border);
	}

	.secrets-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.secret-row {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 0.5rem;
		padding: 0.4rem 0.55rem;
		background: #1d1d1a;
		border: 1px solid #383530;
		border-radius: 5px;
		font-size: 0.85rem;
		line-height: 1.4;
		color: #cfcabd;
	}
	.secret-row.authored { border-left: 2px solid var(--color-accent); }

	.secret-text { flex: 1; word-break: break-word; }
	.secret-mine {
		flex-shrink: 0;
		font-size: 0.65rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-accent);
	}

	.empty.small { font-size: 0.82rem; padding: 0.3rem 0; }

	/* Pick-a-marginalium-to-break picker (replaces grid on old MC during swap) */
	.mc-picker {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.mc-picker-list {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.mc-picker-item {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
		text-align: left;
		min-height: 44px;
		padding: 0.5rem 0.6rem;
		background: #1d1d1a;
		border: 1px solid #5a3d3d;
		border-radius: 5px;
		font-family: inherit;
		font-size: 0.85rem;
		color: #cfcabd;
		cursor: pointer;
	}
	.mc-picker-item:hover { background: #261b1b; border-color: #b35454; color: var(--color-danger); }
	.mc-picker-item:focus-visible { outline: 2px solid #b35454; outline-offset: 1px; }
	.mc-picker-item:disabled { opacity: 0.5; cursor: not-allowed; }

	.mc-picker-item .m-pos {
		flex-shrink: 0;
		font-weight: 600;
		color: var(--color-accent);
	}

	.main-badge {
		font-size: 0.7rem;
		background: #4a3010;
		color: #e8c080;
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		flex-shrink: 0;
	}

	.asset-type {
		font-size: 0.7rem;
		background: var(--color-border-warm);
		color: var(--color-accent);
		padding: 0.1rem 0.4rem;
		border-radius: 3px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		flex-shrink: 0;
	}

	/* ── Marginalia 2×2 grid ─────────────────────────────────────────────── */

	.m-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.35rem;
	}

	.m-tile {
		min-height: 44px;
		padding: 0.35rem 0.45rem;
		background: #1d1d1a;
		border: 1px solid #383530;
		border-radius: 5px;
		font-size: 0.78rem;
		line-height: 1.25;
		color: #cfcabd;
		display: flex;
		align-items: center;
		overflow: hidden;
	}
	.m-tile.empty {
		background: transparent;
		border: 1px dashed #3a3a36;
	}
	.m-tile.torn {
		opacity: 0.45;
		text-decoration: line-through;
	}

	.m-tile-text {
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
		word-break: break-word;
	}

	/* "+" add affordance on owner empty slots */
	.m-tile.empty.add {
		justify-content: center;
		color: #6a6a64;
		cursor: pointer;
		font-family: inherit;
		font-size: inherit;
	}
	.m-tile.empty.add:hover { color: var(--color-accent); border-color: #5a5a52; }
	.m-tile.empty.add:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
	.add-plus { font-size: 1.4rem; line-height: 1; }

	/* Owner edit affordance on filled (untorn) slots */
	.m-tile.filled {
		text-align: left;
		font-family: inherit;
		font-size: 0.78rem;
		color: #cfcabd;
		cursor: pointer;
	}
	.m-tile.filled:hover { background: #232320; border-color: #5a5a52; }
	.m-tile.filled:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	/* ── Inline marginalia editor (replaces grid while active) ───────────── */

	.m-editor {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.m-editor-label {
		font-size: 0.72rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-accent);
		margin: 0;
	}

	.m-editor-input {
		width: 100%;
		font-family: inherit;
		font-size: 0.88rem;
		line-height: 1.4;
		padding: 0.5rem 0.6rem;
		background: #1d1d1a;
		color: var(--color-text);
		border: 1px solid #5a5a52;
		border-radius: 6px;
		resize: vertical;
		min-height: 84px;
	}
	.m-editor-input:focus { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	.m-editor-error {
		color: var(--color-danger);
		font-size: 0.78rem;
		margin: 0;
	}

	.m-editor-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}

	.m-btn {
		padding: 0.45rem 0.9rem;
		min-height: 40px;
		border-radius: 6px;
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
	}
	.m-btn.primary { background: var(--color-accent); color: var(--color-bg); }
	.m-btn.secondary { background: var(--color-border); color: var(--color-text); border: 1px solid #4a4a44; }
	.m-btn:disabled { opacity: 0.45; cursor: not-allowed; }
</style>
