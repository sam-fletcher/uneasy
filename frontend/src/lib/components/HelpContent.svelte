<!-- HelpContent.svelte
  Shared "how to play" reference, rendered both in the header ? panel and
  front-and-centre in the lobby. One source of truth so the two never drift.

  Prose-first with a few small theme-aware SVG diagrams for the genuinely
  spatial concepts (the Public Record timeline, the rankings ladder). No
  screenshots — they rot on every UI change and read poorly on mobile.

  Tabs follow the game's own structure rather than the app's screens, since
  that's how players think about it. The feedback link sits in a footer
  visible from every tab.
-->
<script lang="ts">
	import '$lib/components/shared/rankStrip.css';
	import '$lib/components/shared/rankChip.css';
	import '$lib/components/shared/cornerBadge.css';
	import '$lib/components/shared/marginaliaTile.css';
	import { PLAN_SHORT, TRACK_ORDER } from './plans/shared';
	import CrownGlyph from './CrownGlyph.svelte';
	import AssetTypeIcon from './AssetTypeIcon.svelte';
	import type { Asset } from '$lib/api';

	// Panel mode (the ? sheet): the help fills the sheet to a fixed height and the
	// body scrolls internally, so the footer is pinned and blank space is never
	// scrollable. The lobby leaves it false (footer sits in normal flow).
	// onFeedback: called when the footer's "Send feedback" trigger is tapped —
	// every caller supplies this (per the no-sheet-nesting ruling, opening
	// Feedback means the CALLER closes whatever sheet it owns and opens its
	// own separate Feedback sheet; this component has no opinion on that).
	let { panel = false, onFeedback }: { panel?: boolean; onFeedback: () => void } = $props();

	type TabId = 'goal' | 'record' | 'plans' | 'rankings' | 'dice' | 'assets';
	const tabs: { id: TabId; label: string }[] = [
		{ id: 'goal', label: 'Goal' },
		{ id: 'record', label: 'Public Record' },
		{ id: 'rankings', label: 'Rankings' },
		{ id: 'plans', label: 'Plans' },
		{ id: 'assets', label: 'Assets' },
		{ id: 'dice', label: 'Dice' },
	];

	// Opens on Goal: it's the orientation tab, and a player who taps "?" without
	// knowing what the game IS gets that answer before the Public Record's
	// mechanics. Anyone past that point is one tap from where they were.
	let active = $state<TabId>('goal');

	// y-coordinate of the Shake-Up pseudo-row in the record diagram, one row
	// pitch (11 units) below row 13's line (y = 7 + 12 * 11 = 139).
	const SHAKEUP_ROW_Y = 7 + 13 * 11;

	// Worked Mar example for the Dice tab. The pool is 2 starting dice + 1 help
	// die; a single interference die cancels the matching 4. The survivors (a 1
	// and a 6) make two distinct faces — short of a difficulty of 3.
	// Only ACTOR dice are ever cancelled (handler/rolls_dice.go
	// cancelInterference writes the cancelling die's id onto the actor's die and
	// nothing onto the interference die) — so the 4 that interferes is drawn
	// intact, and only the matching 4 in the pool is struck.
	const marDifficulty = 3;
	type MarDie = { face: number; group: 'start' | 'help' | 'interfere'; canceled: boolean };
	const marDice: MarDie[] = [
		{ face: 4, group: 'start',     canceled: true },
		{ face: 1, group: 'start',     canceled: false },
		{ face: 6, group: 'help',      canceled: false },
		{ face: 4, group: 'interfere', canceled: false },
	];
	// Result = distinct faces among the surviving pool dice (interference excluded).
	const marResult = new Set(
		marDice.filter((d) => d.group !== 'interfere' && !d.canceled).map((d) => d.face),
	).size;

	// One border for every die (owner ruling): a die is a die, and the group's
	// label above it — white / green / red — is what says whose it is. The
	// help and interference dice carry the family's chip fill as a second,
	// quieter cue, matching DiceRollPanel's own red-tinted `.die.int`.
	function dieFill(group: MarDie['group']): string {
		if (group === 'help') return 'var(--color-chip-green-bg)';
		if (group === 'interfere') return 'var(--color-chip-red-bg)';
		return 'var(--color-surface-2)';
	}

	// Pip centres for a die face, expressed in a 0–1 unit square.
	function pips(face: number): [number, number][] {
		const a = 0.28, b = 0.5, c = 0.72;
		const layout: Record<number, [number, number][]> = {
			1: [[b, b]],
			2: [[a, a], [c, c]],
			3: [[a, a], [b, b], [c, c]],
			4: [[a, a], [c, a], [a, c], [c, c]],
			5: [[a, a], [c, a], [b, b], [a, c], [c, c]],
			6: [[a, a], [c, a], [a, b], [c, b], [a, c], [c, c]],
		};
		return layout[face] ?? [];
	}

	// Example player's ranks for the Rankings-tab replicas (status = 6 − rank).
	const exRankStrip: { label: string; rank: number; status: number }[] = [
		{ label: 'Power', rank: 2, status: 4 },
		{ label: 'Knowledge', rank: 1, status: 5 },
		{ label: 'Esteem', rank: 4, status: 2 },
	];

	// The four asset types for the Assets-tab reference grid. The glyph comes
	// from the real AssetTypeIcon so the help can never teach an icon the app
	// no longer draws; the badge is the .ex-asset-type replica of RetinueView's
	// .asset-type chip (restyle the two together).
	const assetTypes: { id: Asset['asset_type']; label: string; desc: string }[] = [
		{ id: 'holding', label: 'Holding', desc: 'Land and buildings.' },
		{ id: 'peer', label: 'Peer', desc: 'The people of the court.' },
		{ id: 'artifact', label: 'Artifact', desc: 'Trinkets, relics, and other objects.' },
		{ id: 'resource', label: 'Resource', desc: 'Materials, traditions, logistics.' },
	];

	// The twelve plans, grouped by category, for the Plans-tab reference grid.
	// Names come from the canonical PLAN_SHORT and the order from TRACK_ORDER —
	// the same two the in-game plan sheet reads — so the help can't drift from
	// what a player sees when they prepare.
	const planGroups: { category: string; plans: string[] }[] = (
		[
			['Power', 'power'],
			['Knowledge', 'knowledge'],
			['Esteem', 'esteem'],
		] as const
	).map(([category, track]) => ({
		category,
		plans: TRACK_ORDER[track].map((pt) => PLAN_SHORT[pt]),
	}));
