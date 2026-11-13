import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { render } from 'svelte/server';
import LobbyView from './phases/LobbyView.svelte';
import ShakeUpView from './phases/ShakeUpView.svelte';
import EndedView from './phases/EndedView.svelte';
import EndgameVotePanel from './EndgameVotePanel.svelte';
import { playerColor } from '$lib/playerColor';
import { declarations, styleOf } from './shared/cssTestKit';
import type { Game, Player, Ranking } from '$lib/api';

/*
 * The lobby, the Shake-Up, the ending vote and the ended screen. What a player
 * sees is checked by what the components emit (rendered through svelte/server,
 * as the other component tests are: no DOM, so nothing that waits on onMount's
 * fetch is reachable here, and the specs in tests/e2e/ read the live screens).
 * The rules that live only in a stylesheet are checked in the stylesheet, read
 * with comments stripped so prose that names a property can't fail or hide
 * anything.
 */

const read = (rel: string) => readFileSync(join(__dirname, rel), 'utf8');
const html = (component: unknown, props: Record<string, unknown> = {}) =>
	render(component as never, { props } as never).body;
/** Rendered text, with Svelte's hydration comments and the whitespace folded away. */
const text = (body: string) =>
	body.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();

// ── fixtures ────────────────────────────────────────────────────────────────

function player(id: number, name: string, extra: Partial<Player> = {}): Player {
	return {
		id,
		game_id: 1,
		account_id: id,
		display_name: name,
		joined_at: '2026-11-12T00:00:00Z',
		is_facilitator: false,
		token_color: null,
		seat_order: id,
		...extra,
	};
}
/** Server rendering still runs onDestroy, which these two components use to drop
 *  their window listeners; the suite has no window, so give it an empty one. */
function withWindowStub() {
	beforeAll(() => {
		vi.stubGlobal('window', { addEventListener: () => {}, removeEventListener: () => {} });
	});
	afterAll(() => {
		vi.unstubAllGlobals();
	});
}

const alice = player(1, 'alice', { is_facilitator: true });
const bob = player(2, 'bob');
const carol = player(3, 'carol');

function game(over: Partial<Game> = {}): Game {
	return {
		id: 1, join_code: 'ABC123', created_at: '', facilitator_id: 1, phase: 'lobby', current_row: 0,
		focus_player_id: null, ending_mode: null, ending_vote_open: false, dummy_token_mode: 'static',
		prologue_ranking_step: null, shake_up_category: null, shake_up_step: null, throne_established: false,
		...over,
	};
}

// ── the ended screen ────────────────────────────────────────────────────────

