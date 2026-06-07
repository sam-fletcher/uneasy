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

	type TabId = 'record' | 'plans' | 'rankings' | 'dice' | 'assets';
	const tabs: { id: TabId; label: string }[] = [
		{ id: 'record', label: 'The Record' },
		{ id: 'plans', label: 'Plans' },
		{ id: 'rankings', label: 'Rankings' },
		{ id: 'dice', label: 'Dice' },
		{ id: 'assets', label: 'Assets' },
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
</script>

<div class="help">
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
			<h4>The Public Record</h4>
			<p>This timeline guides the game once the prologue is over.</p>
			<p>You step down it row by row — <em>setting a scene</em> on each, and <em>preparing plans</em> that land on later rows.</p>
			<p>Rankings can only change at 3 points: after rows 4, 8, and 12.</p>

			<figure class="diagram">
				<svg viewBox="0 0 240 150" role="img" aria-label="The 13-row Public Record timeline, played top to bottom, with the Shake-Up after the last row.">
					{#each Array(13) as _, i}
						{@const y = 6 + i * 10}
						{@const engrailed = i === 3 || i === 7 || i === 11}
						<line x1="40" y1={y + 9} x2="232" y2={y + 9}
							stroke={engrailed ? 'var(--color-accent)' : 'var(--color-border-strong)'}
							stroke-width={engrailed ? 2 : 1} />
						<text x="34" y={y + 7} text-anchor="end" class="d-num">{i + 1}</text>
					{/each}
					<text x="232" y="148" text-anchor="end" class="d-cap">↓ Shake-Up</text>
				</svg>
				<figcaption>13 rows, played top to bottom. The finale — the Shake-Up — comes after the last one.</figcaption>
			</figure>
		{/if}

		{#if active === 'plans'}
			<h4>Plans</h4>
			<p>Twelve plans, split across three categories — <em>Power</em>, <em>Knowledge</em>, and <em>Esteem</em>.</p>
			<p>Each one blends roleplaying with a dice roll, and takes a few turns to resolve after you prepare it.</p>
		{/if}

		{#if active === 'rankings'}
			<h4>Rankings</h4>
			<p>After the prologue, everyone is ranked against each other in Power, Knowledge, and Esteem.</p>
			<p>Your rank feeds into the dice rolls for your plans and your scenes.</p>

			<figure class="diagram">
				<svg viewBox="0 0 240 130" role="img" aria-label="Rankings ladder, rank 1 at the top down to rank 5 at the bottom.">
					{#each Array(5) as _, i}
						{@const rank = i + 1}
						{@const y = 6 + i * 24}
						<rect x="40" y={y} width="160" height="18" rx="3"
							fill="var(--color-surface-2)" stroke="var(--color-border-strong)" />
						<text x="48" y={y + 13} class="d-row">Rank {rank}</text>
					{/each}
					<text x="36" y="14" text-anchor="end" class="d-cap">↑ top</text>
				</svg>
				<figcaption>Rank 1 sits at the top of each category.</figcaption>
			</figure>
		{/if}

		{#if active === 'dice'}
			<h4>Dice</h4>
			<p>Roll dice when an outcome is in doubt — two to start, plus one for each asset you <em>leverage</em>.</p>
			<p>Allies can add <em>help</em> dice; rivals can add <em>interference</em> dice.</p>
			<p>Each interference die cancels a matching die. Count the <em>distinct faces</em> left standing: meet the difficulty to <em>Make</em>, fall short and you <em>Mar</em>.</p>

			<figure class="diagram">
				<svg viewBox="0 0 240 132" role="img" aria-label="A Mar example: two starting dice (4 and 1) plus a help die (6) face one interference die (4). The interference die cancels the matching 4, leaving a 1 and a 6 — two distinct faces, short of a difficulty of 3.">
					<text x="45" y="11" text-anchor="middle" class="d-glabel">Start</text>
					<text x="116" y="11" text-anchor="middle" class="d-glabel">Help</text>
					<text x="214" y="11" text-anchor="middle" class="d-glabel d-glabel-int">Interfere</text>
					<line x1="164" y1="16" x2="164" y2="58" stroke="var(--color-border)" stroke-width="1" stroke-dasharray="3 3" />
					{#each marDice as d}
						{@const s = 32}
						{@const y = 18}
						<g opacity={d.canceled ? 0.4 : 1}>
							<rect x={d.x} y={y} width={s} height={s} rx={6}
								fill="var(--color-surface-2)" stroke={dieBorder(d.group)}
								stroke-width={d.group === 'start' ? 1 : 1.5} />
							{#each pips(d.face) as [px, py]}
								<circle cx={d.x + px * s} cy={y + py * s} r="3" fill="var(--color-text)" />
							{/each}
							{#if d.canceled}
								<line x1={d.x + 5} y1={y + s - 5} x2={d.x + s - 5} y2={y + 5}
									stroke="var(--color-danger)" stroke-width="2" />
							{/if}
						</g>
					{/each}
					<text x="120" y="82" text-anchor="middle" class="d-calc">{marResult} distinct faces · difficulty {marDifficulty}</text>
					<rect x="88" y="92" width="64" height="28" rx="6" fill="none" stroke="var(--color-danger)" stroke-width="2" />
					<text x="120" y="111" text-anchor="middle" class="d-mar">MAR</text>
				</svg>
				<figcaption>The interference die cancels your matching 4, leaving a 1 and a 6 — two distinct faces, short of the difficulty. A Mar.</figcaption>
			</figure>
		{/if}

		{#if active === 'assets'}
			<h4>Assets</h4>
			<p>Everything your character controls is an asset. Four types:</p>
			<ul>
				<li><em>Holdings</em> — land and buildings.</li>
				<li><em>Peers</em> — the people of the court.</li>
				<li><em>Artifacts</em> — trinkets, relics, and other objects.</li>
				<li><em>Resources</em> — just about anything else.</li>
			</ul>
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
	}

	/* Tabs: horizontally scrollable on narrow screens, never wrapping. */
	.tabs {
		display: flex;
		gap: 0.4rem;
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
		padding: 0.4rem 0.9rem;
		font-family: var(--font-serif);
		font-size: 0.85rem;
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
		font-style: italic;
	}

	.body { color: var(--color-text); line-height: 1.55; }
	.body :global(h4) {
		font-family: var(--font-serif);
		font-size: 1.02rem;
		margin: 1rem 0 0.35rem;
		color: var(--color-text);
	}
	.body :global(h4:first-child) { margin-top: 0; }
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
	.diagram figcaption {
		margin-top: 0.6rem;
		font-size: 0.85rem;
		color: var(--color-text-muted);
		line-height: 1.45;
	}
	.d-num { font-family: var(--font-serif); font-size: 7px; fill: var(--color-text-muted); }
	.d-row { font-family: var(--font-serif); font-size: 9px; fill: var(--color-text); }
	.d-cap { font-family: var(--font-serif); font-size: 8px; fill: var(--color-accent); }
	.d-glabel { font-family: var(--font-serif); font-size: 8px; fill: var(--color-text-muted); text-transform: uppercase; letter-spacing: 0.06em; }
	.d-glabel-int { fill: var(--color-danger); }
	.d-calc { font-family: var(--font-serif); font-size: 10px; fill: var(--color-text-muted); }
	.d-mar { font-family: var(--font-serif); font-size: 14px; letter-spacing: 0.12em; fill: var(--color-danger); }

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
