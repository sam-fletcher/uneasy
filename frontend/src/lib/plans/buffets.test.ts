import { describe, expect, it } from 'vitest';
import {
	DECREE_BUFFET, DUEL_BUFFET, WAR_BUFFET, LIAISE_BUFFET, demandBuffet,
} from './buffets';
import { DEMAND_OPTIONS, demandOptionInertReason } from '$lib/components/plans/shared';
import type { PlanType } from '$lib/api';

// What each plan's "What can happen?" says, from the book's plan pages. These pin the
// facts a player decides on (a count, who chooses, what is lost), not the exact words;
// the wording beyond the book is the owner's to settle (docs/work/questions).

const flat = (tabs: { label: string; always?: string; intro?: string; opts?: { label: string; desc?: string }[] }[]) =>
	tabs.map(t => [t.label, t.always, t.intro, ...(t.opts ?? []).flatMap(o => [o.label, o.desc])].filter(Boolean).join(' ')).join(' | ');
const tab = (tabs: ReturnType<typeof demandBuffet>, key: string) => tabs.find(t => t.key === key)!;
const text = (t: { always?: string; intro?: string; opts?: { label: string; desc?: string }[] }) =>
	flat([{ label: '', ...t }]);

describe('Propose Decree', () => {
	it('has a make and a mar', () => {
		expect(DECREE_BUFFET.map(t => t.key)).toEqual(['make', 'mar']);
	});

	it('make: an addendum with "but" or "and" puts the law in force, under Laws, and the preparer gains a resource', () => {
		const t = text(tab(DECREE_BUFFET, 'make'));
		expect(t).toMatch(/addendum/i);
		expect(t).toContain('“but” or “and”');
		expect(t).toMatch(/Laws/);
		expect(t).toMatch(/resource asset/);
	});

	it('mar: says the law may be changed, not that it is: amending is optional', () => {
		const t = text(tab(DECREE_BUFFET, 'mar'));
		expect(t).toContain('the council may change it');
		expect(t).not.toContain('not as the preparer wrote it');
	});

	it('mar: the other council members amend at will, lowest power first, then the addendum; no new resource', () => {
		const t = text(tab(DECREE_BUFFET, 'mar'));
		expect(t).toMatch(/each other council member may amend/i);
		expect(t).toMatch(/lowest power first/);
		expect(t).toMatch(/addendum/i);
		expect(t).toMatch(/no new resource/i);
	});

	it('speaks of no one in the second person', () => {
		expect(flat(DECREE_BUFFET)).not.toMatch(/\byou(r)?\b/i);
	});
});

describe('Propose Duel', () => {
	it('make: the preparer takes as many of the challenged player\'s staked assets as the result', () => {
		const t = text(tab(DUEL_BUFFET, 'make'));
		expect(t).toMatch(/preparer takes/);
		expect(t).toMatch(/result/);
		expect(t).toMatch(/challenged player/);
	});

	it('mar: the challenged player may take as many as the difficulty, or spare them, all or nothing', () => {
		const t = text(tab(DUEL_BUFFET, 'mar'));
		expect(t).toMatch(/challenged player may take/);
		expect(t).toMatch(/difficulty/);
		expect(t).toMatch(/spare/);
		expect(t).toMatch(/all or nothing/);
	});

	it('either way, every staked asset is then leveraged', () => {
		for (const k of ['make', 'mar']) expect(text(tab(DUEL_BUFFET, k))).toMatch(/leverage every asset/);
	});

	it('speaks of no one in the second person', () => {
		expect(flat(DUEL_BUFFET)).not.toMatch(/\byou(r)?\b/i);
	});
});

describe('Make War', () => {
	it('has no make or mar: a war rolls nothing', () => {
		expect(WAR_BUFFET.map(t => t.key)).not.toContain('make');
		expect(WAR_BUFFET.map(t => t.key)).not.toContain('mar');
	});

	it('the cost of battle: every row, reverse power order, and the four ways to pay', () => {
		const t = text(tab(WAR_BUFFET, 'cost'));
		expect(t).toMatch(/public record advances/);
		expect(t).toMatch(/reverse power order/);
		expect(t).toMatch(/Break an asset/);
		expect(t).toMatch(/Leverage two/);
		expect(t).toMatch(/surrender unconditionally/);
		expect(t).toMatch(/Propose peace/);
		expect(t).toMatch(/each opponent on the other side/i);
	});

	it('the cost is paid from the payer\'s OWN assets, not the opponent\'s', () => {
		const opts = tab(WAR_BUFFET, 'cost').opts!;
		expect(opts.find(o => o.key === 'break')!.desc).toContain('their own marginalia');
		expect(opts.find(o => o.key === 'leverage')!.desc).toContain('their own assets');
	});

	it('surrender: the war is lost and each opponent takes one asset of their choice', () => {
		const t = text(tab(WAR_BUFFET, 'surrender'));
		expect(t).toMatch(/loses the war/);
		expect(t).toMatch(/each opponent takes one asset of their choice/);
	});

	it('peace: anything everyone at war agrees to; an outsider who disagrees may pay to join', () => {
		const t = text(tab(WAR_BUFFET, 'peace'));
		expect(t).toMatch(/everyone at war agrees/);
		expect(t).toMatch(/not at war who disagrees may pay the cost of battle/);
	});
});

