import { describe, expect, it } from 'vitest';
import type { ToneTopic, ToneTopicStatus } from '$lib/api';
import { lockedTones, restLine, TONE_ORDER } from './toneGroups';

let nextID = 1;
const topic = (name: string, status: ToneTopicStatus): ToneTopic => ({
	id: nextID++,
	game_id: 1,
	topic: name,
	status,
});

describe('lockedTones', () => {
	const topics = [
		topic('Betrayal', 'include'),
		topic('Plague', 'avoid_detail'),
		topic('Religion', 'default'),
		topic('Harm to children', 'never'),
		topic('Poison', 'include'),
		topic('Spiders', 'default'),
		topic('Torture', 'avoid_detail'),
	];

	it('groups Never first, then Avoid detail, then Include', () => {
		expect(TONE_ORDER).toEqual(['never', 'avoid_detail', 'include']);
		const { groups } = lockedTones(topics);
		expect(groups.map((g) => g.label)).toEqual(['Never', 'Avoid detail', 'Include']);
	});

	it('lists only topics someone set, in the order the server sent them', () => {
		const { groups } = lockedTones(topics);
		expect(groups.map((g) => g.topics.map((t) => t.topic))).toEqual([
			['Harm to children'],
			['Plague', 'Torture'],
			['Betrayal', 'Poison'],
		]);
		// A topic nobody set is in no group.
		expect(groups.flatMap((g) => g.topics).some((t) => t.status === 'default')).toBe(false);
	});

	it('counts the rest rather than listing them', () => {
		expect(lockedTones(topics).restCount).toBe(2);
	});

	it('leaves out a group with nothing in it', () => {
		const { groups } = lockedTones([topic('Poison', 'include'), topic('Fire', 'default')]);
		expect(groups.map((g) => g.status)).toEqual(['include']);
	});

	it('has no groups and counts everything when nothing was set', () => {
		const lone = [topic('Fire', 'default'), topic('Gore', 'default')];
		expect(lockedTones(lone)).toEqual({ groups: [], restCount: 2 });
	});
});

describe('restLine', () => {
	it('counts the topics with no opinion', () => {
		expect(restLine(7, 34)).toBe('34 more with no opinion.');
	});

	it('says so plainly when nothing was set', () => {
		expect(restLine(0, 41)).toBe('All 41 topics have no opinion.');
		expect(restLine(0, 1)).toBe('The one topic has no opinion.');
	});

	it('has no line when every topic was set', () => {
		expect(restLine(41, 0)).toBeNull();
	});
});