describe('EndedView', () => {
	const rankings = [
		// Power: alice 2, a dummy holds 1, carol 3. The dummy is nobody and is not listed.
		{ id: 1, game_id: 1, player_id: null, category: 'power', rank: 1 },
		{ id: 2, game_id: 1, player_id: 1, category: 'power', rank: 2 },
		{ id: 3, game_id: 1, player_id: 3, category: 'power', rank: 3 },
		// Written out of order on purpose: the list is sorted by rank, not by arrival.
		{ id: 4, game_id: 1, player_id: 2, category: 'knowledge', rank: 3 },
		{ id: 5, game_id: 1, player_id: 1, category: 'knowledge', rank: 2 },
		{ id: 6, game_id: 1, player_id: 2, category: 'esteem', rank: 2 },
	] as Ranking[];
	const body = html(EndedView, { rankings, players: [alice, bob, carol] });

	it('lists the three tracks in the order Power, Knowledge, Esteem', () => {
		const heads = [...body.matchAll(/<h4[^>]*>([^<]*)<\/h4>/g)].map((m) => m[1]);
		expect(heads).toEqual(['power', 'knowledge', 'esteem']);
	});

	it('lists each track best rank first, and leaves a dummy slot out', () => {
		const tracks = body.split('<div class="track').slice(1);
		const names = tracks.map((t) => [...t.matchAll(/class="name[^"]*"[^>]*>([^<]*)</g)].map((m) => m[1]));
		expect(names).toEqual([['alice', 'carol'], ['alice', 'bob'], ['bob']]);
	});

	it('draws every standing as a shape and a name in the player\'s colour', () => {
		// One shape per listed player: 2 + 2 + 1.
		expect(body.match(/class="player-shape[^"]*"/g)).toHaveLength(5);
		for (const p of [alice, bob, carol]) {
			expect(body.toLowerCase()).toContain(playerColor(p).toLowerCase());
		}
	});

	it('shows the closing words with no standings when no rankings exist', () => {
		const empty = html(EndedView, { rankings: [], players: [alice] });
		expect(text(empty)).toContain('Game Over');
		expect(text(empty)).toContain('The public record is sealed.');
		expect(empty).not.toContain('Final Rankings');
	});
});

// ── the ending vote ─────────────────────────────────────────────────────────

describe('EndgameVotePanel', () => {
	withWindowStub();
	let body = '';
	beforeAll(() => {
		body = html(EndgameVotePanel, {
			gameID: 1,
			players: [alice, bob, carol],
			currentPlayerID: 2,
			facilitatorID: 1,
		});
	});

	it('offers the two endings as chips, neither picked until a vote exists', () => {
		const chips = [...body.matchAll(/<button[^>]*class="[^"]*\bchip\b[^"]*"[^>]*>/g)].map((m) => m[0]);
		expect(chips).toHaveLength(2);
		for (const chip of chips) {
			expect(chip).toContain('aria-pressed="false"');
			expect(chip).not.toContain('selected');
		}
		expect(text(body)).toContain('Smooth Landing');
		expect(text(body)).toContain('Explosive Finale');
	});

	it('ties each chip to the words that say what its ending does', () => {
		for (const mode of ['smooth_landing', 'explosive_finale']) {
			expect(body).toContain(`aria-describedby="mode-text-${mode}"`);
			expect(body).toContain(`id="mode-text-${mode}"`);
		}
	});

	it('states the tally and the tie-break up front, in plain words', () => {
		expect(text(body)).toContain('0 of 3 votes in.');
		expect(text(body)).toContain('Yours is one of the ones missing');
		expect(text(body)).toContain("If the vote ties, alice's side wins");
	});
});

// ── the Shake-Up ────────────────────────────────────────────────────────────

describe('ShakeUpView', () => {
	withWindowStub();
	const rankings = [
		{ id: 1, game_id: 1, player_id: 1, category: 'knowledge', rank: 1 },
		{ id: 2, game_id: 1, player_id: 2, category: 'knowledge', rank: 2 },
	] as Ranking[];
	const props = (g: Game) => ({
		gameID: '1', game: g, players: [alice, bob], assets: [], rankings, currentPlayerID: 1,
		activeRoll: null, activeRollDice: [], activeRollParticipants: [],
		waitingOn: { waitees: [] },
	});
	let mid = '';
	let first = '';
	beforeAll(() => {
		mid = html(ShakeUpView, props(game({ phase: 'shake_up', shake_up_category: 'knowledge', shake_up_step: 2 })));
		first = html(ShakeUpView, props(game({ phase: 'shake_up', shake_up_category: 'esteem', shake_up_step: 1 })));
	});

	it('marks the act in play, and only that one, with its step in words', () => {
		const acts = mid.split('<li').slice(1).map((li) => li.split('</li>')[0]);
		const current = acts.filter((a) => a.includes('aria-current="step"'));
		expect(current).toHaveLength(1);
		expect(current[0]).toContain('Knowledge');
		expect(current[0]).toContain('Spending');
		// Esteem is behind it, Power ahead: neither names a step.
		expect(acts.filter((a) => a.includes('Spending'))).toHaveLength(1);
		expect(acts[0]).toContain('done');
		expect(acts[2]).not.toContain('done');
		expect(text(first)).toContain('Rolling');
	});

	it('opens "About the Shake-Up" on arrival only in the first act', () => {
		expect(text(first)).toContain('This is the finale.');
		expect(text(mid)).not.toContain('This is the finale.');
		expect(mid).toContain('About the Shake-Up');
	});

	it('draws the token pools as the shared roster, in turn order, with no chips', () => {
		expect(mid).toContain('class="table-roster');
		expect(mid).not.toContain('roller-chip');
	});

	it('says the same things it always did: your tokens, and a turn it is not yet', () => {
		expect(text(mid)).toContain('Your tokens: 0');
		expect(text(mid)).toContain('Token pools');
	});
});

// ── the lobby ───────────────────────────────────────────────────────────────

describe('LobbyView', () => {
	const props = (players: Player[], me: number, facilitator: boolean) => ({
		gameID: '1', game: game(), players, members: [], currentPlayerID: me,
		waitingPlayerIDs: new Set<number>(), isFacilitator: facilitator, vapidPublicKey: '',
		tonesSetCount: 0, onOpenTones: () => {}, onFeedback: () => {},
	});

	it('gives the host a Start Prologue button that waits for a second player', () => {
		const alone = html(LobbyView, props([alice], 1, true));
		expect(alone).toMatch(/<button[^>]*class="[^"]*action-btn primary[^"]*"[^>]*disabled/);
		expect(text(alone)).toContain('Is everyone here?');
		expect(text(alone)).toContain('One more player and you can begin.');

		const two = html(LobbyView, props([alice, bob], 1, true));
		expect(two).not.toMatch(/action-btn primary[^>]*disabled/);
		expect(text(two)).not.toContain('One more player');
	});

	it('gives a guest the verdict and no button to press', () => {
		const guest = html(LobbyView, props([alice, bob], 2, false));
		expect(text(guest)).toContain("You're seated. Nothing to do yet.");
		expect(text(guest)).toContain('alice will start the game');
		expect(guest).not.toContain('Start Prologue');
	});

	it('offers the join code on an invite chair while a seat is free, and not once the table is full', () => {
		const open = html(LobbyView, props([alice, bob], 1, true));
		expect(open).toContain('class="invite-chair');
		expect(open).toContain('ABC123');
		expect(text(open)).toContain('2 seated · room for 3 more');

		const five = [alice, bob, carol, player(4, 'dave'), player(5, 'erin')];
		const full = html(LobbyView, props(five, 1, true));
		expect(full).not.toContain('invite-chair');
		expect(text(full)).toContain('5 seated · table full');
	});

	it('lists the three optional things to do while waiting', () => {
		const body = text(html(LobbyView, props([alice, bob], 2, false)));
		expect(body).toContain('Read the two-minute primer');
		expect(body).toContain('Adjust the tone of the game');
	});
});

// ── the rules, read off the stylesheets ─────────────────────────────────────

const FILES = {
	'phases/LobbyView.svelte': ['.verdict', '.lobby-table', '.while-you-wait', '.phase-view'],
	'phases/ShakeUpView.svelte': ['.shake-up', '.step', '.tokens-panel', '.announce-form', 'header'],
	'phases/EndedView.svelte': ['.phase-view', '.standings', '.track'],
	'EndgameVotePanel.svelte': ['.endgame-vote', '.modes', '.mode', '.standing'],
};

describe('the four screens, as styled', () => {
	for (const [file, groups] of Object.entries(FILES)) {
		const source = read(file);
		const css = styleOf(source);
		const decls = declarations(css);

		it(`${file} carries no retired token, no opacity and no migration marker`, () => {
			for (const retired of [
				'--color-chip-', '--color-surface-active', '--color-surface-sunken',
				'--color-border-warm-strong', '--color-border-strong', '--color-accent-bright',
				'--color-text-muted', 'violet',
			]) {
				expect(css, `${retired} is retired`).not.toContain(retired);
			}
			expect(decls.filter((d) => d.prop === 'opacity'), 'greyed out is never faded').toEqual([]);
			expect(source.split('\n')[0], 'the marker is gone').not.toContain('tokens: not migrated');
		});

		it(`${file}: no group is drawn with an outline or a fill`, () => {
			for (const sel of groups) {
				const own = decls.filter((d) => d.selector.split(',').map((s) => s.trim()).includes(sel));
				const boxy = own.filter(
					(d) => /^(background|border|border-radius|box-shadow)(-color)?$/.test(d.prop)
						&& !(d.prop === 'background' && d.value === 'none'),
				);
				expect(boxy, `${sel} has an outline or a fill`).toEqual([]);
			}
		});
	}

	it('the invite chair is a control: dashed, neutral border, the control corner', () => {
		const decls = declarations(styleOf(read('phases/LobbyView.svelte')));
		const chair = (prop: string) => decls.find((d) => d.selector === '.invite-chair' && d.prop === prop)?.value;
		expect(chair('border')).toContain('dashed');
		expect(chair('border')).toContain('--color-border-control');
		expect(chair('border-radius')).toBe('var(--radius-control)');
		expect(chair('min-height')).toBe('var(--size-tap)');
	});

	it('a picked marginalia to tear is a bright gold border and a tick, with no fill', () => {
		const decls = declarations(styleOf(read('phases/ShakeUpView.svelte')));
		const picked = decls.filter((d) => d.selector === '.su-note.picked');
		expect(picked.find((d) => d.prop === 'border-color')?.value).toBe('var(--color-you)');
		expect(picked.find((d) => d.prop === 'box-shadow')?.value).toContain('--color-you');
		expect(picked.filter((d) => d.prop.startsWith('background'))).toEqual([]);
	});
});
