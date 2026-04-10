<!-- PrologueView.svelte
  Prologue phase: asset creation, retinue display, rankings editor (facilitator),
  and seat order assignment. Handles its own state; rankings and players are
  bindable so the parent stays in sync when the facilitator saves changes.
-->
<script lang="ts">
	import { startMainEvent, createAsset, tearMarginalia, setRankings, setSeats } from '$lib/api';
	import type { Player, Asset, Marginalium, Ranking, RankingCategory } from '$lib/api';
	import AssetCard from '$lib/components/AssetCard.svelte';

	interface Props {
		gameID: string;
		players: Player[];
		assets: Asset[];
		/** Two-way binding — saveRankings writes back to the parent. */
		rankings: Ranking[];
		currentPlayerID: number | null;
		isFacilitator: boolean;
	}

	let {
		gameID,
		players = $bindable(),
		assets,
		rankings = $bindable(),
		currentPlayerID,
		isFacilitator,
	}: Props = $props();

	const myAssets = $derived(assets.filter(a => a.owner_id === currentPlayerID));

	function rankingLabel(playerID: number | null): string {
		if (playerID === null) return 'Dummy';
		return players.find(p => p.id === playerID)?.display_name ?? '?';
	}

	// ── Asset creation ────────────────────────────────────────────────────────
	let newAssetType = $state<Asset['asset_type']>('peer');
	let newAssetName = $state('');
	let newAssetIsMain = $state(false);
	let newAssetMarginalia = $state(['', '']);
	let creatingAsset = $state(false);
	let error = $state('');

	async function submitAsset() {
		const name = newAssetName.trim();
		if (!name || creatingAsset) return;
		creatingAsset = true;
		error = '';
		try {
			const marginalia = newAssetMarginalia.map(m => m.trim()).filter(Boolean);
			await createAsset(gameID, {
				asset_type: newAssetType,
				name,
				is_main_character: newAssetIsMain && newAssetType === 'peer',
				marginalia,
			});
			newAssetName = '';
			newAssetIsMain = false;
			newAssetMarginalia = ['', ''];
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not create asset.';
		} finally {
			creatingAsset = false;
		}
	}

	async function onTearMarginalia(asset: Asset, m: Marginalium) {
		if (!confirm(`Tear "${m.text}"? This cannot be undone.`)) return;
		try {
			await tearMarginalia(asset.id, m.position);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not tear marginalia.';
		}
	}

	// ── Rankings (facilitator) ────────────────────────────────────────────────
	type RankSlot = number | null | 'unset';
	let draftRankings = $state<Record<RankingCategory, RankSlot[]>>({
		power:     ['unset','unset','unset','unset','unset'],
		knowledge: ['unset','unset','unset','unset','unset'],
		esteem:    ['unset','unset','unset','unset','unset'],
	});
	let savingRankings = $state(false);

	// Re-initialise draft whenever the server's rankings arrive or change.
	$effect(() => {
		if (rankings.length > 0) {
			const draft: Record<RankingCategory, RankSlot[]> = {
				power:     ['unset','unset','unset','unset','unset'],
				knowledge: ['unset','unset','unset','unset','unset'],
				esteem:    ['unset','unset','unset','unset','unset'],
			};
			for (const r of rankings) {
				draft[r.category][r.rank - 1] = r.player_id ?? null;
			}
			draftRankings = draft;
		}
	});

	const rankingOptions = $derived([
		...players.map(p => ({ value: String(p.id), label: p.display_name })),
		{ value: 'null', label: 'Dummy token' },
	]);

	const rankingsComplete = $derived(
		(['power', 'knowledge', 'esteem'] as RankingCategory[])
			.every(cat => draftRankings[cat].every(v => v !== 'unset'))
	);

	async function saveRankings() {
		const entries: Array<{ player_id: number | null; category: RankingCategory; rank: number }> = [];
		for (const cat of ['power', 'knowledge', 'esteem'] as RankingCategory[]) {
			for (let i = 0; i < 5; i++) {
				const val = draftRankings[cat][i];
				if (val === 'unset') {
					error = `Please fill all ranking slots (missing: ${cat} rank ${i + 1})`;
					return;
				}
				entries.push({ player_id: val as number | null, category: cat, rank: i + 1 });
			}
		}
		savingRankings = true;
		error = '';
		try {
			const result = await setRankings(gameID, entries);
			rankings = result.rankings;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not save rankings.';
		} finally {
			savingRankings = false;
		}
	}

	// ── Seat order (facilitator) ──────────────────────────────────────────────
	let draftSeats = $state<Record<number, string>>({});
	let savingSeats = $state(false);

	$effect(() => {
		const seats: Record<number, string> = {};
		for (const p of players) {
			seats[p.id] = p.seat_order != null ? String(p.seat_order) : '';
		}
		draftSeats = seats;
	});

	const seatsComplete = $derived(
		players.length > 0 && players.every(p => p.seat_order != null)
	);

	async function saveSeats() {
		const seats: Array<{ player_id: number; seat_order: number }> = [];
		for (const p of players) {
			const raw = draftSeats[p.id];
			const n = parseInt(raw, 10);
			if (!raw || isNaN(n) || n < 1) {
				error = `Enter a valid seat number for ${p.display_name}`;
				return;
			}
			seats.push({ player_id: p.id, seat_order: n });
		}
		savingSeats = true;
		error = '';
		try {
			await setSeats(gameID, seats);
			// Optimistic update (no WS broadcast for seat changes).
			players = players.map(p => {
				const s = seats.find(s => s.player_id === p.id);
				return s ? { ...p, seat_order: s.seat_order } : p;
			});
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not save seats.';
		} finally {
			savingSeats = false;
		}
	}

	// ── Phase advance ─────────────────────────────────────────────────────────
	let advancing = $state(false);

	async function advanceToMainEvent() {
		if (advancing) return;
		advancing = true;
		error = '';
		try {
			await startMainEvent(gameID);
			// Phase change propagates via WebSocket → parent re-renders.
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not start main event.';
		} finally {
			advancing = false;
		}
	}
