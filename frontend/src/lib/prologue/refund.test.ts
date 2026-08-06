import { describe, it, expect } from 'vitest';
import type { CommittedHeart, PlayerCardRow } from '$lib/api';
import {
  cardRank,
  computeTrackRanking,
  computeFinalSlots,
  computeBrightHearts,
  openRanksForCount,
  placeableHeartCount,
  resolvedTracksAt,
  trackResolved,
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

describe('trackResolved', () => {
  it('locks only the tracks before the live one', () => {
    expect(trackResolved('power', 'declare_power')).toBe(false);
    expect(trackResolved('power', 'place_set_asides_power')).toBe(false);
    expect(trackResolved('power', 'declare_knowledge')).toBe(true);
    expect(trackResolved('knowledge', 'declare_knowledge')).toBe(false);
    expect(trackResolved('knowledge', 'declare_esteem')).toBe(true);
    expect(trackResolved('esteem', 'declare_esteem')).toBe(false);
  });

  it('treats a step past the ranking machine as everything locked', () => {
    expect(trackResolved('esteem', 'closing')).toBe(true);
    expect(resolvedTracksAt('declare_esteem')).toEqual(new Set(['power', 'knowledge']));
    expect(resolvedTracksAt('declare_power').size).toBe(0);
  });
});

// The server runs the same rule (game/prologue_placeable.go) to decide whether
// it still needs this player's Done tap, so these cases are the contract
// between the two: a disagreement means a button nothing is listening for.
describe('placeableHeartCount', () => {
  it('counts unspent ANY cards, ignoring suited ones', () => {
    const cards = [nat(1, 'H', 'K'), nat(1, 'H', '4'), nat(1, 'C', '9')];
    expect(placeableHeartCount(1, 'power', 'declare_power', cards, [])).toBe(2);
  });

  it('is zero for a player who never held one', () => {
    const cards = [nat(1, 'C', 'K'), nat(1, 'S', '3')];
    expect(placeableHeartCount(1, 'power', 'declare_power', cards, [])).toBe(0);
  });

  it('discounts cards locked into a resolved track', () => {
    const cards = [nat(1, 'H', 'K', 11), nat(1, 'H', '4', 12)];
    const spent = [heart(1, 'power', 'K', 11)];
    expect(placeableHeartCount(1, 'knowledge', 'declare_knowledge', cards, spent)).toBe(1);
  });

  it('is zero once every card the player held is locked away', () => {
    const cards = [nat(1, 'H', 'K', 11), nat(1, 'H', '4', 12)];
    const spent = [heart(1, 'power', 'K', 11), heart(1, 'knowledge', '4', 12)];
    expect(placeableHeartCount(1, 'esteem', 'declare_esteem', cards, spent)).toBe(0);
  });

  it('still counts a card sitting on the live track — it can be retracted', () => {
    const cards = [nat(1, 'H', 'K', 11)];
    const live = [heart(1, 'esteem', 'K', 11)];
    expect(placeableHeartCount(1, 'esteem', 'declare_esteem', cards, live)).toBe(1);
  });

  it('ignores other players entirely', () => {
    const cards = [nat(1, 'H', 'K', 11), nat(2, 'H', 'Q', 21)];
    const spent = [heart(2, 'power', 'Q', 21)];
    expect(placeableHeartCount(1, 'knowledge', 'declare_knowledge', cards, spent)).toBe(1);
    expect(placeableHeartCount(2, 'knowledge', 'declare_knowledge', cards, spent)).toBe(0);
  });
});
