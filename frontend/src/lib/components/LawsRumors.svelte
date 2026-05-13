<!-- LawsRumors.svelte
  Sidebar list of enacted laws and active rumors. These are long-form
  narrative records written by players (Propose Decree → laws; Spread
  Rumors and Host Festivity → rumors). The authoring player may edit
  their entry inline; everyone at the table can read.

  Authoring rules (matching the backend):
    - Law:   signatory, else the origin plan's preparer.
    - Rumor: source_player_id, else the origin plan's preparer.
-->
<script lang="ts">
	import { updateLaw, updateRumor, type Law, type Rumor, type Plan } from '$lib/api';

	interface Props {
		kind?: 'laws' | 'rumors' | 'both';
		laws: Law[];
		rumors: Rumor[];
		plans: Plan[];
		playerNames: Map<number, string>;
		currentPlayerID: number | null;
	}

	const { kind = 'both', laws, rumors, plans, playerNames, currentPlayerID }: Props = $props();

	function playerName(id: number | null | undefined): string {
		if (id == null) return '?';
		return playerNames.get(id) ?? '?';
	}

	function preparerOf(planID: number | null | undefined): number | null {
		if (planID == null) return null;
		return plans.find(p => p.id === planID)?.preparer_id ?? null;
	}

	function canEditLaw(law: Law): boolean {
		if (currentPlayerID == null) return false;
		if (law.signatory_id === currentPlayerID) return true;
		return preparerOf(law.origin_plan_id) === currentPlayerID;
	}
	function canEditRumor(rumor: Rumor): boolean {
		if (currentPlayerID == null) return false;
		if (rumor.source_player_id === currentPlayerID) return true;
		return preparerOf(rumor.origin_plan_id) === currentPlayerID;
	}

	// Editing state. Only one entry is edited at a time.
	let editingLawID = $state<number | null>(null);
	let editingRumorID = $state<number | null>(null);
	let draftText = $state('');
	let draftAddendum = $state('');
	let saving = $state(false);
	let errorMsg = $state('');

	function startEditLaw(law: Law) {
		editingLawID = law.id;
		editingRumorID = null;
		draftText = law.text;
		draftAddendum = law.addendum ?? '';
		errorMsg = '';
	}
	function startEditRumor(rumor: Rumor) {
		editingRumorID = rumor.id;
		editingLawID = null;
		draftText = rumor.text;
		errorMsg = '';
	}
	function cancelEdit() {
		editingLawID = null; editingRumorID = null;
		draftText = ''; draftAddendum = '';
		errorMsg = '';
	}

	async function saveLaw() {
		if (editingLawID == null || saving || !draftText.trim()) return;
		saving = true; errorMsg = '';
		try {
			await updateLaw(editingLawID, {
				text: draftText.trim(),
				addendum: draftAddendum.trim() ? draftAddendum.trim() : null,
			});
			cancelEdit();
		} catch (e) {
			errorMsg = e instanceof Error ? e.message : 'Could not save.';
		} finally { saving = false; }
	}

	async function saveRumor() {
		if (editingRumorID == null || saving || !draftText.trim()) return;
		saving = true; errorMsg = '';
		try {
			await updateRumor(editingRumorID, draftText.trim());
			cancelEdit();
		} catch (e) {
			errorMsg = e instanceof Error ? e.message : 'Could not save.';
		} finally { saving = false; }
	}
</script>

<aside class="laws-rumors">
	{#if errorMsg}<p class="lr-error">{errorMsg}</p>{/if}

	{#if kind !== 'rumors'}
	<section class="lr-section">
		<!-- <h4 class="lr-heading">Laws ({laws.length})</h4> -->
		{#if laws.length === 0}
			<p class="lr-empty">No laws yet.</p>
		{:else}
			<ul class="lr-list">
				{#each laws as law (law.id)}
					<li class="lr-item">
						{#if editingLawID === law.id}
							<textarea rows={3} bind:value={draftText} class="lr-textarea"></textarea>
							<input type="text" bind:value={draftAddendum} class="lr-input"
								placeholder="Addendum (but/and …)" />
							<div class="lr-row">
								<button class="action-btn primary" onclick={saveLaw}
									disabled={saving || !draftText.trim()}>
									{saving ? '…' : 'Save'}
								</button>
								<button class="action-btn" onclick={cancelEdit} disabled={saving}>Cancel</button>
							</div>
						{:else}
							<p class="lr-text">{law.text}</p>
							{#if law.addendum}<p class="lr-addendum"><em>…{law.addendum}</em></p>{/if}
							<p class="lr-meta">
								<!-- signatory: {playerName(law.signatory_id)} -->
								{#if canEditLaw(law)}
									· <button class="lr-edit" onclick={() => startEditLaw(law)}>edit</button>
								{/if}
							</p>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>
	{/if}

	{#if kind !== 'laws'}
	<section class="lr-section">
		<!-- <h4 class="lr-heading">Rumors ({rumors.length})</h4> -->
		{#if rumors.length === 0}
			<p class="lr-empty">No rumors yet.</p>
		{:else}
			<ul class="lr-list">
				{#each rumors as rumor (rumor.id)}
					<li class="lr-item">
						{#if editingRumorID === rumor.id}
							<textarea rows={3} bind:value={draftText} class="lr-textarea"></textarea>
							<div class="lr-row">
								<button class="action-btn primary" onclick={saveRumor}
									disabled={saving || !draftText.trim()}>
									{saving ? '…' : 'Save'}
								</button>
								<button class="action-btn" onclick={cancelEdit} disabled={saving}>Cancel</button>
							</div>
						{:else}
							<p class="lr-text">{rumor.text}</p>
							<p class="lr-meta">
								<!-- source: {rumor.source_player_id == null
									? 'hidden'
									: playerName(rumor.source_player_id)} -->
								{#if canEditRumor(rumor)}
									· <button class="lr-edit" onclick={() => startEditRumor(rumor)}>edit</button>
								{/if}
							</p>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>
	{/if}
</aside>

<style>
	.laws-rumors {
		display: flex; flex-direction: column; gap: 1rem;
		padding: 0.75rem; border: 1px solid var(--border-color, #d4c5a8);
		background: var(--panel-bg, #2a2a2a);
		border-radius: 6px;
	}
	.lr-heading { margin: 0 0 0.5rem; font-size: 0.95rem; }
	.lr-empty { margin: 0; color: var(--muted, #8a7d61); font-style: italic; }
	.lr-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.75rem; }
	.lr-item {
		padding: 0.5rem 0.6rem;
		background: var(--card-bg, #2a2a2a);
		border: 1px solid var(--border-color, #d4c5a8);
		border-radius: 4px;
	}
	.lr-text { margin: 0 0 0.25rem; white-space: pre-wrap; }
	.lr-addendum { margin: 0 0 0.25rem; color: var(--muted, #5a5446); }
	.lr-meta { margin: 0; font-size: 0.8rem; color: var(--muted, #8a7d61); }
	.lr-edit {
		background: none; border: none; color: inherit;
		text-decoration: underline; cursor: pointer; font: inherit; padding: 0;
	}
	.lr-textarea, .lr-input {
		width: 100%; margin-bottom: 0.35rem; font: inherit;
		padding: 0.25rem 0.35rem;
	}
	.lr-row { display: flex; gap: 0.4rem; }
	.lr-error { color: #a00; margin: 0; }
</style>