</script>

<div class="prologue-view">
	<h2>Prologue</h2>
	<p class="muted">
		Each player creates their main character (a peer) plus any additional assets.
		{#if isFacilitator}
			As facilitator, you also set the initial rankings and seat order before starting.
		{/if}
	</p>

	{#if error}
		<p class="local-error">{error}</p>
	{/if}

	<div class="prologue-columns">
		<!-- Left column: asset creation + retinue -->
		<section class="prologue-section">
			<h3>Create an Asset</h3>
			<div class="asset-form">
				<div class="form-row">
					<label>
						Type
						<select bind:value={newAssetType}>
							<option value="peer">Peer</option>
							<option value="holding">Holding</option>
							<option value="artifact">Artifact</option>
							<option value="resource">Resource</option>
						</select>
					</label>
					<label class="name-label">
						Name
						<input
							type="text"
							bind:value={newAssetName}
							placeholder="Asset name…"
							maxlength={80}
						/>
					</label>
				</div>

				{#if newAssetType === 'peer'}
					<label class="checkbox-label">
						<input type="checkbox" bind:checked={newAssetIsMain} />
						Main character (sets this peer as your character)
					</label>
				{/if}

				<div class="marginalia-inputs">
					<span class="field-label">Marginalia (optional, up to 4)</span>
					{#each newAssetMarginalia as _, i}
						<input
							type="text"
							placeholder="Marginalia {i + 1}…"
							bind:value={newAssetMarginalia[i]}
							maxlength={200}
						/>
					{/each}
					{#if newAssetMarginalia.length < 4}
						<button class="text-btn" onclick={() => { newAssetMarginalia = [...newAssetMarginalia, '']; }}>
							+ Add marginalia
						</button>
					{/if}
				</div>

				<button
					class="primary"
					onclick={submitAsset}
					disabled={!newAssetName.trim() || creatingAsset}
				>
					{creatingAsset ? '…' : 'Create Asset'}
				</button>
			</div>

			{#if myAssets.length > 0}
				<h3 style="margin-top: 1.5rem;">Your Retinue</h3>
				<div class="asset-list">
					{#each myAssets as asset (asset.id)}
						<AssetCard {asset} onTear={onTearMarginalia} />
					{/each}
				</div>
			{/if}
		</section>

		<!-- Right column: rankings + seats (facilitator only) -->
		{#if isFacilitator}
			<section class="prologue-section facilitator-panel">
				<h3>Initial Rankings</h3>
				<p class="muted small">
					Assign each rank (1 = highest) to a player or a dummy token.
					You need to fill all 15 slots before starting.
				</p>

				<div class="rankings-grid">
					{#each ['power', 'knowledge', 'esteem'] as cat}
						<div class="rank-col">
							<h4>{cat}</h4>
							{#each [1,2,3,4,5] as rank}
								<div class="rank-slot">
									<span class="rank-num">{rank}</span>
									<select
										value={
											draftRankings[cat as RankingCategory][rank - 1] === 'unset'
												? ''
												: draftRankings[cat as RankingCategory][rank - 1] === null
													? 'null'
													: String(draftRankings[cat as RankingCategory][rank - 1])
										}
										onchange={(e) => {
											const v = (e.target as HTMLSelectElement).value;
											draftRankings[cat as RankingCategory][rank - 1] =
												v === '' ? 'unset' : v === 'null' ? null : Number(v);
										}}
									>
										<option value="">— pick —</option>
										{#each rankingOptions as opt}
											<option value={opt.value}>{opt.label}</option>
										{/each}
									</select>
								</div>
							{/each}
						</div>
					{/each}
				</div>

				<button class="secondary" onclick={saveRankings} disabled={savingRankings}>
					{savingRankings ? '…' : 'Save Rankings'}
				</button>

				<h3 style="margin-top: 1.5rem;">Seat Order</h3>
				<p class="muted small">
					Assign a clockwise seat number to each player (1, 2, 3…).
				</p>

				<div class="seat-grid">
					{#each players as p}
						<div class="seat-row">
							<span class="seat-name">{p.display_name}</span>
							<input
								type="number"
								min="1"
								max={players.length}
								placeholder="#"
								class="seat-input"
								value={draftSeats[p.id] ?? ''}
								oninput={(e) => { draftSeats[p.id] = (e.target as HTMLInputElement).value; }}
							/>
						</div>
					{/each}
				</div>

				<button class="secondary" onclick={saveSeats} disabled={savingSeats}>
					{savingSeats ? '…' : 'Save Seat Order'}
				</button>

				<div class="start-section">
					<button
						class="primary"
						onclick={advanceToMainEvent}
						disabled={advancing || !rankingsComplete || !seatsComplete}
						title={
							!rankingsComplete ? 'Rankings are not fully set' :
							!seatsComplete    ? 'Seat order is not fully set' : undefined
						}
					>
						{advancing ? '…' : 'Start Main Event'}
					</button>
					{#if !rankingsComplete}
						<p class="hint">Fill all 15 ranking slots first.</p>
					{:else if !seatsComplete}
						<p class="hint">Assign a seat to every player first.</p>
					{/if}
				</div>
			</section>
		{/if}
	</div>

	<!-- Current rankings for non-facilitators -->
	{#if !isFacilitator && rankings.length > 0}
		<div class="rankings-preview">
			{#each ['power', 'knowledge', 'esteem'] as cat}
				<div class="rank-col">
					<h3>{cat}</h3>
					{#each rankings.filter(r => r.category === cat).sort((a, b) => a.rank - b.rank) as r}
						<div class="rank-slot-display">
							{r.rank}. {rankingLabel(r.player_id)}
						</div>
					{/each}
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.prologue-view {
		flex: 1;
		display: flex;
		flex-direction: column;
		padding: 1rem 0;
		gap: 1rem;
		overflow-y: auto;
		min-height: 0;
	}

	.prologue-view h2 {
		color: #c8a96e;
		font-size: 1.3rem;
		margin: 0;
	}

	.prologue-view h3 {
		color: #c8a96e;
		font-size: 1rem;
		margin: 0;
	}

	.local-error {
		color: #e07070;
		font-size: 0.85rem;
		margin: 0;
	}

	/* ── Columns ──────────────────────────────────────────────────────────────── */

	.prologue-columns {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 2rem;
		align-items: start;
	}

	@media (max-width: 700px) {
		.prologue-columns { grid-template-columns: 1fr; }
	}

	.prologue-section {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	/* ── Asset form ────────────────────────────────────────────────────────────── */

	.asset-form {
		background: #222;
		border-radius: 8px;
		padding: 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	.form-row {
		display: flex;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.form-row label {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		font-size: 0.8rem;
		color: #aaa;
	}

	.name-label { flex: 1; }

	.form-row select, .form-row input {
		background: #333;
		color: #e8e4d9;
		border: 1px solid #555;
		border-radius: 4px;
		padding: 0.3rem 0.5rem;
		font-size: 0.9rem;
	}

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.85rem;
		color: #ccc;
	}

	.field-label {
		font-size: 0.8rem;
		color: #aaa;
	}

	.marginalia-inputs {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.marginalia-inputs input {
		background: #333;
		color: #e8e4d9;
		border: 1px solid #555;
		border-radius: 4px;
		padding: 0.3rem 0.5rem;
		font-size: 0.85rem;
	}

	/* ── Asset list ────────────────────────────────────────────────────────────── */

	.asset-list { display: flex; flex-direction: column; gap: 0.6rem; }

	/* ── Facilitator panel ─────────────────────────────────────────────────────── */

	.facilitator-panel {
		background: #1e1e1e;
		border: 1px solid #333;
		border-radius: 8px;
		padding: 1rem;
	}

	.rankings-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.75rem;
		margin-bottom: 0.75rem;
	}

	.rank-col h4 {
		font-size: 0.8rem;
		color: #c8a96e;
		text-transform: capitalize;
		margin: 0 0 0.4rem;
	}

	.rank-slot {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		margin-bottom: 0.3rem;
	}

	.rank-num {
		font-size: 0.75rem;
		color: #666;
		width: 1rem;
		flex-shrink: 0;
	}

	.rank-slot select {
		flex: 1;
		font-size: 0.8rem;
		background: #2a2a2a;
		color: #e8e4d9;
		border: 1px solid #555;
		border-radius: 3px;
		padding: 0.2rem 0.3rem;
	}

	.seat-grid {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		margin-bottom: 0.75rem;
	}

	.seat-row {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}

	.seat-name {
		flex: 1;
		font-size: 0.9rem;
	}

	.seat-input {
		width: 3.5rem;
		background: #2a2a2a;
		color: #e8e4d9;
		border: 1px solid #555;
		border-radius: 4px;
		padding: 0.25rem 0.4rem;
		font-size: 0.9rem;
		text-align: center;
	}

	.start-section {
		margin-top: 1.25rem;
		padding-top: 1rem;
		border-top: 1px solid #333;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	/* ── Rankings preview (non-facilitator) ──────────────────────────────────── */

	.rankings-preview {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 1rem;
	}

	.rank-col { display: flex; flex-direction: column; gap: 0.2rem; }

	.rank-slot-display {
		font-size: 0.85rem;
		color: #ccc;
		padding: 0.15rem 0;
	}

	/* ── Utility classes (re-declared here for component isolation) ───────────── */
	/* These mirror the global utilities in +layout.svelte */

	.muted {
		color: #999;
		font-size: 0.9rem;
		line-height: 1.5;
		margin: 0;
	}

	.muted.small { font-size: 0.8rem; }

	.primary {
		background: #c8a96e;
		color: #1a1a1a;
		font-weight: 600;
		padding: 0.6rem 1.2rem;
		border-radius: 6px;
		align-self: flex-start;
	}

	.primary:disabled { opacity: 0.4; cursor: not-allowed; }

	.secondary {
		background: #333;
		color: #e8e4d9;
		font-weight: 600;
		padding: 0.5rem 1rem;
		border-radius: 6px;
		align-self: flex-start;
		border: 1px solid #555;
	}

	.secondary:disabled { opacity: 0.4; cursor: not-allowed; }

	.text-btn {
		background: none;
		color: #c8a96e;
		padding: 0;
		font-size: 0.85rem;
		text-decoration: underline;
		cursor: pointer;
	}

	.hint {
		font-size: 0.8rem;
		color: #e0a060;
		margin: 0;
	}
</style>
