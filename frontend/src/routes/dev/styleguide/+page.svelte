<!--
  /dev/styleguide — every token and shared piece of the visual language, in
  each state, drawn by the REAL components and recipes the app uses.

  Nothing here is a copy. The marks, chips, rows, notices and sheets are
  imported from lib/components; the buttons and status words use the shared
  stylesheets; every token value and contrast ratio is measured from the
  browser at render time (see `onMount`). So when a token in app.css is
  retuned, or a component changes, this page changes with it, and a piece that
  only looks right in a screenshot gets caught here.

  It ships to the live site (the route is part of the static build) but
  nothing in the app links to it, and it tells crawlers to stay away. It holds
  no game data and no rule text: the names and sentences are placeholders.

  Hover and focus can't be frozen in a still image, so they're described under
  each recipe; tab through the page, or hover, to see them for real.
  docs/STYLE_GUIDE.md is the written companion.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import '$lib/components/shared/marginaliaTile.css';
	import { onMount } from 'svelte';
	import type { Asset, Player } from '$lib/api';
	import AssetCardSelectable from '$lib/components/AssetCardSelectable.svelte';
	import CharCounter from '$lib/components/CharCounter.svelte';
	import CrownGlyph from '$lib/components/CrownGlyph.svelte';
	import LogMark from '$lib/components/LogMark.svelte';
	import RetinueSheet from '$lib/components/RetinueSheet.svelte';
	import BarbuteMark from '$lib/components/shared/BarbuteMark.svelte';
	import ChecklistRow from '$lib/components/shared/ChecklistRow.svelte';
	import Chip from '$lib/components/shared/Chip.svelte';
	import ClockMark from '$lib/components/shared/ClockMark.svelte';
	import DieMark from '$lib/components/shared/DieMark.svelte';
	import NewDot from '$lib/components/shared/NewDot.svelte';
	import Notice from '$lib/components/shared/Notice.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';
	import RowMark from '$lib/components/shared/RowMark.svelte';
	import StarMark from '$lib/components/shared/StarMark.svelte';
	import StepHeading from '$lib/components/shared/StepHeading.svelte';
	import DieTile from '$lib/components/plans/shared/DieTile.svelte';
	import FinishedStep from '$lib/components/plans/shared/FinishedStep.svelte';
	import ToneMark from '$lib/components/shared/ToneMark.svelte';
	import TableRoster from '$lib/components/shared/TableRoster.svelte';
	import { playerColor, PLAYER_SLOTS, UNKNOWN_PLAYER_COLOR } from '$lib/playerColor';
	import { PLAYER_SHAPES } from '$lib/playerShape';
	import Cell from './Cell.svelte';
	import { contrast, parseRGB, ratioLabel } from './color';
	import Section from './Section.svelte';
	import {
		BORDERS,
		GAPS,
		GOLDS,
		MEANING,
		RADII,
		SURFACES,
		TEXT_DISPLAY,
		TEXT_SHADES,
		TEXT_SIZES,
		TOUCH,
	} from './tokens';

	const SECTIONS: [string, string][] = [
		['type', 'Type'],
		['gaps', 'Gaps'],
		['corners', 'Corners'],
		['surfaces', 'Surfaces'],
		['text', 'Text'],
		['borders', 'Borders'],
		['gold', 'Gold'],
		['meaning', 'Meaning'],
		['players', 'Players'],
		['marks', 'Marks'],
		['buttons', 'Buttons'],
		['chips', 'Chips'],
		['rows', 'Rows'],
		['status', 'Status words'],
		['notices', 'Notices'],
		['sheets', 'Sheets'],
		['pieces', 'Game pieces'],
		['steps', 'Play steps'],
	];

	// ── Measured values ────────────────────────────────────────────────────
	// Filled on mount, from the browser's own resolved styles. Keys are
	// "<property>:<token>" for a single value and "<fg>|<bg>" for a contrast.
	let measured = $state<Record<string, string>>({});
	let ratios = $state<Record<string, string>>({});
	let playerRatios = $state<Record<string, string>>({});

	onMount(() => {
		const probe = document.createElement('div');
		probe.style.cssText = 'position:absolute;visibility:hidden;pointer-events:none;';
		document.body.appendChild(probe);

		const read = (prop: string, token: string): string => {
			probe.style.setProperty(prop, `var(${token})`);
			const value = getComputedStyle(probe).getPropertyValue(prop);
			probe.style.removeProperty(prop);
			return value;
		};
		const colour = (token: string) => parseRGB(read('color', token));

		const out: Record<string, string> = {};
		for (const t of [...TEXT_SIZES, TEXT_DISPLAY]) out[t.name] = read('font-size', t.name);
		for (const t of GAPS) out[t.name] = read('width', t.name);
		out[TOUCH.name] = read('width', TOUCH.name);
		for (const t of RADII) out[t.name] = read('border-top-left-radius', t.name);
		measured = out;

		const surfaces = SURFACES.map((s) => [s.name, colour(s.name)] as const);
		const r: Record<string, string> = {};
		for (const shade of TEXT_SHADES) {
			const fg = colour(shade.name);
			if (!fg) continue;
			for (const [bgName, bg] of surfaces) {
				if (bg) r[`${shade.name}|${bgName}`] = ratioLabel(contrast(fg, bg));
			}
		}
		ratios = r;

		const page = colour('--color-bg');
		const pr: Record<string, string> = {};
		if (page) {
			for (let seat = 1; seat <= PLAYER_SLOTS; seat++) {
				const fg = parseRGB(playerColor({ token_color: null, seat_order: seat }));
				if (fg) pr[String(seat)] = ratioLabel(contrast(fg, page));
			}
		}
		playerRatios = pr;

		probe.remove();
	});

	function radiusWord(value: string | undefined): string {
		if (!value) return '';
		if (value === '50%') return 'circle';
		return parseFloat(value) >= 500 ? 'fully round' : value;
	}

	// ── Placeholder players (no game data) ─────────────────────────────────
	const NAMES = ['Ash', 'Wren', 'otto', 'Mira', 'Tamsin'];
	const players: Player[] = NAMES.map((display_name, i) => ({
		id: i + 1,
		game_id: 1,
		account_id: i + 1,
		display_name,
		joined_at: '2026-01-01T00:00:00Z',
		is_facilitator: false,
		token_color: null,
		seat_order: i + 1,
	}));
	// Ash is "you"; the game is waiting on you and on Mira.
	const waiting = new Set([1, 4]);

	// Placeholder assets for the game-pieces section: made-up names, no game data.
	function sampleAsset(id: number, over: Partial<Asset>, notes: { text: string; torn?: boolean }[]): Asset {
		return {
			id, game_id: 0, owner_id: 0, creator_id: 0, asset_type: 'peer', name: 'Sample',
			is_main_character: false, is_leveraged: false, is_destroyed: false, created_at: '',
			destroyed_at: null, linked_card_suit: null, linked_card_value: null, secret_count: 0,
			marginalia: notes.map((n, i) => ({
				id: id * 10 + i, asset_id: id, position: i + 1, text: n.text, is_torn: !!n.torn,
				torn_at: null, torn_by_id: null, title: null,
			})),
			...over,
		};
	}
	const NOTES = [{ text: 'Keeps the harbour ledgers.' }, { text: 'Owes the crown a ship.' }, { text: 'Never seen to run.' }];
	const cardPlain = sampleAsset(1, { name: 'Lady Merrow' }, NOTES);
	const cardLeveraged = sampleAsset(2, { name: 'The Old Ledger', asset_type: 'artifact', is_leveraged: true }, NOTES);
	const cardMain = sampleAsset(3, { name: 'Captain Vell', is_main_character: true }, NOTES);
	const cardTorn = sampleAsset(
		4,
		{ name: 'The Salt Road', asset_type: 'resource' },
		[NOTES[0], { text: 'A third of it is owed already.', torn: true }, NOTES[2]],
	);
	const cardAtRisk = sampleAsset(5, { name: 'Wren’s Tower', asset_type: 'holding' }, [NOTES[0]]);
	const cardMarginalia = sampleAsset(6, { name: 'Lady Merrow' }, NOTES);
	let cardPicked = $state(false);
	let notePicked = $state<number | null>(null);

	// ── Live state for the interactive specimens ───────────────────────────
	let choice = $state<'aid' | 'interfere'>('aid');
	let subTab = $state<'events' | 'laws' | 'rumors'>('events');
	let sheetOpen = $state(false);
	let sheetChoice = $state<'one' | 'two' | 'out'>('one');
	let counterText = $state('Sixteen chars...');