describe('Clandestinely Liaise', () => {
	it('has no make or mar: it rolls nothing', () => {
		expect(LIAISE_BUFFET.map(t => t.key)).not.toContain('make');
	});

	it('the things we share: the five options the book lists, each pick revealed together', () => {
		const share = tab(LIAISE_BUFFET, 'share');
		expect(share.opts).toHaveLength(5);
		const t = text(share);
		expect(t).toMatch(/revealed together/);
		expect(t).toMatch(/secrets/);
		expect(t).toMatch(/Update/);
		expect(t).toMatch(/Break/);
		expect(t).toMatch(/gift/);
		expect(t).toMatch(/bank a die/);
	});

	it('meeting again: a die face of 0 from either player calls the meeting off', () => {
		const t = text(tab(LIAISE_BUFFET, 'again'));
		expect(t).toMatch(/average, rounded up/);
		expect(t).toMatch(/either reveals no die \(0\)/);
		expect(t).toMatch(/called off/);
	});

	it('speaks of no one in the second person', () => {
		expect(flat(LIAISE_BUFFET)).not.toMatch(/\byou(r)?\b/i);
	});
});

describe('Make Demands, against each target', () => {
	const names = { demander: 'ada', target: 'bea' };
	const ALL: PlanType[] = [
		'exchange_courtiers', 'make_introductions', 'spread_propaganda', 'seek_answers', 'spread_rumors',
		'chronicle_histories', 'propose_decree', 'clandestinely_liaise', 'propose_duel', 'host_festivity',
	];

	it('has a make (the four options) and a mar (the counter-demand)', () => {
		const tabs = demandBuffet('seek_answers', names);
		expect(tabs.map(t => t.key)).toEqual(['make', 'mar']);
		expect(tab(tabs, 'make').opts).toHaveLength(4);
		expect(tab(tabs, 'make').opts!.map(o => o.key)).toEqual(DEMAND_OPTIONS);
	});

	it('make: who picks first, and that the four are shared out one at a time, alternately', () => {
		const t = text(tab(demandBuffet('seek_answers', names), 'make'));
		expect(t).toContain('ada and bea take turns choosing one option at a time');
		expect(t).not.toContain('one option each');
		expect(t).toContain('higher on the power track');
		expect(t).toContain('Seek Answers');
	});

	it('mar: the target rebuffs and places a demand back on the demander, now or on their next plan', () => {
		const t = text(tab(demandBuffet('seek_answers', names), 'mar'));
		expect(t).toContain('bea rebuffs');
		expect(t).toMatch(/one of ada.s plans straight away/);
		expect(t).toMatch(/next plan ada prepares/);
	});

	it.each(ALL)('against %s: each option that does nothing says so, with the same reason the draft card gives', (planType) => {
		const opts = tab(demandBuffet(planType, names), 'make').opts!;
		for (const o of opts) {
			const inert = demandOptionInertReason(o.key as (typeof DEMAND_OPTIONS)[number], planType);
			if (inert) {
				expect(o.desc, `${planType} ${o.key}`).toContain('No effect here');
				expect(o.desc).toContain(inert);
			} else {
				expect(o.desc, `${planType} ${o.key}`).not.toContain('No effect here');
			}
		}
	});

	it.each(ALL)('against %s: speaks of the people involved by name, never "you"', (planType) => {
		expect(flat(demandBuffet(planType, names))).not.toMatch(/\byou(r)?\b/i);
	});

	describe('control leverage', () => {
		it('lives where the target\'s preparer rolls: the winner chooses how many go onto the roll', () => {
			const o = tab(demandBuffet('spread_rumors', names), 'make').opts!.find(x => x.key === 'control_leverage')!;
			expect(o.desc).toMatch(/how many of bea.s assets go onto their roll/);
			// The winner chooses how many: all and none are only the most dramatic cases, not the choices.
			expect(o.desc).not.toMatch(/all of them|or none/);
		});

		it.each(['host_festivity', 'propose_duel', 'clandestinely_liaise'] as PlanType[])('is a dud against %s', (p) => {
			const o = tab(demandBuffet(p, names), 'make').opts!.find(x => x.key === 'control_leverage')!;
			expect(o.desc).toContain('No effect here');
		});
	});

	describe('keep or change the target: who, never what', () => {
		const line = (p: PlanType) => tab(demandBuffet(p, names), 'make').opts!.find(x => x.key === 'keep_or_change_target')!.desc!;
		it('names what each target plan is aimed at', () => {
			expect(line('exchange_courtiers')).toMatch(/which peer the exchange asks for/);
			expect(line('spread_rumors')).toMatch(/which asset the rumor is about/);
			expect(line('propose_duel')).toMatch(/who is challenged/);
			expect(line('clandestinely_liaise')).toMatch(/who the meeting is with/);
		});
		it.each(['seek_answers', 'spread_propaganda', 'propose_decree', 'chronicle_histories', 'make_introductions', 'host_festivity'] as PlanType[])('is a dud against %s', (p) => {
			expect(line(p)).toContain('No effect here');
		});
	});

	describe('keep assets: what the target\'s preparer would have gained', () => {
		const line = (p: PlanType) => tab(demandBuffet(p, names), 'make').opts!.find(x => x.key === 'keep_assets')!.desc!;
		it('names the thing each plan hands its preparer, and who it goes to instead', () => {
			expect(line('exchange_courtiers')).toMatch(/the peer the exchange brings in/);
			expect(line('make_introductions')).toMatch(/each newcomer who would join the preparer.s retinue/);
			expect(line('spread_propaganda')).toMatch(/the artifact the make creates/);
			expect(line('spread_rumors')).toMatch(/any asset the rumor takes/);
			expect(line('propose_decree')).toMatch(/the resource the law creates/);
			expect(line('propose_duel')).toMatch(/the staked assets the preparer takes/);
			expect(line('host_festivity')).toMatch(/what the host gains/);
			expect(line('clandestinely_liaise')).toMatch(/the gift the preparer takes/);
			for (const p of ['exchange_courtiers', 'spread_propaganda', 'propose_duel']) expect(line(p as PlanType)).toContain('in place of bea');
		});
		it.each(['seek_answers', 'chronicle_histories'] as PlanType[])('is a dud against %s: it gives no assets', (p) => {
			expect(line(p)).toContain('No effect here');
			expect(line(p)).toContain('gives its preparer no assets to keep');
		});
	});

	describe('perform steps: only once the roll is done, and a target-driven mar stays with its target', () => {
		const line = (p: PlanType) => tab(demandBuffet(p, names), 'make').opts!.find(x => x.key === 'perform_steps')!.desc!;
		it('is never a dud, and makes the choices after the roll', () => {
			for (const p of ['seek_answers', 'spread_propaganda', 'chronicle_histories', 'make_introductions']) {
				expect(line(p as PlanType)).not.toContain('No effect here');
				expect(line(p as PlanType)).toMatch(/options for bea once they have rolled/);
			}
		});
		it('Propose Decree has no options to pick: the stand-in enacts the law, and places the addendum when the preparer signs', () => {
			const t = line('propose_decree');
			expect(t).toContain('Enact the law in bea’s place once the dice have landed');
			expect(t).toContain('place the addendum too when bea is the signatory');
			expect(t).not.toContain('options');
		});

		it('Propose Duel: the stand-in plays the preparer\'s side of the bouts with the stakes already laid, and claims after a make', () => {
			const t = line('propose_duel');
			expect(t).toContain('side of the bouts');
			expect(t).toContain('the stakes they laid');
			expect(t).toContain('claim after a make');
			// It does not stake (staking comes before the roll): "choose the stakes" would read as that.
			expect(t).not.toMatch(/Choose the stakes/i);
		});

		it('keeps a mar with the plan\'s own target where that target chooses it: exchange, rumors, duel', () => {
			for (const p of ['exchange_courtiers', 'spread_rumors', 'propose_duel'] as PlanType[]) {
				expect(line(p), p).toMatch(/a mar.s (choice|claim) stays with the plan.s own target/i);
			}
			for (const p of ['seek_answers', 'spread_propaganda'] as PlanType[]) {
				expect(line(p), p).not.toMatch(/stays with/);
			}
		});
	});
});
