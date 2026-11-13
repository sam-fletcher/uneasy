<!-- LawsRumors.svelte
  The Public Record's Laws and Rumors lists. These are long-form narrative
  records written by players (Propose Decree → laws; Spread Rumors and Host
  Festivity → rumors). Everyone at the table can read them; the player who
  wrote one may edit it in place.

  An entry is plain text between lines, not a card: boxes are for controls, and
  these entries are only read. The one control is the Edit button on your own.

  Each entry carries a byline, so where a law or rumor came from is legible at a
  glance, and every name in one comes with its player's shape:
    - Law:   "Proposed by" the origin plan's preparer, "Signed by" the
             signatory (often a different, higher-power player).
    - Rumor: "Spread by" the source player, or "Unknown" (faint italics, no
             shape) when the spreader hid themselves (Spread Rumors, hide-source).
             An "About" line above it names the asset the rumor is about, with
             the shape of whoever owns that asset.

  A pale "new" dot starts an entry you haven't seen. The server keeps, per player
  and per list, the highest revision seen, and an edit draws a new revision, so an
  edit counts as new (lib/lawsRumorsSeen). The table page reports a list as seen
  the moment it is on screen, and the marker it holds moves as it does, so this
  component keeps the marker as it was when the list OPENED and draws dots from
  that: an entry keeps its dot for as long as you are reading the list. Leaving the list and coming
  back opens it again against the newer marker. Your own edit through this
  component isn't new to you either way.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { untrack } from 'svelte';
	import {
		updateLaw, updateRumor,
		type Asset, type Law, type Rumor, type Plan, type Player,
	} from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import { isNewEntry } from '$lib/lawsRumorsSeen';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import NewDot from '$lib/components/shared/NewDot.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';

	interface Props {
		/** Which list the pane is showing. */
		kind: 'laws' | 'rumors';
		laws: Law[];
		rumors: Rumor[];
		plans: Plan[];
		players: Player[];
		playerNames: Map<number, string>;
		currentPlayerID: number | null;
		/** Every asset, for a rumor's "About" line. */
		assets?: Asset[];
		/** The viewer's seen marker for each list (null until the table has loaded
		 *  it). The dots are drawn from it as it was when the list opened. */
		seenLaws?: number | null;
		seenRumors?: number | null;
	}

	const {
		kind, laws, rumors, plans, players, playerNames, currentPlayerID,
		assets = [], seenLaws = null, seenRumors = null,
	}: Props = $props();

	const markerOf = (k: 'laws' | 'rumors') => (k === 'laws' ? seenLaws : seenRumors);

	// ── "New", against the marker as it was when this list opened ──────────────
	// `dotsFrom` is taken once per opening of a list: from the prop when the
	// component first renders (the server has not yet been told it is seen), again
	// when the pane switches between Laws and Rumors, and, if the marker hadn't
	// loaded yet, as soon as it does. It never follows the prop afterwards. An
	// $effect.pre so a switch never paints one list against the other's marker.
	let dotsFrom = $state<number | null>(untrack(() => markerOf(kind)));
	let shownKind: 'laws' | 'rumors' = untrack(() => kind);
	$effect.pre(() => {
		const marker = markerOf(kind);
		untrack(() => {
			if (kind !== shownKind) {
				shownKind = kind;
				dotsFrom = marker;
			} else if (dotsFrom == null) {
				dotsFrom = marker;
			}
		});
	});

	// The revisions this viewer wrote through the Edit button. An edit draws a new
	// revision, which would read as new to the person who just wrote it.
	let ownEdits = $state<Record<string, number>>({});
	const isNew = (key: string, entry: { rev: number }) =>
		dotsFrom != null && isNewEntry(entry, dotsFrom) && ownEdits[key] !== entry.rev;

	// ── Who, and what about ───────────────────────────────────────────────────
	function playerName(id: number | null | undefined): string {
		if (id == null) return '?';
		return playerNames.get(id) ?? '?';
	}
	const playerOf = (id: number | null | undefined) =>
		id == null ? null : (players.find((pl) => pl.id === id) ?? null);

	function preparerOf(planID: number | null | undefined): number | null {
		if (planID == null) return null;
		return plans.find((p) => p.id === planID)?.preparer_id ?? null;
	}

	/** The asset a rumor is about and who owns it now, or null when the rumor
	 *  names none (or the asset isn't on this client). */
	function aboutOf(rumor: Rumor): { name: string; owner: Player | null } | null {
		if (rumor.target_asset_id == null) return null;
		const asset = assets.find((a) => a.id === rumor.target_asset_id);
		if (!asset) return null;
		return { name: asset.name, owner: playerOf(asset.owner_id) };
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
			const { law } = await updateLaw(editingLawID, {
				text: draftText.trim(),
				addendum: draftAddendum.trim() ? draftAddendum.trim() : null,
			});
			ownEdits[`law:${law.id}`] = law.rev;
			cancelEdit();
		} catch (e) {
			errorMsg = e instanceof Error ? e.message : 'Could not save.';
		} finally { saving = false; }
	}

	async function saveRumor() {
		if (editingRumorID == null || saving || !draftText.trim()) return;
		saving = true; errorMsg = '';
		try {
			const { rumor } = await updateRumor(editingRumorID, draftText.trim());
			ownEdits[`rumor:${rumor.id}`] = rumor.rev;
			cancelEdit();
		} catch (e) {
			errorMsg = e instanceof Error ? e.message : 'Could not save.';
		} finally { saving = false; }
	}

	const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;
