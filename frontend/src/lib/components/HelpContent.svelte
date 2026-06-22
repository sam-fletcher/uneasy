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
	import { feedbackHref } from '$lib/feedback';
	import { PLAN_SHORT, PLAN_DESCRIPTION, TRACK_ORDER } from './plans/shared';

	// Panel mode (the ? sheet): the help fills the sheet to a fixed height and the
	// body scrolls internally, so the footer is pinned and blank space is never
	// scrollable. The lobby leaves it false (footer sits in normal flow).
	let { panel = false }: { panel?: boolean } = $props();

	type TabId = 'record' | 'plans' | 'rankings' | 'dice' | 'assets';
	const tabs: { id: TabId; label: string }[] = [
		{ id: 'record', label: 'Public Record' },
		{ id: 'rankings', label: 'Rankings' },
		{ id: 'plans', label: 'Plans' },
		{ id: 'assets', label: 'Assets' },
		{ id: 'dice', label: 'Dice' },
	];

	let active = $state<TabId>('record');

	// Worked Mar example for the Dice tab. The pool is 2 starting dice + 1 help
	// die; a single interference die cancels the matching 4. The survivors (a 1
	// and a 6) make two distinct faces — short of a difficulty of 3.
	const marDifficulty = 3;
	type MarDie = { face: number; group: 'start' | 'help' | 'interfere'; x: number; canceled: boolean };
	const marDice: MarDie[] = [
		{ face: 4, group: 'start',     x: 10,  canceled: true },
		{ face: 1, group: 'start',     x: 46,  canceled: false },
		{ face: 6, group: 'help',      x: 100, canceled: false },
		{ face: 4, group: 'interfere', x: 198, canceled: true },
	];
	// Result = distinct faces among the surviving pool dice (interference excluded).
	const marResult = new Set(
		marDice.filter((d) => d.group !== 'interfere' && !d.canceled).map((d) => d.face),
	).size;

	function dieBorder(group: MarDie['group']): string {
		if (group === 'help') return 'var(--color-accent)';
		if (group === 'interfere') return 'var(--color-danger)';
		return 'var(--color-border-strong)';
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

	// The twelve plans, grouped by category, for the Plans-tab reference grid.
	// Names and descriptions are derived from the canonical PLAN_SHORT /
	// PLAN_DESCRIPTION used in-game, so the help and gameplay never drift.
	const planGroups: { category: string; plans: { name: string; desc: string }[] }[] = (
		[
			['Power', 'power'],
			['Knowledge', 'knowledge'],
			['Esteem', 'esteem'],
		] as const
	).map(([category, track]) => ({
		category,
		plans: TRACK_ORDER[track].map((pt) => ({
			name: PLAN_SHORT[pt],
			desc: PLAN_DESCRIPTION[pt],
		})),
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
		{#if active === 'record'}
			<div class="record-intro">
				<figure class="diagram diagram-record">
					<svg viewBox="4 0 80 156" role="img" aria-label="The 13-row Public Record timeline, played top to bottom, with the Shake-Up after the last row.">
						{#each Array(13) as _, i}
							{@const y = 7 + i * 11}
							{@const engrailed = i === 3 || i === 7 || i === 11}
							<text x="16" y={y + 3} text-anchor="end" class="d-num">{i + 1}</text>
							<line x1="24" y1={y} x2="79" y2={y}
								stroke={engrailed ? 'var(--color-accent)' : 'var(--color-border-strong)'}
								stroke-width={engrailed ? 2 : 1} />
						{/each}
						<text x="79" y="153" text-anchor="end" class="d-cap">↓ Shake-Up</text>
					</svg>
				</figure>
				<div class="record-text">
					<p>This timeline guides the game once the prologue is over.</p>
					<p>You step down it row by row — <em>setting a scene</em> on each, and <em>preparing plans</em> that land on later rows.</p>
					<p><em>Rankings</em> will only change at 3 points: after rows 4, 8, and 12.</p>
					<p>The finale (the Shake-Up) occurs after row 13.</p>
				</div>
			</div>
		{/if}

		{#if active === 'plans'}
			<p>Twelve plans, split across three categories. Each blends roleplaying with a dice roll, and takes a few turns to resolve after you prepare it.</p>
			<p>Preparing plans higher in the columns will help more when ranks are updated.</p>

			<div class="plan-grid">
				{#each planGroups as group}
					<div class="plan-col">
						<h5 class="plan-cat">{group.category}</h5>
						<div class="plan-list">
							{#each group.plans as plan}
								<div class="plan-item">
									<span class="plan-name">{plan.name}</span>
									<span class="plan-desc">{plan.desc}</span>
								</div>
							{/each}
						</div>
					</div>
				{/each}
			</div>
		{/if}

		{#if active === 'rankings'}
			<p>After the prologue, everyone is ranked against each other in Power, Knowledge, and Esteem.</p>
			<p>The relevant rank feeds into the dice rolls for plans.</p>
			<p>The ranks will change after rows 4, 8, and 12 based on each player's <em>plans</em> in the category.</p>

			<figure class="diagram">
				<div class="ex-chip" aria-hidden="true">
					<span class="ex-chip-body">
						<span class="ex-chip-name"><span class="ex-dot"></span>Alric</span>
						<span class="ex-ranks">
							<span class="ex-mr"><span class="ex-mr-cat">P</span>2</span>
							<span class="ex-mr top"><span class="ex-mr-cat">K</span>1</span>
							<span class="ex-mr"><span class="ex-mr-cat">E</span>4</span>
						</span>
					</span>
				</div>
				<figcaption>Tap a chip to open that player's <em>Retinue</em>.</figcaption>
			</figure>

			<figure class="diagram">
				<div class="ex-rankstrip" aria-hidden="true">
					{#each exRankStrip as t}
						<div class="ex-rank-cell">
							<span class="ex-rank-label">{t.label}</span>
							<div class="ex-rank-pair">
								<span class="ex-rank-stat"><span class="ex-rank-num">{t.rank}</span><span class="ex-rank-sub">Rank</span></span>
								<span class="ex-rank-stat"><span class="ex-rank-num">{t.status}</span><span class="ex-rank-sub">Status</span></span>
							</div>
						</div>
					{/each}
				</div>
				<figcaption>At the top of each Retinue, every track shows your <em>Rank</em> (how hard your own actions are) and <em>Status</em> (how hard others find you to target).</figcaption>
			</figure>
		{/if}

		{#if active === 'dice'}
			<p>Roll dice when an outcome is in doubt — two to start, plus one for each asset you <em>leverage</em>.</p>
			<p>Other players can leverage their own assets to <em>help</em> or <em>interfere</em>.</p>
			<p>Each interference die cancels a matching die. Count the <em>distinct faces</em> left standing: meet the difficulty to <em>Make</em>, fall short and you <em>Mar</em>.</p>

			{#snippet dieFace(d: MarDie)}
				<svg class="die" class:canceled={d.canceled} viewBox="0 0 36 36" aria-hidden="true">
					<rect x="2" y="2" width="32" height="32" rx="7"
						fill="var(--color-surface-2)" stroke={dieBorder(d.group)}
						stroke-width={d.group === 'start' ? 1.5 : 2} />
					{#each pips(d.face) as [px, py]}
						<circle cx={2 + px * 32} cy={2 + py * 32} r="3.2" fill="var(--color-text)" />
					{/each}
					{#if d.canceled}
						<line x1="8" y1="28" x2="28" y2="8" stroke="var(--color-danger)" stroke-width="2.5" />
					{/if}
				</svg>
			{/snippet}

			<figure class="diagram diagram-dice">
				<div class="dice-board" aria-hidden="true">
					<div class="dice-group">
						<span class="dice-label">Start</span>
						<div class="dice-set">
							{#each marDice.filter((d) => d.group === 'start') as d}{@render dieFace(d)}{/each}
						</div>
					</div>
					<div class="dice-group">
						<span class="dice-label">Help</span>
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
				<div class="dice-result">{marResult} distinct faces · difficulty {marDifficulty}</div>
				<div class="dice-mar-wrap"><span class="dice-mar">MAR</span></div>
				<figcaption>The interference die cancels your matching 4, leaving a 1 and a 6 — two distinct faces, short of the difficulty. A Mar.</figcaption>
			</figure>
		{/if}

		{#if active === 'assets'}
			<p>Everything your character controls is an asset. There are 4 types:</p>
			<ul>
				<li><em>Holdings</em> — land and buildings.</li>
				<li><em>Peers</em> — the people of the court.</li>
				<li><em>Artifacts</em> — trinkets, relics, and other objects.</li>
				<li><em>Resources</em> — anything from materials to traditions to logistics.</li>
			</ul>
			<p>Each asset has up to 4 <em>marginalia</em> — descriptive words or phrases that flesh out the asset.</p>

			<figure class="diagram">
				<div class="ex-asset main" aria-hidden="true">
					<div class="ex-asset-head">
						<span class="ex-asset-name">Lady Mirabel <span class="ex-star">★</span><span class="ex-lev"><svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="3" /><circle cx="8" cy="8" r="1.2" fill="currentColor" stroke="none" /><circle cx="16" cy="8" r="1.2" fill="currentColor" stroke="none" /><circle cx="12" cy="12" r="1.2" fill="currentColor" stroke="none" /><circle cx="8" cy="16" r="1.2" fill="currentColor" stroke="none" /><circle cx="16" cy="16" r="1.2" fill="currentColor" stroke="none" /></svg></span></span>
						<span class="ex-eyes">
							<span class="ex-eye known"><svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" /><circle cx="12" cy="12" r="3" /></svg><span class="ex-eye-num">1</span></span>
							<span class="ex-eye hidden"><svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" /><circle cx="12" cy="12" r="3" /><line x1="3" y1="21" x2="21" y2="3" /></svg><span class="ex-eye-num">2</span></span>
						</span>
						<span class="ex-asset-type">Peer</span>
					</div>
					<div class="ex-mgrid">
						<span class="ex-mtile">Silver-tongued</span>
						<span class="ex-mtile">Spymaster</span>
						<span class="ex-mtile torn">Old war wound</span>
						<span class="ex-mtile empty">+</span>
					</div>
				</div>
				<figcaption>
					<div class="ex-legend">
						<span class="ex-leg-row">
							<span class="ex-star">★</span>
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
		<a class="feedback" href={feedbackHref}>Send feedback</a>
	</footer>
</div>

<style>
	.help {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		font-family: var(--font-serif);
		font-size: 0.95rem;
	}

	/* Tabs: tuned to fit all five on a ~390px phone without scrolling; still
	   scrolls horizontally on narrower devices as a fallback. */
	.tabs {
		display: flex;
		gap: 0.25rem;
		overflow-x: auto;
		-webkit-overflow-scrolling: touch;
		scrollbar-width: none;
		margin: 0 -0.25rem;
		padding: 0 0.25rem 0.25rem;
	}
	.tabs::-webkit-scrollbar { display: none; }

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
		white-space: nowrap;
	}
	.tab:hover { background: var(--color-border); color: var(--color-text); }
	.tab:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
	.tab.active {
		color: var(--color-bg);
		background: var(--color-accent);
		border-color: var(--color-accent);
	}

	.body { color: var(--color-text); line-height: 1.55; }

	/* Lobby (inline): no reserved min-height — that pushed the feedback footer far
	   down shorter tabs, forcing a scroll through blank space to reach it.
	   Instead the footer is sticky to the bottom of the scrolling phase view, so
	   it stays in sight on every tab. */
	.help:not(.panel) .help-footer {
		position: sticky;
		bottom: 0;
		padding-bottom: 0.75rem;
		background: var(--color-bg);
	}

	/* Panel (? sheet) on mobile: the help is a fixed height that fills the sheet,
	   and the BODY is the only scroll region. Short tabs leave non-scrollable
	   blank above the pinned footer; the tallest tab scrolls real content only.
	   The 130px allowance covers the sheet header, its padding, and the
	   "How to play" title above this component. */
	@media (max-width: 699px) {
		.help.panel { height: calc(85dvh - 130px); }
		.help.panel .tabs { flex-shrink: 0; }
		.help.panel .body { flex: 1 1 auto; min-height: 0; overflow-y: auto; }
		.help.panel .help-footer { flex-shrink: 0; }
	}
	.body :global(p) { margin: 0 0 0.6rem; }
	.body :global(ul), .body :global(ol) {
		margin: 0 0 0.7rem;
		padding-left: 1.3rem;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}
	.body :global(li) { padding-left: 0.15rem; }

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
	.diagram figcaption {
		margin-top: 0.6rem;
		font-size: 0.85rem;
		color: var(--color-text-muted);
		line-height: 1.45;
	}
	.d-num { font-family: var(--font-serif); font-size: 7px; fill: var(--color-text-muted); }
	.d-cap { font-family: var(--font-serif); font-size: 8px; fill: var(--color-accent); }

	/* ── Dice example (Dice tab) ─────────────────────────────────────────── */
	/* Labels and the result line are HTML so they share the prose size; only
	   the dice (and the MAR stamp) are graphical. */
	.dice-board { display: flex; justify-content: center; align-items: flex-end; gap: 0.6rem; }
	.dice-group { display: flex; flex-direction: column; align-items: center; gap: 0.3rem; }
	.dice-label { font-size: 0.85rem; color: var(--color-text-muted); }
	.dice-label.int { color: var(--color-danger); }
	.dice-set { display: flex; gap: 6px; }
	.diagram .die { width: 30px; height: 30px; flex-shrink: 0; }
	.die.canceled { opacity: 0.4; }
	.dice-sep { align-self: stretch; width: 0; border-left: 1px dashed var(--color-border); margin: 0 0.15rem; }
	.dice-result { margin: 0.65rem 0 0.45rem; text-align: center; font-size: 0.85rem; color: var(--color-text-muted); }
	.dice-mar-wrap { text-align: center; }
	.dice-mar { display: inline-block; border: 2px solid var(--color-danger); border-radius: 6px; padding: 0.05rem 0.7rem; color: var(--color-danger); font-style: italic; font-size: 1rem; letter-spacing: 0.12em; }

	/* ── Plans reference grid ────────────────────────────────────────────── */
	/* Always three columns (one per category). Type shrinks on narrow widths
	   rather than dropping columns — cramped is acceptable, hidden is not. */
	.plan-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.3rem;
		margin-top: 0.25rem;
	}
	.plan-cat {
		margin: 0 0 0.35rem;
		font-size: clamp(0.68rem, 2vw, 0.85rem);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		text-align: center;
		color: var(--color-accent);
	}
	.plan-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.35rem; }
	.plan-item {
		padding: 0.4rem 0.45rem;
		background: var(--color-surface-sunken, var(--color-surface-2));
		border: 1px solid var(--color-border);
		border-radius: 4px;
	}
	/* Type scales with width: small enough for 3 columns on a phone, comfortably
	   larger in the roomy desktop modal. */
	.plan-name { display: block; font-size: clamp(0.74rem, 2.7vw, 1rem); line-height: 1.2; color: var(--color-text); }
	.plan-desc { display: block; margin-top: 0.15rem; font-size: clamp(0.68rem, 2.2vw, 0.88rem); line-height: 1.3; color: var(--color-text-muted); }

	/* ── Header-chip replica (Rankings tab) ──────────────────────────────── */
	.ex-chip {
		display: flex; align-items: center; gap: 0.4rem;
		min-height: 44px; padding: 0.3rem 0.7rem;
		background: #262626; border: 1px solid var(--color-border); border-radius: 999px;
		width: fit-content; margin: 0 auto;
	}
	.ex-chip-body { display: flex; flex-direction: column; align-items: center; gap: 0.12rem; }
	.ex-chip-name { display: inline-flex; align-items: center; gap: 0.4rem; font-size: 0.85rem; color: var(--color-text); }
	.ex-dot { width: 8px; height: 8px; border-radius: 50%; background: #7fb5d6; flex-shrink: 0; }
	.ex-ranks { display: flex; gap: 0.4rem; font-size: 0.62rem; line-height: 1; color: var(--color-text-muted); font-variant-numeric: tabular-nums; }
	.ex-mr { display: inline-flex; align-items: baseline; gap: 0.08rem; }
	.ex-mr-cat { color: var(--color-text-faint); font-size: 0.9em; font-weight: 600; }
	.ex-mr.top { color: var(--color-accent); font-weight: 700; }
	.ex-mr.top .ex-mr-cat { color: var(--color-accent); }

	/* ── Rank-strip replica (Rankings tab) ───────────────────────────────── */
	.ex-rankstrip {
		display: grid; grid-template-columns: repeat(3, 1fr); gap: 0.4rem;
		background: #161614; border: 1px solid var(--color-border-subtle);
		border-radius: 8px; padding: 0.5rem 0.6rem; width: 100%; max-width: 340px; margin: 0 auto;
	}
	.ex-rank-cell { display: flex; flex-direction: column; align-items: center; gap: 0.15rem; }
	.ex-rank-label { font-size: 0.66rem; color: var(--color-text-muted); text-transform: uppercase; letter-spacing: 0.05em; }
	.ex-rank-pair { display: flex; gap: 0.7rem; }
	.ex-rank-stat { display: flex; flex-direction: column; align-items: center; gap: 0.05rem; }
	.ex-rank-num { font-size: 1.05rem; color: var(--color-text); font-variant-numeric: tabular-nums; line-height: 1.1; }
	.ex-rank-sub { font-size: 0.66rem; color: var(--color-text-faint); }

	/* ── Example asset card (Assets tab) ─────────────────────────────────── */
	.ex-asset {
		background: #242420; border: 1px solid var(--color-border-strong); border-radius: 8px;
		padding: 0.6rem 0.7rem; display: flex; flex-direction: column; gap: 0.5rem;
		width: 100%; max-width: 340px; margin: 0 auto;
	}
	.ex-asset.main { border-color: var(--color-accent); }
	/* Name | eye | type — the eye sits centred between the star and the badge. */
	.ex-asset-head { display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: 0.5rem; }
	.ex-asset-name { justify-self: start; font-size: 0.95rem; color: var(--color-text); display: inline-flex; align-items: center; gap: 0.4rem; }
	.ex-star { font-size: 0.7rem; background: #4a3010; color: #e8c080; padding: 0.1rem 0.4rem; border-radius: 3px; }
	.ex-eyes { justify-self: center; display: inline-flex; align-items: center; gap: 0.45rem; }
	.ex-eye { display: inline-flex; align-items: center; gap: 1px; }
	.ex-eye.known { color: var(--color-accent); }
	.ex-eye.hidden { color: var(--color-text-muted); }
	.ex-eye-num { font-size: 0.7rem; font-weight: 600; }
	.ex-asset-type { justify-self: end; font-size: 0.7rem; background: var(--color-border-warm); color: var(--color-accent); padding: 0.1rem 0.4rem; border-radius: 3px; text-transform: uppercase; letter-spacing: 0.05em; }
	.ex-mgrid { display: grid; grid-template-columns: 1fr 1fr; gap: 0.35rem; }
	.ex-mtile {
		min-height: 38px; padding: 0.35rem 0.45rem; background: #1d1d1a; border: 1px solid #383530;
		border-radius: 5px; font-size: 0.78rem; line-height: 1.25; color: #cfcabd; display: flex; align-items: center;
	}
	.ex-mtile.torn { opacity: 0.45; text-decoration: line-through; }
	.ex-mtile.empty { background: transparent; border: 1px dashed #3a3a36; justify-content: center; color: #6a6a64; font-size: 1.2rem; }

	/* Legend under the example card */
	.ex-legend { display: flex; flex-direction: column; gap: 0.5rem; }
	.ex-leg-row { display: flex; align-items: flex-start; gap: 0.5rem; }
	.ex-leg-row > :first-child { flex-shrink: 0; margin-top: 0.05rem; }
	.ex-eye.sm { flex-shrink: 0; }
	.ex-strike { flex-shrink: 0; text-decoration: line-through; opacity: 0.6; color: #cfcabd; font-size: 0.78rem; }
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
