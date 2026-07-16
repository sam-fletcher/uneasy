<!-- LawsRumors.svelte
  Sidebar list of enacted laws and active rumors. These are long-form
  narrative records written by players (Propose Decree → laws; Spread
  Rumors and Host Festivity → rumors). The authoring player may edit
  their entry inline; everyone at the table can read.

  Each card is accented with its author's token colour and carries a
  byline so the source of a law or rumor is legible at a glance:
    - Law:   "Proposed by" the origin plan's preparer, "Signed by" the
             signatory (often a different, higher-power player).
    - Rumor: "Spread by" the source player, or "Unknown" when the
             spreader chose to hide themselves (Spread Rumors hide-source).
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { updateLaw, updateRumor, type Law, type Rumor, type Plan, type Player } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import { TEXT_LIMITS } from '$lib/textLimits';

	interface Props {
		kind?: 'laws' | 'rumors' | 'both';
		laws: Law[];
		rumors: Rumor[];
		plans: Plan[];
		players: Player[];
		playerNames: Map<number, string>;
		currentPlayerID: number | null;
	}

	const { kind = 'both', laws, rumors, plans, players, playerNames, currentPlayerID }: Props = $props();

	const NEUTRAL = 'var(--color-accent-dim)';

	function playerName(id: number | null | undefined): string {
		if (id == null) return '?';
		return playerNames.get(id) ?? '?';
	}

	function colorOf(id: number | null | undefined): string {
		if (id == null) return NEUTRAL;
		const p = players.find(pl => pl.id === id);
		return p ? playerColor(p) : NEUTRAL;
	}

	function preparerOf(planID: number | null | undefined): number | null {
		if (planID == null) return null;
		return plans.find(p => p.id === planID)?.preparer_id ?? null;
	}

	// A law is accented by its proposer (the plan's preparer); if that's
	// unknown (e.g. a prologue law with no origin plan) fall back to the
	// signatory's colour, then neutral.
	function lawAccent(law: Law): string {
		const proposer = preparerOf(law.origin_plan_id);
		if (proposer != null) return colorOf(proposer);
		return colorOf(law.signatory_id);
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

{#snippet byline(label: string, name: string, color: string, hidden = false)}
	<span class="lr-byline" class:hidden>
		<span class="lr-dot" style:background={color} aria-hidden="true"></span>
		<span class="lr-byline-label">{label}</span>
		<span class="lr-byline-name">{name}</span>
	</span>
{/snippet}

<aside class="laws-rumors">
	{#if errorMsg}<p class="error-text">{errorMsg}</p>{/if}

	{#if kind !== 'rumors'}
	<section class="lr-section">
		{#if laws.length === 0}
			<p class="lr-empty">No laws yet.</p>
		{:else}
			<ul class="lr-list">
				{#each laws as law (law.id)}
					{@const proposerID = preparerOf(law.origin_plan_id)}
					<li class="lr-item" style:--accent={lawAccent(law)}>
						{#if editingLawID === law.id}
							<textarea rows={3} bind:value={draftText} class="lr-textarea" maxlength={TEXT_LIMITS.LONG_TEXT}></textarea>
							<input type="text" bind:value={draftAddendum} class="lr-input"
								placeholder="Addendum (but/and …)" maxlength={TEXT_LIMITS.LONG_TEXT} />
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
							<div class="lr-bylines">
								{#if proposerID != null}
									{@render byline('Proposed by', playerName(proposerID), colorOf(proposerID))}
								{/if}
								{#if law.signatory_id != null}
									{@render byline('Signed by', playerName(law.signatory_id), colorOf(law.signatory_id))}
								{/if}
								{#if canEditLaw(law)}
									<button class="lr-edit" onclick={() => startEditLaw(law)}>edit</button>
								{/if}
							</div>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>
	{/if}

	{#if kind !== 'laws'}
	<section class="lr-section">
		{#if rumors.length === 0}
			<p class="lr-empty">No rumors yet.</p>
		{:else}
			<ul class="lr-list">
				{#each rumors as rumor (rumor.id)}
					{@const hidden = rumor.source_player_id == null}
					<li class="lr-item" class:lr-anon={hidden} style:--accent={colorOf(rumor.source_player_id)}>
						{#if editingRumorID === rumor.id}
							<textarea rows={3} bind:value={draftText} class="lr-textarea" maxlength={TEXT_LIMITS.LONG_TEXT}></textarea>
							<div class="lr-row">
								<button class="action-btn primary" onclick={saveRumor}
									disabled={saving || !draftText.trim()}>
									{saving ? '…' : 'Save'}
								</button>
								<button class="action-btn" onclick={cancelEdit} disabled={saving}>Cancel</button>
							</div>
						{:else}
							<p class="lr-text">{rumor.text}</p>
							<div class="lr-bylines">
								{@render byline(
									'Spread by',
									hidden ? 'Unknown' : playerName(rumor.source_player_id),
									colorOf(rumor.source_player_id),
									hidden,
								)}
								{#if canEditRumor(rumor)}
									<button class="lr-edit" onclick={() => startEditRumor(rumor)}>edit</button>
								{/if}
							</div>
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
	}
	.lr-empty { margin: 0; color: var(--color-text-muted); font-style: italic; }
	.lr-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.6rem; }
	.lr-item {
		--accent: var(--color-accent-dim);
		padding: 0.6rem 0.75rem;
		background:
			color-mix(in srgb, var(--accent) 7%, var(--card-bg, var(--color-surface-2)));
		border: 1px solid color-mix(in srgb, var(--accent) 25%, var(--parchment-200));
		border-left: 4px solid var(--accent);
		border-radius: 6px;
	}
	/* Anonymous rumors get a muted, dashed accent so "Unknown" reads as a
	   deliberate absence rather than an un-coloured player. */
	.lr-item.lr-anon {
		border-left-style: dashed;
		background: var(--card-bg, var(--color-surface-2));
	}
	.lr-text { margin: 0 0 0.25rem; white-space: pre-wrap; font-family: var(--font-serif); line-height: 1.4; }
	.lr-addendum { margin: 0 0 0.35rem; color: var(--color-text-muted); font-family: var(--font-serif); }
	.lr-bylines {
		display: flex; flex-wrap: wrap; align-items: center; gap: 0.35rem 0.6rem;
		margin-top: 0.4rem; font-size: 0.78rem;
	}
	.lr-byline {
		display: inline-flex; align-items: center; gap: 0.3rem;
		padding: 0.1rem 0.45rem 0.1rem 0.3rem;
		border-radius: 999px;
		background: color-mix(in srgb, var(--accent) 22%, transparent);
		color: var(--color-text);
		white-space: nowrap;
	}
	.lr-byline.hidden { background: transparent; border: 1px dashed var(--parchment-200); }
	.lr-dot { width: 0.5rem; height: 0.5rem; border-radius: 50%; flex: none; }
	.lr-byline-label { color: var(--color-text-muted); }
	.lr-edit {
		background: none; border: none; color: var(--color-text-muted);
		text-decoration: underline; cursor: pointer; font: inherit; padding: 0;
		margin-left: auto;
	}
	.lr-textarea, .lr-input {
		width: 100%; margin-bottom: 0.35rem; font: inherit;
		padding: 0.25rem 0.35rem;
	}
	.lr-row { display: flex; gap: 0.4rem; }
</style>