</script>

<!-- A name in a byline: the player's shape and then the name in their colour.
     A hidden source has neither: "Unknown" in faint italics. -->
{#snippet byline(label: string, who: Player | null, name: string, hidden = false)}
	<span class="lr-byline">
		<span class="lr-byline-label">{label}</span>
		{#if hidden}
			<span class="lr-unknown">Unknown</span>
		{:else}
			<PlayerShape player={who} />
			<span class="lr-byline-name" style:color={playerColor(who)}>{name}</span>
		{/if}
	</span>
{/snippet}

{#snippet editButton(onclick: () => void)}
	<button class="action-btn secondary lr-edit" {onclick}>Edit</button>
{/snippet}

{#snippet editForm(withAddendum: boolean, save: () => void)}
	<textarea rows={3} bind:value={draftText} class="lr-field" maxlength={TEXT_LIMITS.LONG_TEXT}
		aria-label={withAddendum ? 'Law text' : 'Rumor text'}></textarea>
	{#if withAddendum}
		<input type="text" bind:value={draftAddendum} class="lr-field"
			placeholder="Addendum (but/and …)" aria-label="Addendum" maxlength={TEXT_LIMITS.LONG_TEXT} />
	{/if}
	<div class="lr-row">
		<button class="action-btn primary" onclick={save} disabled={saving || !draftText.trim()}>
			{saving ? '…' : 'Save'}
		</button>
		<button class="action-btn secondary" onclick={cancelEdit} disabled={saving}>Cancel</button>
	</div>
{/snippet}

<aside class="laws-rumors">
	{#if errorMsg}<ErrorText message={errorMsg} />{/if}

	{#if kind === 'laws'}
		<section class="lr-section" aria-label="Laws">
			{#if laws.length === 0}
				<p class="muted-text lr-empty">No laws yet.</p>
			{:else}
				<p class="lr-count">{plural(laws.length, 'law', 'laws')}</p>
				<ul class="lr-list">
					{#each laws as law (law.id)}
						{@const proposerID = preparerOf(law.origin_plan_id)}
						<li class="lr-item">
							{#if editingLawID === law.id}
								{@render editForm(true, saveLaw)}
							{:else}
								<div class="lr-body">
									{#if isNew(`law:${law.id}`, law)}<span class="lr-dot"><NewDot /></span>{/if}
									<p class="lr-text">{law.text}</p>
								</div>
								{#if law.addendum}<p class="lr-addendum">…{law.addendum}</p>{/if}
								<div class="lr-bylines">
									{#if proposerID != null}
										{@render byline('Proposed by', playerOf(proposerID), playerName(proposerID))}
									{/if}
									{#if law.signatory_id != null}
										{@render byline('Signed by', playerOf(law.signatory_id), playerName(law.signatory_id))}
									{/if}
									{#if canEditLaw(law)}
										{@render editButton(() => startEditLaw(law))}
									{/if}
								</div>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{:else}
		<section class="lr-section" aria-label="Rumors">
			{#if rumors.length === 0}
				<p class="muted-text lr-empty">No rumors yet.</p>
			{:else}
				<p class="lr-count">{plural(rumors.length, 'rumor', 'rumors')}</p>
				<ul class="lr-list">
					{#each rumors as rumor (rumor.id)}
						{@const hidden = rumor.source_player_id == null}
						{@const about = aboutOf(rumor)}
						<li class="lr-item">
							{#if editingRumorID === rumor.id}
								{@render editForm(false, saveRumor)}
							{:else}
								{#if about}
									<div class="lr-about">
										<span>About</span>
										{#if about.owner}
											<PlayerShape player={about.owner} label="{playerName(about.owner.id)}’s" />
										{/if}
										<span class="lr-about-name">{about.name}</span>
									</div>
								{/if}
								<div class="lr-body">
									{#if isNew(`rumor:${rumor.id}`, rumor)}<span class="lr-dot"><NewDot /></span>{/if}
									<p class="lr-text">{rumor.text}</p>
								</div>
								<div class="lr-bylines">
									{@render byline(
										'Spread by',
										playerOf(rumor.source_player_id),
										playerName(rumor.source_player_id),
										hidden,
									)}
									{#if canEditRumor(rumor)}
										{@render editButton(() => startEditRumor(rumor))}
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
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		/* The page's scroller already gives the list 8px at its edges; this brings
		   it to the same 16px the Events list keeps. */
		padding: 0 var(--space-2);
	}
	.lr-empty { font-style: italic; padding: var(--space-3) 0; }

	/* The list's own total ("2 laws"). Totals live here, never on a tab. */
	.lr-count {
		margin: 0;
		font-size: var(--text-label);
		color: var(--color-text-faint);
	}
	.lr-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	/* Plain entries separated by hairlines: no card, no tint. */
	.lr-item {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding: var(--space-3) 0 var(--space-1);
		border-bottom: 1px solid var(--color-border);
	}

	/* "About [shape] Lady Merrow": small, so the rumor's own words lead. */
	.lr-about {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-label);
		color: var(--color-text-faint);
	}
	.lr-about-name { color: var(--color-text-secondary); }

	/* The dot sits level with the first line of the text, not the paragraph. */
	.lr-body {
		display: flex;
		align-items: flex-start;
		gap: var(--space-2);
		font-size: var(--text-body);
		line-height: 1.5;
	}
	.lr-dot {
		display: flex;
		align-items: center;
		flex: none;
		height: 1lh;
	}
	.lr-text {
		margin: 0;
		min-width: 0;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		font-family: var(--font-serif);
		line-height: 1.5;
	}
	.lr-addendum {
		margin: 0;
		font-family: var(--font-serif);
		font-size: var(--text-small);
		font-style: italic;
		line-height: 1.5;
		color: var(--color-text-secondary);
	}

	/* Bylines, then the Edit button at the far end. The row is at least a tap
	   tall, so an entry with no button keeps the same rhythm as one with. */
	.lr-bylines {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-1) var(--space-3);
		min-height: var(--size-tap);
		font-size: var(--text-small);
	}
	.lr-byline {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		white-space: nowrap;
	}
	.lr-byline-label { color: var(--color-text-faint); }
	/* A hidden source is a deliberate absence, so it carries no shape and no
	   colour: faint, italic. */
	.lr-unknown { color: var(--color-text-faint); font-style: italic; }

	/* A bordered 44px button (the house secondary), pushed to the end. It is a
	   little smaller in type than the body so it doesn't outweigh the byline. */
	.lr-edit {
		align-self: center;
		margin-left: auto;
		font-size: var(--text-small);
	}

	/* Editing in place. Body text size on every input, so iPhone Safari doesn't
	   zoom the page when one is focused. */
	.lr-field {
		box-sizing: border-box;
		width: 100%;
		padding: var(--space-2) var(--space-3);
		background: var(--color-surface);
		color: var(--color-text);
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
		font: inherit;
		font-size: var(--text-body);
	}
	.lr-field:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}
	.lr-row { display: flex; gap: var(--space-2); }
</style>
