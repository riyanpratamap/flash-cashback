import { splitByAge } from './launch';
import type { SavedAttempt } from './types';

const NOW = Date.parse('2026-10-03T10:00:00Z');
const minutesAgo = (m: number) => new Date(NOW - m * 60_000).toISOString();
const attempt = (key: string, created_at: string): SavedAttempt => ({
  user_id: 'user_a',
  kind: 'payment',
  amount: 100000,
  key,
  created_at,
});

describe('splitByAge (D48)', () => {
  it.each([
    ['0 minutes', minutesAgo(0), 'recent'],
    ['9 min 59 s', new Date(NOW - 599_000).toISOString(), 'recent'],
    ['exactly 10 minutes', minutesAgo(10), 'unconfirmed'],
    ['11 minutes', minutesAgo(11), 'unconfirmed'],
    ['negative age (clock moved back)', minutesAgo(-30), 'recent'],
    ['unreadable time', 'not a date', 'unconfirmed'],
  ])('%s', (_name, createdAt, group) => {
    const split = splitByAge([attempt('K', createdAt)], NOW);
    expect(split.recent.map((a) => a.key)).toEqual(group === 'recent' ? ['K'] : []);
    expect(split.unconfirmed.map((a) => a.key)).toEqual(group === 'unconfirmed' ? ['K'] : []);
  });

  it('orders each group oldest first', () => {
    const split = splitByAge(
      [attempt('new', minutesAgo(1)), attempt('old', minutesAgo(9)), attempt('x2', minutesAgo(30)), attempt('x1', minutesAgo(60))],
      NOW,
    );
    expect(split.recent.map((a) => a.key)).toEqual(['old', 'new']);
    expect(split.unconfirmed.map((a) => a.key)).toEqual(['x1', 'x2']);
  });
});
