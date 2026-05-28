import { describe, it, expect } from 'vitest';
import type { CommittedHeart, PlayerCardRow } from '$lib/api';
import {
  cardRank,
  computeTrackRanking,
  computeFinalSlots,
  computeBrightHearts,
  openRanksForCount,
} from './refund';

// Small helpers to keep fixtures terse.
function nat(player_id: number, suit: 'C' | 'D' | 'S' | 'H', value: string, id = player_id * 100 + cardRank(value)): PlayerCardRow {
  return { id, game_id: 1, player_id, card_suit: suit, card_value: value };
}
function heart(player_id: number, track: 'power' | 'knowledge' | 'esteem', value: string, card_id = player_id * 1000 + cardRank(value)): CommittedHeart {
  return { player_id, track, card_id, value, suit: 'H' };
}

describe('cardRank', () => {
  it('ranks face cards above number cards', () => {
    expect(cardRank('A')).toBeGreaterThan(cardRank('K'));
    expect(cardRank('K')).toBeGreaterThan(cardRank('Q'));
    expect(cardRank('10')).toBeGreaterThan(cardRank('9'));
  });
  it('returns 0 for unknown values', () => {
    expect(cardRank('Z')).toBe(0);
  });
});

describe('openRanksForCount', () => {
  it('uses all five slots for a full table', () => {
    expect(openRanksForCount(5)).toEqual([1, 2, 3, 4, 5]);
  });
  it('inserts dummies at the documented positions', () => {
    expect(openRanksForCount(4)).toEqual([1, 2, 4, 5]);
    expect(openRanksForCount(3)).toEqual([2, 3, 4]);
    expect(openRanksForCount(2)).toEqual([2, 4]);
  });
});

describe('computeTrackRanking', () => {
  it('ranks players by descending card count, then by highest card', () => {
    // Power → clubs. p1 has two clubs, p2 has one higher club.
    const cards = [nat(1, 'C', 'K'), nat(1, 'C', '5'), nat(2, 'C', 'A')];
    const r = computeTrackRanking('power', [1, 2], cards, []);
    // p1 has 2 cards vs p2's 1 → p1 wins despite p2 holding the ace.
    expect(r.ranked).toEqual([1, 2]);
    expect(r.setAside).toEqual([]);
  });

  it('treats players with no contribution as set-aside', () => {
    const cards = [nat(1, 'C', 'K')];
    const r = computeTrackRanking('power', [1, 2, 3], cards, []);
    expect(r.ranked).toEqual([1]);
    expect(r.setAside.sort()).toEqual([2, 3]);
  });

  it('breaks ties between natural and heart by preferring natural', () => {
    // Both players have a single 9 on the power track. p1 holds it
    // naturally (a 9 of clubs); p2 has committed a 9 of hearts. Natural
    // beats heart at a tie, so p1 outranks p2.
    const cards = [nat(1, 'C', '9')];
    const hearts = [heart(2, 'power', '9')];
    const r = computeTrackRanking('power', [1, 2], cards, hearts);
    expect(r.ranked).toEqual([1, 2]);
  });

  it('filters by suit so hearts on the wrong track are ignored', () => {
    // p1 commits a heart to power; p2 commits a heart to knowledge.
    // Asking about the knowledge track should only see p2.
    const hearts = [heart(1, 'power', 'A'), heart(2, 'knowledge', '5')];
    const r = computeTrackRanking('knowledge', [1, 2], [], hearts);
    expect(r.ranked).toEqual([2]);
    expect(r.setAside).toEqual([1]);
  });
});

describe('computeFinalSlots', () => {
  it('assigns slot numbers from the open-ranks sequence', () => {
    // 3-player table → open ranks [2, 3, 4]. p1 has the most cards, p3
    // is set aside.
    const cards = [nat(1, 'C', 'K'), nat(1, 'C', '5'), nat(2, 'C', 'A')];
    const slots = computeFinalSlots('power', [1, 2, 3], cards, []);
    expect(slots.get(1)).toBe(2);
    expect(slots.get(2)).toBe(3);
    expect(slots.get(3)).toBe(4); // set-aside appended → final slot
  });
});

describe('computeBrightHearts', () => {
  it('marks a decisive heart as bright', () => {
    // p1 has only a committed heart on power; without it she'd be
    // set-aside in slot 4 instead of ranked in slot 2. The heart is
    // load-bearing → bright.
    const hearts = [heart(1, 'power', 'A')];
    const cards = [nat(2, 'C', '5')];
    const result = computeBrightHearts('power', [1, 2, 3], cards, hearts);
    const p1Bright = result.get(1)!;
    expect(p1Bright.size).toBe(1);
    expect([...p1Bright][0]).toBe(hearts[0].card_id);
  });

  it('marks a redundant heart as grey', () => {
    // p1 has a natural king of clubs (locking slot 1) AND a committed
    // heart. Removing the heart doesn't change p1's slot → grey.
    const cards = [nat(1, 'C', 'K'), nat(2, 'C', '5')];
    const hearts = [heart(1, 'power', '3')];
    const result = computeBrightHearts('power', [1, 2], cards, hearts);
    const p1Bright = result.get(1)!;
    expect(p1Bright.size).toBe(0);
  });
});