</script>

<div class="help" class:panel>
	<nav class="tabs" aria-label="Help topics">
		{#each tabs as tab}
			<button
				type="button"
				class="tab"
				class:active={active === tab.id}
				aria-pressed={active === tab.id}
				onclick={() => (active = tab.id)}
			>
				{tab.label}
			</button>
		{/each}
	</nav>

	<div class="body">
		{#if active === 'goal'}
			<p><em>Uneasy Lies the Head</em> is a story game for 2-5 players in a royal court. 
				You each play a noble and the retinue around them, scheming for Power, Knowledge, and Esteem.</p>
			<p>There's no winner, but the game works best if everyone embraces the competitive, political nature of the game.</p>

			<div class="pace-pair">
				<div class="pace-item">
					<h5 class="pace-head">In real time</h5>
					<span class="pace-text">Everyone together, maybe in a voice or video call, making moves live.</span>
				</div>
				<div class="pace-item">
					<h5 class="pace-head">Play by post</h5>
					<span class="pace-text">Take your turn, close the tab, come back tomorrow.</span>
				</div>
			</div>

			<p>Either pace works — the game will always wait for you.</p>
			<p>The <em>chat</em> serves both styles: table conversation and a running log of everything the court has done, 
				so it's the quickest way to catch up.</p>
			<p><em>Profile → Notifications</em> sets how often the game reminds you it's your move.</p>
		{/if}

		{#if active === 'record'}
			<div class="record-intro">
				<figure class="diagram diagram-record">
					<svg viewBox="4 0 80 168" role="img" aria-label="The 13-row Public Record timeline, played top to bottom, with the Shake-Up marked after the last row.">
						{#each Array(13) as _, i}
							{@const y = 7 + i * 11}
							{@const engrailed = i === 3 || i === 7 || i === 11}
							<text x="16" y={y + 3} text-anchor="end" class="d-num">{i + 1}</text>
							<!-- The engrailed rows (4/8/12, where rankings update) read as a
							     heavier accent line, NOT the podium the Public Record and
							     chat carry: at this diagram's scale the 4-bar podium didn't
							     read (owner ruling), and the thicker gold line already says
							     "something happens here". -->
							<line x1="24" y1={y} x2="79" y2={y}
								stroke={engrailed ? 'var(--color-accent)' : 'var(--color-border-strong)'}
								stroke-width={engrailed ? 2 : 1} />
						{/each}
						<!-- The Shake-Up pseudo-row — a heavier ✶ glyph and dashed
						     tie-line, mirroring the Public Record sidebar's row-14
						     treatment (PublicRecord.svelte). -->
						<text x="17" y={SHAKEUP_ROW_Y + 4} text-anchor="end" class="d-shakeup-star">✶</text>
						<line x1="24" y1={SHAKEUP_ROW_Y} x2="79" y2={SHAKEUP_ROW_Y}
							stroke="var(--color-accent)" stroke-width="1.5" stroke-dasharray="2,2" />
						<text x="79" y={SHAKEUP_ROW_Y + 14} text-anchor="end" class="d-cap">The Shake-Up</text>
					</svg>
				</figure>
				<div class="record-text">
					<p>This timeline guides the game once the prologue is over.</p>
					<p>You step down it row by row — <em>setting a scene</em> on each, and <em>preparing plans</em> that land on later rows.</p>
					<p><em>Rankings</em> will only change at 3 points: after rows 4, 8, and 12.</p>
					<p>The finale (the Shake-Up) occurs after row 13.</p>
				</div>
			</div>
			<!-- Deliberately no endgame-mode section here (owner ruling,
			     2026-07-28): choosing how the game ends is table admin, and
			     confronting a new player with it in Help costs them attention
			     they need elsewhere. The row 7 → 8 vote panel is the right and
			     sufficient first contact — it appears exactly once, at the
			     moment it matters, and explains itself in full
			     (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §7). -->
		{/if}

		{#if active === 'plans'}
			<p>Twelve plans, split across three categories. Each blends roleplaying with a dice roll, and takes a few turns to resolve after you prepare it.</p>
			<p>Preparing plans higher in the columns will help more when ranks are updated.</p>

			<!-- One grid rather than three stacked columns: the cells are placed
			     into shared rows so tier N of Power sits level with tier N of
			     Knowledge and Esteem. That alignment is the point — the
			     sentence above only reads as true if the columns line up. -->
			<div class="plan-grid">
				{#each planGroups as group, gi}
					<h5 class="plan-cat" style:grid-column={gi + 1} style:grid-row={1}>{group.category}</h5>
					{#each group.plans as name, pi}
						<span class="plan-item" style:grid-column={gi + 1} style:grid-row={pi + 2}>{name}</span>
					{/each}
				{/each}
			</div>
		{/if}

		{#if active === 'rankings'}
			<p>After the prologue, everyone is ranked against each other in Power, Knowledge, and Esteem.</p>
			<p>The relevant rank feeds into the dice rolls for <em>plans</em>.</p>
			<p>The ranks will change after rows 4, 8, and 12 based on each player's plans in the category.</p>

			<figure class="diagram">
				<div class="ex-chip-row" aria-hidden="true">
					<div class="ex-chip waiting mine">
						<span class="ex-badge">2</span>
						<span class="ex-chip-body">
							<span class="ex-chip-name"><span class="ex-dot"></span>You</span>
							<span class="ex-ranks">
								<span class="mr"><span class="mr-cat">P</span>3</span>
								<span class="mr"><span class="mr-cat">K</span>2</span>
								<span class="mr"><span class="mr-cat">E</span>5</span>
							</span>
						</span>
					</div>
					<div class="ex-chip">
						<span class="ex-badge">1</span>
						<span class="ex-chip-body">
							<span class="ex-chip-name"><span class="ex-dot"></span>Alric</span>
							<span class="ex-ranks">
								<span class="mr"><span class="mr-cat">P</span>2</span>
								<span class="mr top"><span class="mr-cat">K</span>1</span>
								<span class="mr"><span class="mr-cat">E</span>4</span>
							</span>
						</span>
					</div>
				</div>
				<figcaption>
					Tap any chip to open it. A gold outline means the game is waiting on that player. 
					The red number counts that player's <em>assets</em> that are one tear from destruction
					but still have an empty <em>marginalia</em> slot.
				</figcaption>
			</figure>

			<figure class="diagram">
				<div class="rank-strip ex-rankstrip" aria-hidden="true">
					{#each exRankStrip as t}
						<div class="rank-cell">
							<span class="rank-label">{t.label}</span>
							<div class="rank-pair">
								<span class="rank-stat"><span class="rank-num">{t.rank}</span><span class="rank-sublabel">Rank</span></span>
								<span class="rank-stat"><span class="rank-num">{t.status}</span><span class="rank-sublabel">Status</span></span>
							</div>
						</div>
					{/each}
				</div>
				<figcaption>At the top of each Retinue, every track shows your <em>Rank</em> (how hard your own actions are) and <em>Status</em> (how hard others find you to target).</figcaption>
			</figure>
		{/if}

		{#if active === 'dice'}
			<p>Sometimes your plans will require rolling dice — two to start, plus one for each asset you <em>leverage</em>.</p>
			<p>Other players can leverage their own assets to
				<span class="w-aid">help</span> or <span class="w-int">interfere</span>.</p>
			<p>The game will tell you how many unique dice faces you need to succeed (called a Make). Less means failure (a Mar).</p>

			{#snippet dieFace(d: MarDie)}
				<svg class="die" class:canceled={d.canceled} viewBox="0 0 36 36" aria-hidden="true">
					<rect x="2" y="2" width="32" height="32" rx="7"
						fill={dieFill(d.group)} stroke="var(--color-border-strong)"
						stroke-width="1.5" />
					{#each pips(d.face) as [px, py]}
						<circle cx={2 + px * 32} cy={2 + py * 32} r="3.2" fill="var(--color-text)" />
					{/each}
					{#if d.canceled}
						<!-- Struck through the middle, not diagonally: on a 4 either
						     diagonal runs straight over two of the pips, and the face
						     stops being countable. A horizontal rule also matches
						     DiceRollPanel's line-through on a cancelled die. -->
						<line x1="6" y1="18" x2="30" y2="18" stroke="var(--color-danger)" stroke-width="2.5" />
					{/if}
				</svg>
			{/snippet}

			<figure class="diagram diagram-dice">
				<h5 class="fig-head">Example</h5>
				<div class="dice-board" aria-hidden="true">
					<div class="dice-group">
						<span class="dice-label">Start</span>
						<div class="dice-set">
							{#each marDice.filter((d) => d.group === 'start') as d}{@render dieFace(d)}{/each}
						</div>
					</div>
					<div class="dice-group">
						<span class="dice-label aid">Help</span>
						<div class="dice-set">
							{#each marDice.filter((d) => d.group === 'help') as d}{@render dieFace(d)}{/each}
						</div>
					</div>
					<div class="dice-sep"></div>
					<div class="dice-group">
						<span class="dice-label int">Interfere</span>
						<div class="dice-set">
							{#each marDice.filter((d) => d.group === 'interfere') as d}{@render dieFace(d)}{/each}
						</div>
					</div>
				</div>
				<div class="dice-result">
					<span class="dice-count">{marResult}</span> distinct faces ·
					<span class="dice-count">{marDifficulty}</span> needed
				</div>
				<div class="dice-mar-wrap"><span class="dice-mar">MAR</span></div>
				<figcaption>You leveraged no assets, so you rolled your two starting dice. One player
					<span class="w-aid">helped</span>, another <span class="w-int">interfered</span> — their 4
					cancels your 4, leaving a 1 and a 6. Two distinct faces, one short of the difficulty.</figcaption>
			</figure>
		{/if}

		{#if active === 'assets'}
			<p>Everything your character controls is an asset. There are four types:</p>

			<div class="type-grid">
				{#each assetTypes as t}
					<div class="type-item">
						<span class="type-head">
							<AssetTypeIcon type={t.id} size={20} />
							<span class="ex-asset-type">{t.label}</span>
						</span>
						<span class="type-desc">{t.desc}</span>
					</div>
				{/each}
			</div>
			<!-- <p class="type-note">
				You'll meet the <em>glyph</em> on its own wherever space is tight — picker rows, prologue
				cards — and the <em>badge</em> on the asset's card in a Retinue.
			</p> -->

			<p>
				Each asset has up to four <em>marginalia</em> — descriptive words or phrases that flesh out the asset.
				Writing marginalia is one of the best tools you have to set the stakes of the story.
			</p>
			<p>
				Adding and removing marginalia fundamentally changes the asset in the fiction.
				If a flaw gets torn off, that might represent character growth. 
				If a belief or a utility is broken, it might be a tragedy.
			</p>

			<figure class="diagram">
				<div class="ex-asset main" aria-hidden="true">
					<div class="ex-asset-head">
						<span class="ex-asset-name">Lady Mirabel</span>
						<!-- Status glyphs + type chip in one right-aligned cluster, mirroring
						     the real asset card's .tile-head-meta. -->
						<span class="ex-meta">
							<span class="ex-star" aria-hidden="true"><svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor" stroke="none"><path d="M12 17.75l-6.172 3.245l1.179 -6.873l-5 -4.867l6.9 -1l3.086 -6.253l3.086 6.253l6.9 1l-5 4.867l1.179 6.873z" /></svg></span>
							<span class="ex-lev" aria-hidden="true"><svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="3" /><circle cx="8" cy="8" r="1.2" fill="currentColor" stroke="none" /><circle cx="16" cy="8" r="1.2" fill="currentColor" stroke="none" /><circle cx="12" cy="12" r="1.2" fill="currentColor" stroke="none" /><circle cx="8" cy="16" r="1.2" fill="currentColor" stroke="none" /><circle cx="16" cy="16" r="1.2" fill="currentColor" stroke="none" /></svg></span>
							<span class="ex-eye known" aria-hidden="true"><svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" /><circle cx="12" cy="12" r="3" /></svg><span class="corner-badge known">1</span></span>
							<span class="ex-eye hidden" aria-hidden="true"><svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" /><circle cx="12" cy="12" r="3" /><line x1="3" y1="21" x2="21" y2="3" /></svg><span class="corner-badge hidden">2</span></span>
							<span class="ex-asset-type">Peer</span>
						</span>
					</div>
					<div class="ex-mgrid">
						<span class="m-tile ex-mtile">Silver-tongued</span>
						<span class="m-tile ex-mtile titled">Second heir<CrownGlyph mark={{ role: 'successor', ordinal: 2 }} size={14} /></span>
						<span class="m-tile ex-mtile torn">Old war wound</span>
						<span class="m-tile ex-mtile empty">+</span>
					</div>
				</div>
				<figcaption>
					<div class="ex-legend">
						<span class="ex-leg-row">
							<span class="ex-star" aria-hidden="true"><svg viewBox="0 0 24 24" width="15" height="15" fill="currentColor" stroke="none"><path d="M12 17.75l-6.172 3.245l1.179 -6.873l-5 -4.867l6.9 -1l3.086 -6.253l3.086 6.253l6.9 1l-5 4.867l1.179 6.873z" /></svg></span>
							<span class="ex-leg-text">Your <em>main character</em> — a peer you play.</span>
						</span>
						<span class="ex-leg-row">
							<span class="ex-lev sm"><svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="3" /><circle cx="8" cy="8" r="1.2" fill="currentColor" stroke="none" /><circle cx="16" cy="8" r="1.2" fill="currentColor" stroke="none" /><circle cx="12" cy="12" r="1.2" fill="currentColor" stroke="none" /><circle cx="8" cy="16" r="1.2" fill="currentColor" stroke="none" /><circle cx="16" cy="16" r="1.2" fill="currentColor" stroke="none" /></svg></span>
							<span class="ex-leg-text">A <em>die</em> after the name means it's <em>leveraged</em> for a roll, until you refresh it.</span>
						</span>
						<span class="ex-leg-row">
							<span class="ex-eye sm known"><svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" /><circle cx="12" cy="12" r="3" /></svg></span>
							<span class="ex-leg-text"><em>Secrets</em> held by the asset that are known to you.</span>
						</span>
						<span class="ex-leg-row">
							<span class="ex-eye sm hidden"><svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" /><circle cx="12" cy="12" r="3" /><line x1="3" y1="21" x2="21" y2="3" /></svg></span>
							<span class="ex-leg-text">Secrets hidden from you. To learn them, you must <em>take</em> or <em>break</em> the asset.</span>
						</span>
						<span class="ex-leg-row">
							<CrownGlyph mark={{ role: 'successor', ordinal: 2 }} size={14} />
							<span class="ex-leg-text">A <em>crown</em> on a peer's marginalia means they are in the line of succession for the throne.</span>
						</span>
						<span class="ex-leg-row">
							<span class="ex-strike">torn</span>
							<span class="ex-leg-text">A <em>torn</em> marginalia, caused by <em>breaking</em> the asset. Tear them all and the asset is destroyed.</span>
						</span>
					</div>
				</figcaption>
			</figure>
		{/if}
	</div>

	<footer class="help-footer">
		<span>Something confusing or broken?</span>
		<button type="button" class="feedback" onclick={onFeedback}>Send feedback</button>
	</footer>
</div>

<style>
	.help {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		font-family: var(--font-serif);
		font-size: 0.95rem;
		/* Own query container for the tab row below. It has to be THIS element
		   rather than the phase column's named `column`: the help mounts both
		   inline in the lobby (inside that column) and in the "?" sheet (a
		   fixed-position dialog outside it), and only its own width is a fact
		   in both places. */
		container-type: inline-size;
	}

	/* Tabs never scroll. The old horizontal scroller hid its overflow behind a
	   suppressed scrollbar, and at 360 — the floor of the design band
	   (STYLE_GUIDE "Layout widths") — five tabs already overran the panel by
	   ~10px with nothing on screen to say so.

	   Mobile-first default: three equal columns, so six tabs split 3 + 3.
	   Plain `flex-wrap` was the obvious alternative and it's wrong — it fills
	   the first row greedily and strands a lone sixth tab on the second. */
	.tabs {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.3rem 0.25rem;
		padding-bottom: 0.25rem;
	}

	/* One row once the column can hold all six at their natural widths (~388px
	   total). Queried against .help rather than the viewport because the two
	   mount points disagree: at a 800px viewport the "?" sheet is 406px wide
	   (one row fits) while the lobby's inline column is only 364 (it does
	   not) — a viewport query orphans "Dice" in the lobby. The 400 threshold
	   is the documented container literal nearest that 388, and clears it. */
	@container (min-width: 400px) {
		.tabs {
			display: flex;
			flex-wrap: wrap;
			justify-content: center;
		}
	}

	.tab {
		flex-shrink: 0;
		min-height: 44px;
		padding: 0.4rem 0.5rem;
		font-family: var(--font-serif);
		font-size: 0.82rem;
		color: var(--color-text-muted);
		background: var(--color-surface-2);
		border: 1px solid var(--color-border-strong);
		border-radius: 999px;
		cursor: pointer;
		/* No nowrap: in the narrow grid below, an equal third of a 344px column
		   leaves "Public Record" under 2px of slack. Letting it take two lines
		   at the very bottom of the range beats overflowing the pill. */
	}
	.tab:hover { background: var(--color-border); color: var(--color-text); }
	.tab:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
	.tab.active {
		color: var(--color-bg);
		background: var(--color-accent);
		border-color: var(--color-accent);
	}

	.body { color: var(--color-text); line-height: 1.55; }

	/* Lobby (inline): the footer sits in normal flow right below the help body.
	   No reserved min-height (blank space on short tabs) and no sticky pinning
	   (it floated over content while the phase view scrolled) — content height
	   varies per tab and the footer simply follows it. */

	/* Panel (? sheet) below the chat dock: the help is a fixed height that fills
	   the sheet, and the BODY is the only scroll region. Short tabs leave
	   non-scrollable blank above the pinned footer; the tallest tab scrolls real
	   content only. The 130px allowance covers the sheet header, its padding,
	   and the "How to play" title above this component. */
	@media (max-width: 789px) {
		.help.panel { height: calc(85dvh - 130px); }
		.help.panel .tabs { flex-shrink: 0; }
		.help.panel .body { flex: 1 1 auto; min-height: 0; overflow-y: auto; }
		.help.panel .help-footer { flex-shrink: 0; }
	}
	/* No list rules here on purpose: every tab now teaches with a grid or a
	   diagram rather than a bulleted list. Add them back alongside the markup
	   if a list ever returns. */
	.body :global(p) { margin: 0 0 0.6rem; }

	.diagram {
		margin: 0.75rem 0 0.5rem;
		padding: 0.75rem;
		background: var(--color-surface-sunken, var(--color-surface-2));
		border: 1px solid var(--color-border);
		border-radius: 6px;
	}
	.diagram svg { display: block; width: 100%; height: auto; }

	/* Record tab: the timeline sits to the LEFT of the prose, mirroring where the
	   Public Record rail lives during play, and closing the dead space that a
	   stacked short-and-wide timeline left underneath it. The row lines are also
	   ~half their old length so the diagram reads as a narrow left rail. */
	.record-intro { display: flex; gap: 0.9rem; align-items: flex-start; }
	.record-intro .diagram-record { flex: 0 0 auto; width: 150px; margin: 0; }
	.diagram-record svg { max-width: 100%; }
	.record-text { flex: 1 1 auto; min-width: 0; }
	.record-text :global(p):first-child { margin-top: 0; }
	/* Figure heading — same uppercase gold small-label as .pace-head/.plan-cat,
	   so a worked example announces itself before the reader has to infer it
	   from the caption underneath. */
	.fig-head {
		margin: 0 0 0.55rem;
		font-size: 0.72rem;
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: var(--color-accent);
	}
	.diagram figcaption {
		margin-top: 0.6rem;
		font-size: 0.85rem;
		color: var(--color-text-muted);
		line-height: 1.45;
	}
	.d-num { font-family: var(--font-serif); font-size: 7px; fill: var(--color-text-muted); }
	.d-cap { font-family: var(--font-serif); font-size: 8px; fill: var(--color-accent); }
	/* Heavier than .d-num — mirrors the Public Record sidebar's rail glyph,
	   which is likewise bigger and bolder than the engrailed podium dividers. */
	.d-shakeup-star { font-size: 11px; fill: var(--color-accent); }

	/* ── Dice example (Dice tab) ─────────────────────────────────────────── */
	/* Labels and the result line are HTML so they share the prose size; only
	   the dice (and the MAR stamp) are graphical. */
	/* Group gap comfortably wider than the 6px between dice inside a set —
	   at 0.6rem the second Start die and the Help die read as one row of three. */
	.dice-board { display: flex; justify-content: center; align-items: flex-end; gap: 0.9rem; }
	.dice-group { display: flex; flex-direction: column; align-items: center; gap: 0.3rem; }
	/* Full-contrast labels, not muted: they're the figure's legend, and the two
	   coloured ones are the same green/red the prose above marks "help" and
	   "interfere" with. A grey "Start" beside them read as the odd one out. */
	.dice-label { font-size: 0.85rem; color: var(--color-text); }
	.dice-label.aid { color: var(--color-success); }
	.dice-label.int { color: var(--color-danger); }
	.dice-set { display: flex; gap: 6px; }
	.diagram .die { width: 30px; height: 30px; flex-shrink: 0; }
	/* Faded, but its face must stay countable — the caption says "their 4
	   cancels your 4", and at 0.4 the pips were too dim to check that. */
	.die.canceled { opacity: 0.62; }
	.dice-sep { align-self: stretch; width: 0; border-left: 1px dashed var(--color-border); margin: 0 0.15rem; }
	.dice-result { margin: 0.65rem 0 0.45rem; text-align: center; font-size: 0.85rem; color: var(--color-text-muted); }
	/* Standalone numeric counters — the one place bold is allowed. */
	.dice-count { font-weight: 700; color: var(--color-text); }
	.dice-mar-wrap { text-align: center; }
	.dice-mar {
		display: inline-block;
		border: 2px solid var(--color-danger);
		border-radius: 6px;
		padding: 0.05rem 0.7rem;
		background: var(--color-chip-red-bg);
		color: var(--color-danger);
		font-size: 1rem;
		letter-spacing: 0.12em;
	}

	/* The two mechanics, marked in prose and caption with the same green/red the
	   figure's labels and the live roll panel (aid vs interference) use. Colour
	   here identifies WHICH mechanic, the same job the italics do for asset
	   names — it isn't emphasis. */
	.w-aid { color: var(--color-success); }
	.w-int { color: var(--color-danger); }

	/* ── Pace pair (Goal tab) ────────────────────────────────────────────── */
	/* Two cards rather than a sentence each: the whole point is that the two
	   styles are equals, and side-by-side says that faster than prose can. */
	.pace-pair {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.4rem;
		margin: 0.5rem 0 0.7rem;
	}
	.pace-item {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		padding: 0.5rem 0.55rem;
		background: var(--color-surface-sunken, var(--color-surface-2));
		border: 1px solid var(--color-border);
		border-radius: 4px;
	}
	.pace-head {
		margin: 0;
		font-size: clamp(0.7rem, 2.2vw, 0.85rem);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-accent);
	}
	.pace-text {
		font-size: clamp(0.68rem, 2.2vw, 0.88rem);
		line-height: 1.35;
		color: var(--color-text-muted);
	}

	/* ── Plans reference grid ────────────────────────────────────────────── */
	/* Always three columns (one per category). Type shrinks on narrow widths
	   rather than dropping columns — cramped is acceptable, hidden is not.
	   `repeat(4, 1fr)` on the plan rows gives every row the height of the
	   tallest cell, so a two-line name never knocks its neighbours out of
	   line; the header row stays auto. */
	.plan-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		grid-template-rows: auto repeat(4, 1fr);
		gap: 0.4rem;
		margin-top: 0.4rem;
	}
	.plan-cat {
		margin: 0 0 0.1rem;
		font-size: clamp(0.68rem, 2vw, 0.85rem);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		text-align: center;
		color: var(--color-accent);
	}
	/* Names only (owner ruling): the one-line descriptions turned the tab into
	   a wall of text new players skipped, and PlanPanel already shows each
	   description on the card at the moment of choosing. Centred in a
	   fixed-height cell so the twelve read as a grid, not twelve paragraphs. */
	.plan-item {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 48px;
		padding: 0.5rem 0.4rem;
		text-align: center;
		text-wrap: balance;
		background: var(--color-surface-sunken, var(--color-surface-2));
		border: 1px solid var(--color-border);
		border-radius: 4px;
		font-size: clamp(0.78rem, 2.9vw, 1rem);
		line-height: 1.25;
		color: var(--color-text);
	}

	/* ── Header-chip replica (Rankings tab) ──────────────────────────────── */
	/* Mirrors .member/.risk-badge in routes/table/[id]/+page.svelte. Kept as a
	   local replica rather than a shared file (same call as the other help
	   diagrams) — but the two must be restyled together, or the legend starts
	   teaching a chip the header no longer draws. */
	.ex-chip-row {
		display: flex; flex-wrap: wrap; justify-content: center;
		gap: 0.6rem; /* clears the badges' 6px overhang between chips */
	}
	.ex-chip {
		position: relative; /* the badge's containing block */
		display: flex; align-items: center; gap: 0.4rem;
		min-height: 44px; padding: 0.3rem 0.7rem;
		background: var(--color-surface-2); border: 1px solid var(--color-border); border-radius: 999px;
		width: fit-content;
	}
	.ex-chip.waiting { border-color: var(--color-accent); }
	.ex-chip.waiting.mine {
		box-shadow: 0 0 0 1px var(--color-accent), 0 0 8px color-mix(in srgb, var(--color-accent) 45%, transparent);
	}
	.ex-badge {
		position: absolute; top: -6px; right: -6px;
		min-width: 20px; height: 20px; padding: 0 5px; box-sizing: border-box;
		display: inline-flex; align-items: center; justify-content: center;
		border-radius: 999px; font-size: 0.78rem; font-weight: 600; line-height: 1;
		font-variant-numeric: tabular-nums;
		background: var(--color-surface-2); border: 1px solid var(--color-danger-muted);
		color: var(--color-danger);
	}
	.ex-chip-body { display: flex; flex-direction: column; align-items: center; gap: 0.12rem; }
	.ex-chip-name { display: inline-flex; align-items: center; gap: 0.4rem; font-size: 0.85rem; color: var(--color-text); }
	/* Stands in for a player colour. Deliberately NOT a real palette entry:
	   playerColor.ts owns those and they're per-game data, so a diagram can't
	   name one honestly. */
	.ex-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--color-highlight); flex-shrink: 0; }
	.ex-ranks { display: flex; gap: 0.4rem; font-size: 0.62rem; line-height: 1; color: var(--color-text-muted); font-variant-numeric: tabular-nums; }

	/* ── Rank-strip replica (Rankings tab) ───────────────────────────────── */
	/* .rank-strip itself (grid/colors/etc.) comes from shared/rankStrip.css;
	   this just centres it in the narrow help diagram box. */
	.ex-rankstrip { width: 100%; max-width: 340px; margin: 0 auto; }

	/* ── Asset-type reference grid (Assets tab) ──────────────────────────── */
	/* Two columns at every width, same call as .plan-grid: type shrinks rather
	   than columns collapsing, so the four types always read as one set. */
	.type-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 0.4rem;
		margin: 0.25rem 0 0.6rem;
	}
	.type-item {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		padding: 0.45rem 0.5rem;
		background: var(--color-surface-sunken, var(--color-surface-2));
		border: 1px solid var(--color-border);
		border-radius: 4px;
	}
	/* Glyph and badge sit side by side so the two forms are learned as one
	   pair. The glyph keeps AssetTypeIcon's own --color-text ink — that is how
	   it renders in the picker rows this is teaching. */
	.type-head { display: flex; align-items: center; gap: 0.45rem; flex-wrap: wrap; }
	.type-desc {
		font-size: clamp(0.68rem, 2.2vw, 0.88rem);
		line-height: 1.3;
		color: var(--color-text-muted);
	}
	.type-note { font-size: 0.85rem; color: var(--color-text-muted); }

	/* ── Example asset card (Assets tab) ─────────────────────────────────── */
	.ex-asset {
		background: var(--color-surface); border: 1px solid var(--color-border-strong); border-radius: 8px;
		padding: 0.6rem 0.7rem; display: flex; flex-direction: column; gap: 0.5rem;
		width: 100%; max-width: 340px; margin: 0 auto;
	}
	.ex-asset.main { border-color: var(--color-accent); }
	/* Name on the left; status glyphs + type chip in one right-aligned cluster. */
	.ex-asset-head { display: flex; align-items: center; justify-content: space-between; gap: 0.5rem; }
	.ex-asset-name { font-size: 0.95rem; color: var(--color-text); white-space: nowrap; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
	.ex-meta { display: inline-flex; align-items: center; gap: 0.7rem; flex-shrink: 0; padding-left: 0.75rem; }
	.ex-star { color: var(--color-accent); display: inline-flex; align-items: center; }
	.ex-eye { position: relative; display: inline-flex; align-items: center; flex-shrink: 0; }
	.ex-eye.known { color: var(--color-accent); }
	.ex-eye.hidden { color: var(--color-text-muted); }
	/* Replica of RetinueView's .asset-type chip — used by the example card here
	   AND by the type grid above, so restyle all three together. */
	.ex-asset-type { flex-shrink: 0; font-size: 0.7rem; background: var(--color-border-warm); color: var(--color-accent); padding: 0.1rem 0.4rem; border-radius: 3px; text-transform: uppercase; letter-spacing: 0.05em; }
	.ex-mgrid { display: grid; grid-template-columns: 1fr 1fr; gap: 0.35rem; }
	/* Base look (background/border/color/torn/titled/empty) comes from
	   shared/marginaliaTile.css via the .m-tile class alongside .ex-mtile;
	   this is just the static replica's own sizing and "+" glyph. */
	.ex-mtile { min-height: 38px; }
	.ex-mtile.empty { justify-content: center; color: var(--color-text-faint-warm); font-size: 1.2rem; }

	/* Legend under the example card */
	.ex-legend { display: flex; flex-direction: column; gap: 0.5rem; }
	.ex-leg-row { display: flex; align-items: flex-start; gap: 0.5rem; }
	.ex-leg-row > :first-child { flex-shrink: 0; margin-top: 0.05rem; }
	.ex-eye.sm { flex-shrink: 0; }
	.ex-strike { flex-shrink: 0; text-decoration: line-through; opacity: 0.6; color: var(--color-text-secondary); font-size: 0.78rem; }
	/* Leveraged die — inline after the name in the example card, and in the legend. */
	.ex-lev { color: var(--color-leveraged); display: inline-flex; align-items: center; }
	.ex-lev svg { vertical-align: -0.18em; }
	.ex-lev.sm { flex-shrink: 0; }

	.help-footer {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem;
		padding-top: 0.75rem;
		border-top: 1px solid var(--color-border);
		font-size: 0.9rem;
		color: var(--color-text-muted);
	}
	.feedback {
		min-height: 44px;
		display: inline-flex;
		align-items: center;
		padding: 0 0.9rem;
		font-family: var(--font-serif);
		font-size: 0.85rem;
		color: var(--color-bg);
		background: var(--color-accent);
		border-radius: 999px;
		text-decoration: none;
	}
	.feedback:hover { background: var(--color-accent-hover); }
	.feedback:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
</style>