</script>

<svelte:head>
	<title>Style guide · Uneasy Lies the Head</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="guide">
	<header class="masthead">
		<h1>Style guide</h1>
		<p class="lede">
			Every token and shared piece of the visual language, in each state. The pieces below are
			the real components the app uses, and every number is read from the browser, so this page
			cannot drift from the app. The written rules are in <code>docs/STYLE_GUIDE.md</code>.
		</p>
		<nav aria-label="Sections">
			<ul class="jump">
				{#each SECTIONS as [id, label] (id)}
					<li><a href="#{id}">{label}</a></li>
				{/each}
			</ul>
		</nav>
	</header>

	<!-- ── Tokens ──────────────────────────────────────────────────────── -->

	<Section
		id="type"
		title="Type"
		blurb="Five sizes, each with a job, in rem so the browser's text-size setting still applies. 12px is the smallest anything gets."
	>
		<ul class="token-list">
			{#each TEXT_SIZES as t (t.name)}
				<li class="token">
					<div class="token-head">
						<code>{t.name}</code>
						<span class="value">{measured[t.name] ?? ''}</span>
					</div>
					<span class="sample" style:font-size="var({t.name})">Uneasy lies the head</span>
					<span class="job">{t.job}</span>
				</li>
			{/each}
			<li class="token">
				<div class="token-head">
					<code>{TEXT_DISPLAY.name}</code>
					<span class="value">{measured[TEXT_DISPLAY.name] ?? ''}</span>
				</div>
				<span class="sample display" style:font-size="var({TEXT_DISPLAY.name})">Uneasy</span>
				<span class="job">{TEXT_DISPLAY.job}</span>
			</li>
		</ul>
	</Section>

	<Section
		id="gaps"
		title="Gaps"
		blurb="One set for padding, margin and gap. The steps are far enough apart to tell at a glance, so spacing alone shows what belongs together, which is what lets most boxes go. The last block is the touch floor, which is a size rather than a gap."
	>
		<ul class="token-list">
			{#each GAPS as t (t.name)}
				<li class="token gap">
					<div class="token-head">
						<code>{t.name}</code>
						<span class="value">{measured[t.name] ?? ''}</span>
					</div>
					<span class="bar" style:width="var({t.name})"></span>
				</li>
			{/each}
			<li class="token">
				<div class="token-head">
					<code>{TOUCH.name}</code>
					<span class="value">{measured[TOUCH.name] ?? ''}</span>
				</div>
				<span class="tap-sample" style:width="var({TOUCH.name})" style:height="var({TOUCH.name})"></span>
				<span class="job">{TOUCH.job}</span>
			</li>
		</ul>
	</Section>

	<Section
		id="corners"
		title="Corners"
		blurb="The corner is the first hint at what kind of object something is: near-square for a card, soft for a button, a full pill for a chip."
	>
		<ul class="token-list tiles">
			{#each RADII as t (t.name)}
				<li class="token">
					<div class="token-head">
						<code>{t.name}</code>
						<span class="value">{radiusWord(measured[t.name])}</span>
					</div>
					{#if t.name === '--radius-sheet'}
						<span
							class="corner-sample"
							style:border-radius="var({t.name}) var({t.name}) 0 0"
						></span>
					{:else}
						<span class="corner-sample" style:border-radius="var({t.name})"></span>
					{/if}
					<span class="job">{t.job}</span>
				</li>
			{/each}
		</ul>
	</Section>

	<Section
		id="surfaces"
		title="Surfaces"
		blurb="Three levels, one for each depth a box can reach. Boxes nest at most two deep. A fill says something about a control's state, never about how important a region is."
	>
		<ul class="token-list tiles">
			{#each SURFACES as t (t.name)}
				<li class="token">
					<div class="token-head"><code>{t.name}</code></div>
					<span class="swatch" style:background="var({t.name})"></span>
					<span class="job">{t.job}</span>
				</li>
			{/each}
		</ul>
	</Section>

	<Section
		id="text"
		title="Text"
		blurb="Three parchment shades. Faint is the dimmest any text gets, and it clears 4.5:1 on all three surfaces. Contrast is measured here, live."
	>
		<ul class="token-list">
			{#each TEXT_SHADES as t (t.name)}
				<li class="token">
					<div class="token-head"><code>{t.name}</code></div>
					<span class="sample" style:color="var({t.name})">Uneasy lies the head</span>
					<span class="job">{t.job}</span>
					<span class="ratios">
						{#each SURFACES as s (s.name)}
							<span class="ratio">
								<span class="ratio-dot" style:background="var({s.name})"></span>
								{ratios[`${t.name}|${s.name}`] ?? ''}
							</span>
						{/each}
					</span>
				</li>
			{/each}
		</ul>
		<p class="note">The dots are page, raised and raised twice, in that order.</p>
	</Section>

	<Section
		id="borders"
		title="Borders"
		blurb="Warm borders go on game pieces and neutral ones on controls, each drawn to be seen on a phone. Keep a border for things a player can pick up or press; group everything else with space."
	>
		<ul class="token-list tiles">
			{#each BORDERS as t (t.name)}
				<li class="token">
					<div class="token-head"><code>{t.name}</code></div>
					{#if t.name === '--color-border-piece' || t.name.startsWith('--color-border-control')}
						<span
							class="box-sample"
							class:piece={t.name === '--color-border-piece'}
							style:border="1px solid var({t.name})"
						></span>
					{:else}
						<span class="rule-sample" style:border-top="1px solid var({t.name})"></span>
					{/if}
					<span class="job">{t.job}</span>
				</li>
			{/each}
		</ul>
	</Section>

	<Section
		id="gold"
		title="Gold"
		blurb="Two tiers, two meanings. Deep gold is the game. Bright gold is you, and is kept off headings and off fills."
	>
		<ul class="token-list tiles">
			{#each GOLDS as t (t.name)}
				<li class="token">
					<div class="token-head"><code>{t.name}</code></div>
					<span class="swatch" style:background="var({t.name})"></span>
					<span class="job">{t.job}</span>
				</li>
			{/each}
		</ul>
		<div class="specimens">
			<Cell label="deep gold as a heading">
				<span class="heading-sample">A heading</span>
			</Cell>
			<Cell label="bright gold: what you picked">
				<Chip selected>Picked</Chip>
			</Cell>
			<Cell label="bright gold: the waiting clock on you">
				<ClockMark tone="you" size={24} label="waiting on you" />
			</Cell>
		</div>
	</Section>

	<Section
		id="meaning"
		title="Meaning"
		blurb="Each meaning colour has one meaning and always travels with a word, so colour is never the only signal. Options a player is choosing between wear none of them."
	>
		<ul class="token-list tiles">
			{#each MEANING as t (t.name)}
				<li class="token">
					<div class="token-head"><code>{t.name}</code></div>
					<span class="meaning-sample" style:color="var({t.name})">
						<span class="swatch small" style:background="var({t.name})"></span>
						{t.word}
					</span>
					<span class="job">{t.job}</span>
				</li>
			{/each}
		</ul>
	</Section>

	<Section
		id="players"
		title="Players"
		blurb="Seats are filled in join order and the table's creator is seat 1, so most tables use the first colours. A colour is always paired with a shape wherever it stands for a player and no name is near. Selection is never shown in a player colour."
	>
		<ul class="token-list tiles">
			{#each PLAYER_SHAPES as shape, i (shape.name)}
				<li class="token">
					<div class="token-head">
						<span>Seat {i + 1}</span>
						<span class="value">{playerRatios[String(i + 1)] ?? ''} on the page</span>
					</div>
					<span class="player-sample" style:color={playerColor({ token_color: null, seat_order: i + 1 })}>
						<PlayerShape seat={i + 1} size={24} />
						<span>{shape.colorName}</span>
					</span>
					<span class="job">
						{shape.name} · <code>{playerColor({ token_color: null, seat_order: i + 1 })}</code>
					</span>
				</li>
			{/each}
			<li class="token">
				<div class="token-head"><span>No player found</span></div>
				<span class="player-sample" style:color={UNKNOWN_PLAYER_COLOR}>
					<span class="unknown-dot"></span>
					<span>Unknown</span>
				</span>
				<span class="job">a fallback for a player that can't be resolved. Never "a quieter player"</span>
			</li>
		</ul>
		<Cell label="Shapes at 12, 16 and 24px (the smallest is 12)">
			{#each PLAYER_SHAPES as _shape, i (i)}
				<span class="size-run" style:color={playerColor({ token_color: null, seat_order: i + 1 })}>
					<PlayerShape seat={i + 1} size={12} />
					<PlayerShape seat={i + 1} size={16} />
					<PlayerShape seat={i + 1} size={24} />
				</span>
			{/each}
		</Cell>
		<Cell label="The player row: a shape beside every name, a clock on who the game waits on">
			<div class="chip-row">
				{#each players as p (p.id)}
					<Chip compact aria-label={p.display_name}>
						<PlayerShape player={p} />
						<span style:color={playerColor(p)}>{p.id === 1 ? 'You' : p.display_name}</span>
						{#if waiting.has(p.id)}
							<ClockMark tone={p.id === 1 ? 'you' : 'other'} />
						{/if}
					</Chip>
				{/each}
			</div>
		</Cell>
	</Section>

	<!-- ── Marks ───────────────────────────────────────────────────────── -->

	<Section
		id="marks"
		title="Marks"
		blurb="The house marks. Each is one component, drawn once, and every screen borrows it. A mark must render from something the app ships, never a typed character."
	>
		<div class="specimens compact">
			<Cell label="PlayerShape · five seats">
				<span class="size-run">
					{#each PLAYER_SHAPES as _s, i (i)}
						<PlayerShape seat={i + 1} size={24} label="seat {i + 1}" />
					{/each}
				</span>
			</Cell>
			<Cell label="ClockMark · you, other, inherited">
				<ClockMark tone="you" size={24} label="waiting on you" />
				<ClockMark tone="other" size={24} label="waiting on someone else" />
				<ClockMark size={24} label="inherits the text colour" />
				<ClockMark tone="you" />
			</Cell>
			<Cell label="BarbuteMark · war, 16 / 24 / 32">
				<BarbuteMark label="war" />
				<BarbuteMark size={24} label="war" />
				<BarbuteMark size={32} label="war" />
			</Cell>
			<Cell label="DieMark · 16 / 20 / 32">
				<DieMark size={16} label="a die" />
				<DieMark label="a die" />
				<DieMark size={32} label="a die" />
			</Cell>
			<Cell label="StarMark · the main character (solid), and the control that makes one (outline)">
				<StarMark size={24} label="main character" />
				<StarMark size={24} outline label="make main character" />
				<StarMark />
			</Cell>
			<Cell label="ToneMark · warning, danger, success, info (a notice's marks; colour comes from the caller)">
				<span class="tone-sample warning"><ToneMark tone="warning" size={24} label="warning" /></span>
				<span class="tone-sample danger"><ToneMark tone="danger" size={24} label="danger" /></span>
				<span class="tone-sample success"><ToneMark tone="success" size={24} label="success" /></span>
				<span class="tone-sample info"><ToneMark tone="info" size={24} label="info" /></span>
			</Cell>
			<Cell label="ToneMark · the success tick on a picked card, in bright gold">
				<span class="tone-sample picked"><ToneMark tone="success" size={24} label="picked" /></span>
				<ToneMark tone="success" />
			</Cell>
			<Cell label="NewDot · with the tab it sits on">
				<span class="with-dot">Chat <NewDot /></span>
				<NewDot />
			</Cell>
			<Cell label="RowMark · caret (shut, open) expands; arrow opens elsewhere">
				<RowMark kind="caret" />
				<RowMark kind="caret" open />
				<RowMark kind="arrow" />
			</Cell>
			<Cell label="CrownGlyph · monarch, successors">
				<CrownGlyph mark={{ role: 'monarch' }} size={24} />
				<CrownGlyph mark={{ role: 'successor', ordinal: 1 }} size={24} />
				<CrownGlyph mark={{ role: 'successor', ordinal: 2 }} size={24} />
			</Cell>
			<Cell label="LogMark · the older log families">
				<span class="log-run">
					{#each ['plan', 'asset', 'roll', 'law', 'scene', 'ranking', 'clock', 'war'] as family (family)}
						<span class="log-mark-box"><LogMark {family} /></span>
					{/each}
				</span>
			</Cell>
		</div>
		<p class="note">
			The new clock and barbute are LogMark families too (the last two above), so the app keeps
			one place where marks are drawn. <code>PlayerShape</code> and <code>NewDot</code> are
			filled shapes at small sizes, so they have their own components.
		</p>
	</Section>

	<!-- ── Recipes ─────────────────────────────────────────────────────── -->

	<Section
		id="buttons"
		title="Buttons"
		blurb="One main button per step, in deep gold. Everything else is secondary. A button that can't be pressed yet is greyed out, which reads as unavailable without spending the main button's gold. Always at least 44px tall."
	>
		<div class="specimens compact">
			<Cell label="main · .action-btn.primary">
				<button class="action-btn primary">Commit</button>
			</Cell>
			<Cell label="secondary · .action-btn.secondary">
				<button class="action-btn secondary">Pass</button>
			</Cell>
			<Cell label="greyed out · primary, disabled">
				<button class="action-btn primary" disabled>Send</button>
			</Cell>
			<Cell label="greyed out · secondary, disabled">
				<button class="action-btn secondary" disabled>Pass</button>
			</Cell>
			<Cell label="greyed out · aria-disabled (still tappable, to explain why)">
				<button class="action-btn primary" aria-disabled="true">Join</button>
			</Cell>
			<Cell label="a main and a secondary, as in a step">
				<div class="button-pair">
					<button class="action-btn primary">Commit</button>
					<button class="action-btn secondary">Pass</button>
				</div>
			</Cell>
			<Cell label="small · legacy, for the few dense spots that have it">
				<button class="action-btn secondary small">Small</button>
			</Cell>
		</div>
		<p class="note">
			Hover lifts the main button's gold and lightens the secondary's border. Focus draws a 2px
			bright gold ring, offset from the edge. Tab to any button to see it.
		</p>
	</Section>

	<Section
		id="chips"
		title="Chips"
		blurb="A chip is a fully rounded control for choosing among a few things. Selected is a 2px bright gold border and bright gold text, with no fill. Choosing one never moves it or its neighbours."
	>
		<div class="specimens">
			<Cell label="choice · selected, resting (tap to switch)">
				<Chip selected={choice === 'aid'} onclick={() => (choice = 'aid')}>Aid</Chip>
				<Chip selected={choice === 'interfere'} onclick={() => (choice = 'interfere')}>
					Interfere
				</Chip>
			</Cell>
			<Cell label="compact tab chips · with the new dot">
				<Chip compact kind="tab" selected={subTab === 'events'} onclick={() => (subTab = 'events')}>
					Events
				</Chip>
				<Chip compact kind="tab" selected={subTab === 'laws'} onclick={() => (subTab = 'laws')}>
					Laws <NewDot />
				</Chip>
				<Chip compact kind="tab" selected={subTab === 'rumors'} onclick={() => (subTab = 'rumors')}>
					Rumors
				</Chip>
			</Cell>
			<Cell label="disabled">
				<Chip disabled>Unavailable</Chip>
				<Chip selected disabled>Chosen, locked</Chip>
			</Cell>
		</div>
		<p class="note">
			A choice wears no meaning colour: the two chips above are told apart by their words, and
			the one picked is the one in bright gold. Hover lightens the border; focus draws the
			bright gold ring.
		</p>
	</Section>

	<Section
		id="rows"
		title="Rows"
		blurb="A tappable row is a plain line with a hairline beneath it. Its caret or arrow is the only sign that it can be pressed; there is no box and no fill. A caret expands in place, an arrow opens something elsewhere, and neither means there is nothing to do."
	>
		<div class="specimens wide">
			<Cell label="ChecklistRow · expands (shut, open), opens elsewhere, nothing to do">
				<div class="list">
					<ChecklistRow
						id="sg-row-shut"
						title="A row that expands"
						subtitle="One line under the title"
						glyph="help"
						action="expand"
						state={{ text: '4 set' }}
					>
						<p class="muted-text">The body appears here.</p>
					</ChecklistRow>
					<ChecklistRow id="sg-row-open" title="Already open" action="expand" defaultOpen>
						<p class="muted-text">The body is the caller's own markup.</p>
					</ChecklistRow>
					<ChecklistRow
						title="A row that opens a sheet"
						glyph="circle"
						action="navigate"
						onSelect={() => (sheetOpen = true)}
					/>
					<ChecklistRow title="Finished, nothing to do" glyph="tick" tone="done" state={{ text: 'done', tone: 'on' }} />
				</div>
			</Cell>
			<Cell label="state words and tones">
				<div class="list">
					<ChecklistRow title="Neutral state" state={{ text: '2 of 3' }} />
					<ChecklistRow title="A switch worth turning on" state={{ text: 'off', tone: 'off' }} action="navigate" />
					<ChecklistRow title="A deadline" tone="warn" state={{ text: 'last chance', tone: 'warn' }} action="navigate" />
					<ChecklistRow title="Something at risk" tone="risk" state={{ text: 'at risk', tone: 'risk' }} action="navigate" />
					<ChecklistRow title="The primary item" tone="primary" action="navigate" />
				</div>
			</Cell>
			<Cell label="TableRoster · inert (clock on you and on Mira)">
				<div class="list">
					<TableRoster {players} currentPlayerID={1} waitingPlayerIDs={waiting} />
				</div>
			</Cell>
			<Cell label="TableRoster · tappable, with trailing content">
				<div class="list">
					<TableRoster
						players={players.slice(0, 3)}
						currentPlayerID={1}
						waitingPlayerIDs={waiting}
						onSelect={() => {}}
					>
						{#snippet trailing()}<span class="muted-text small">3 peers</span>{/snippet}
					</TableRoster>
				</div>
			</Cell>
		</div>
		<p class="note">
			A row is at least 44px tall. Hover lifts it to the raised surface; focus draws the bright
			gold ring inside it so a list's edges never clip it.
		</p>
	</Section>

	<Section
		id="status"
		title="Status words"
		blurb="Where a thing stands, as small capitals in a colour. They are never chips: a border or fill would make them look pressable. Plain secondary is right for a state; take a meaning colour only for a result, and always beside the word."
	>
		<ul class="status-list">
			<li><span class="status-word">Resolving</span><code>.status-word</code></li>
			<li><span class="status-word">Set aside</span><code>.status-word</code></li>
			<li><span class="status-word success">Make</span><code>.success</code></li>
			<li><span class="status-word danger">Mar</span><code>.danger</code></li>
			<li><span class="status-word warning">Off</span><code>.warning</code></li>
		</ul>
	</Section>

	<Section
		id="notices"
		title="Notices"
		blurb="A notice is a mark and coloured text, like a log line: it sits inside a step, so it is a group, not a thing to touch, and gets no box and no tint. The mark is centred on the first line however far the words wrap."
	>
		<div class="specimens wide">
			<Cell label="warning"><Notice tone="warning">Sample notice about a cost or a risk.</Notice></Cell>
			<Cell label="danger"><Notice tone="danger">Sample notice that something is lost.</Notice></Cell>
			<Cell label="success"><Notice tone="success">Sample notice that it worked.</Notice></Cell>
			<Cell label="info"><Notice tone="info">Sample notice with information or a gain.</Notice></Cell>
			<Cell label="wrapping">
				<Notice tone="warning">
					A longer sample notice that wraps onto a second and a third line, to show the mark
					staying beside the first line and the words hanging cleanly.
				</Notice>
			</Cell>
			<Cell label="with a house mark of the noun instead of the default">
				<Notice tone="danger">
					{#snippet mark()}<LogMark family="tear" />{/snippet}
					Sample notice wearing a different house mark.
				</Notice>
			</Cell>
		</div>
		<Cell label="CharCounter · appears within five of the limit (type in the box)">
			<div class="counter">
				<input
					aria-label="Sample field"
					maxlength="20"
					bind:value={counterText}
				/>
				<CharCounter value={counterText} max={20} />
			</div>
		</Cell>
	</Section>

	<Section
		id="sheets"
		title="Sheets"
		blurb="A sheet is the one layer that floats above the panes: a raised surface rounded on its top corners, with a scrim behind it and a soft shadow, which only sheets get. It has a title in deep gold and a 44px close button. Sheets never nest."
	>
		<Cell label="RetinueSheet · tap to open; close with the button, the scrim, Escape or Back">
			<button class="action-btn secondary" onclick={() => (sheetOpen = true)}>Open the example sheet</button>
		</Cell>
	</Section>

	<Section
		id="pieces"
		title="Game pieces"
		blurb="Asset cards and marginalia tiles are the pieces a player touches and reads. Each takes the warm border, the piece corner and the raised surface. Whose a card is shows as the owner's shape beside its name; its status is a word, and orange is leverage. Selected is the bright gold border with no fill, plus a tick."
	>
		<Cell label="Marginalia tile · filled, torn, titled, empty slot">
			<div class="tile-run">
				<div class="m-tile">Keeps the harbour ledgers.</div>
				<div class="m-tile torn">Owes the crown a ship.</div>
				<div class="m-tile titled">The Monarch <CrownGlyph mark={{ role: 'monarch' }} size={14} /></div>
				<div class="m-tile empty"></div>
			</div>
		</Cell>
		<div class="specimens wide">
			<Cell label="Asset card · not picked (tap the box to pick it)">
				<div class="piece-col">
					<AssetCardSelectable
						asset={cardPlain}
						ownerColor={playerColor({ token_color: null, seat_order: 1 })}
						selectable
						selected={cardPicked}
						onToggle={() => (cardPicked = !cardPicked)}
						defaultExpanded
					/>
				</div>
			</Cell>
			<Cell label="Asset card · selected: bright gold border and a tick, no fill">
				<div class="piece-col">
					<AssetCardSelectable
						asset={cardPlain}
						ownerColor={playerColor({ token_color: null, seat_order: 2 })}
						selectable
						selected
						onToggle={() => {}}
					/>
				</div>
			</Cell>
			<Cell label="Asset card · leveraged: orange, beside its word">
				<div class="piece-col">
					<AssetCardSelectable
						asset={cardLeveraged}
						ownerColor={playerColor({ token_color: null, seat_order: 3 })}
						ownerLabel="Owned by Rose"
					/>
				</div>
			</Cell>
			<Cell label="Asset card · can't be picked: greyed out, not faded">
				<div class="piece-col">
					<AssetCardSelectable
						asset={cardMain}
						ownerColor={playerColor({ token_color: null, seat_order: 4 })}
						selectable
						disabled
						onToggle={() => {}}
					/>
				</div>
			</Cell>
			<Cell label="Asset card · a torn note, struck through with a red tear">
				<div class="piece-col">
					<AssetCardSelectable
						asset={cardTorn}
						ownerColor={playerColor({ token_color: null, seat_order: 5 })}
						defaultExpanded
					/>
				</div>
			</Cell>
			<Cell label="Asset card · one note left and a slot free: the count and caret are red">
				<div class="piece-col">
					<AssetCardSelectable
						asset={cardAtRisk}
						ownerColor={playerColor({ token_color: null, seat_order: 1 })}
						defaultExpanded
					/>
				</div>
			</Cell>
			<Cell label="Asset card · picking a note: the card holding it takes the gold border">
				<div class="piece-col">
					<AssetCardSelectable
						asset={cardMarginalia}
						ownerColor={playerColor({ token_color: null, seat_order: 2 })}
						marginaliaSelectable
						selectedMarginaliaID={notePicked}
						onMarginaliaToggle={(id) => (notePicked = notePicked === id ? null : id)}
					/>
				</div>
			</Cell>
		</div>
	</Section>

	<Section
		id="steps"
		title="Play steps"
		blurb="The Play pane reads as a plan's steps in order. Finished steps are one line each, and only the open step is expanded, under a heading that states the current ask. The roll is one of the steps. There are no boxes: a step is a heading and space, and the only things drawn as objects are what you press."
	>
		<Cell label="Step heading · your move, then someone else's (the ask is stated in words here and only here)">
			<div class="step-col">
				<StepHeading heading={{ text: 'Your move — aid or interfere', yours: true }} />
				<StepHeading heading={{ text: 'Waiting on otto — the roll', yours: false }} />
			</div>
		</Cell>
		<Cell label="Finished steps · plain lines with the step's mark; one with more to say opens in place (caret)">
			<div class="step-col">
				<FinishedStep>
					{#snippet mark()}<LogMark family="plan" />{/snippet}
					otto prepared Seek Answers
				</FinishedStep>
				<FinishedStep>
					{#snippet mark()}<LogMark family="marginalia" />{/snippet}
					otto: “Who really holds the western holdfast, and for how long?”
					{#snippet detail()}“Who really holds the western holdfast, and for how long?”{/snippet}
				</FinishedStep>
				<FinishedStep>
					{#snippet mark()}<DieMark size={16} />{/snippet}
					Rolled <span class="status-word success">Make</span> · 2 distinct faces, difficulty 2
					{#snippet detail()}
						<div class="tile-run">
							<DieTile face={3} color={playerColor({ token_color: null, seat_order: 3 })} label="otto's die" />
							<DieTile face={5} color={playerColor({ token_color: null, seat_order: 3 })} label="otto's die" />
						</div>
					{/snippet}
				</FinishedStep>
			</div>
		</Cell>
		<Cell label="Dice · unrolled (the house die), rolled, cancelled and a repeat; the edge is the owner's colour, and dice carry no shape yet">
			<div class="tile-run">
				<DieTile face={null} color={playerColor({ token_color: null, seat_order: 1 })} label="Lavender's die" />
				<DieTile face={4} color={playerColor({ token_color: null, seat_order: 2 })} label="Peridot's die" />
				<DieTile face={2} color={playerColor({ token_color: null, seat_order: 3 })} label="Rose's die" cancelled />
				<DieTile face={4} color={playerColor({ token_color: null, seat_order: 4 })} label="Sapphire's die, repeats a face" duplicate />
			</div>
		</Cell>
		<Cell label="A choice in a step · a roll's choices (aid or interfere, easier or harder) wear no meaning colour; the one picked is bright gold, and the one main button names what it commits">
			<div class="step-col">
				<div class="chip-row">
					<Chip selected>Easier (−1)</Chip>
					<Chip>Harder (+1)</Chip>
				</div>
				<Notice tone="warning">Lady Merrow will be leveraged.</Notice>
				<div class="chip-row">
					<button class="action-btn primary">Commit aid</button>
					<button class="action-btn secondary">Pass</button>
				</div>
			</div>
		</Cell>
	</Section>
</div>

<RetinueSheet open={sheetOpen} onClose={() => (sheetOpen = false)}>
	<h3 class="sheet-title">Example sheet</h3>
	<div class="sheet-demo">
		<p class="muted-text">
			A sheet holds the same pieces as the page: choices as chips, a cost in plain text, and one
			main button.
		</p>
		<div class="chip-row">
			<Chip selected={sheetChoice === 'one'} onclick={() => (sheetChoice = 'one')}>Option one</Chip>
			<Chip selected={sheetChoice === 'two'} onclick={() => (sheetChoice = 'two')}>Option two</Chip>
			<Chip selected={sheetChoice === 'out'} onclick={() => (sheetChoice = 'out')}>Neither</Chip>
		</div>
		<Notice tone="warning">Sample notice inside a sheet.</Notice>
		<button class="action-btn primary" onclick={() => (sheetOpen = false)}>Confirm</button>
	</div>
</RetinueSheet>

<style>
	.guide {
		max-width: 960px;
		margin: 0 auto;
		padding: var(--space-4) var(--space-4) var(--space-6);
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
		color: var(--color-text);
	}

	.masthead {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	h1 {
		margin: 0;
		font-size: var(--text-title);
		color: var(--color-accent);
		letter-spacing: 0.04em;
		text-transform: uppercase;
	}
	.lede {
		margin: 0;
		font-size: var(--text-body);
		line-height: 1.5;
		max-width: 60ch;
		color: var(--color-text-secondary);
	}
	code {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		color: var(--color-text-secondary);
	}

	/* A single scrolling line of anchors: sixteen wrapped links would take up
	   a whole phone screen. */
	.jump {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		gap: var(--space-4);
		overflow-x: auto;
		scrollbar-width: none;
	}
	.jump a {
		display: inline-flex;
		align-items: center;
		min-height: var(--size-tap);
		white-space: nowrap;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
		text-decoration: none;
	}
	.jump a:hover { color: var(--color-text); }
	.jump a:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}

	/* ── Token lists ─────────────────────────────────────────────────────── */
	.token-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 16rem), 1fr));
		gap: var(--space-4);
	}
	.token-list.tiles {
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 9.5rem), 1fr));
	}
	.token {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		min-width: 0;
	}
	.token-head {
		display: flex;
		flex-wrap: wrap;
		justify-content: space-between;
		align-items: baseline;
		gap: var(--space-2);
		font-size: var(--text-small);
	}
	.value {
		font-size: var(--text-label);
		color: var(--color-text-faint);
	}
	.job {
		font-size: var(--text-label);
		line-height: 1.4;
		color: var(--color-text-faint);
	}
	.sample {
		line-height: 1.2;
		overflow-wrap: anywhere;
	}
	.sample.display { font-family: var(--font-display); }

	.gap .bar {
		display: block;
		height: 16px;
		background: var(--color-accent);
		border-radius: var(--radius-piece);
	}

	.tap-sample {
		display: block;
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
		background: var(--color-surface);
	}
	.corner-sample {
		display: block;
		width: 100%;
		height: 56px;
		background: var(--color-surface);
		border: 1px solid var(--color-border-control);
	}
	.swatch {
		display: block;
		height: 56px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-piece);
	}
	.swatch.small {
		display: inline-block;
		width: 12px;
		height: 12px;
		border: 0;
		vertical-align: middle;
	}
	.box-sample {
		display: block;
		height: 56px;
		background: var(--color-surface);
	}
	.box-sample.piece { border-radius: var(--radius-piece); }
	.box-sample:not(.piece) { border-radius: var(--radius-control); }
	.rule-sample {
		display: block;
		height: 0;
		margin: var(--space-4) 0;
	}

	.ratios {
		display: flex;
		gap: var(--space-3);
		font-size: var(--text-label);
		color: var(--color-text-faint);
	}
	.ratio {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
	}
	.ratio-dot {
		display: inline-block;
		width: 12px;
		height: 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-piece);
	}
	.note {
		margin: 0;
		font-size: var(--text-label);
		line-height: 1.5;
		color: var(--color-text-faint);
		max-width: 60ch;
	}

	.heading-sample {
		font-size: var(--text-heading);
		color: var(--color-accent);
	}
	.meaning-sample {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-body);
	}
	.player-sample {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-body);
	}
	.unknown-dot {
		display: inline-block;
		width: 12px;
		height: 12px;
		border-radius: var(--radius-dot);
		background: currentColor;
	}
	.size-run {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
	}

	/* ── Specimens ───────────────────────────────────────────────────────── */
	.specimens {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 14rem), 1fr));
		gap: var(--space-5) var(--space-4);
	}
	.specimens.compact {
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 10rem), 1fr));
	}
	.specimens.wide {
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 20rem), 1fr));
	}
	.status-list {
		list-style: none;
		margin: 0;
		padding: 0;
		max-width: 28rem;
		border-top: 1px solid var(--color-border);
	}
	.status-list li {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		min-height: var(--size-tap);
		border-bottom: 1px solid var(--color-border);
	}
	.chip-row {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.button-pair {
		display: flex;
		gap: var(--space-2);
	}
	.with-dot {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
	.log-run {
		display: inline-flex;
		flex-wrap: wrap;
		gap: var(--space-3);
		color: var(--color-text-secondary);
	}
	.log-mark-box {
		display: block;
		width: 16px;
		height: 16px;
	}
	.list {
		width: 100%;
		min-width: 0;
		border-top: 1px solid var(--color-border);
	}
	.counter {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		max-width: 20rem;
	}
	.sheet-demo {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		align-items: flex-start;
	}
	.tone-sample { display: inline-flex; }
	.tone-sample.warning { color: var(--color-warning); }
	.tone-sample.danger { color: var(--color-danger); }
	.tone-sample.success { color: var(--color-success); }
	.tone-sample.info { color: var(--color-highlight); }
	.tone-sample.picked { color: var(--color-you); }
	.tile-run {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 9rem), 1fr));
		gap: var(--space-2);
		width: 100%;
		max-width: 28rem;
	}
	.tile-run .m-tile { min-height: var(--size-tap); }
	/* A run of steps or a column of heading and choices: the width of a play column. */
	.step-col {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		width: min(100%, var(--layout-column-min));
	}
	.piece-col {
		width: 100%;
		min-width: 0;
	}
</style>
