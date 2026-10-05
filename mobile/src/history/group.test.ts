import type { HistoryItem } from '../api/queries';
import { dayLabel, groupByDay, timeOf } from './group';

const payment = (id: number, created_at: string): HistoryItem => ({
  type: 'PAYMENT',
  id,
  reference: `PAY-${id}`,
  amount: 100000,
  status: 'SUCCEEDED',
  created_at,
  cashback: { awarded: 5000, reason: 'AWARDED' },
});

describe('dayLabel', () => {
  it.each([
    ['2026-10-03', '2026-10-03', 'TODAY, 3 OCT'],
    ['2026-10-02', '2026-10-03', 'YESTERDAY, 2 OCT'],
    ['2026-10-01', '2026-10-03', '1 OCT'],
    ['2026-09-30', '2026-10-01', 'YESTERDAY, 30 SEP'],
    ['2025-12-31', '2026-01-01', 'YESTERDAY, 31 DEC'],
    ['2026-10-04', '2026-10-10', '4 OCT'],
    ['2026-10-04', null, '4 OCT'],
  ])('%s with today %s is %s', (day, today, label) => {
    expect(dayLabel(day, today)).toBe(label);
  });
});

describe('timeOf', () => {
  it('reads the clock time as sent, with no zone conversion', () => {
    expect(timeOf('2026-10-04T00:30:00+07:00')).toBe('00:30');
  });
});

describe('groupByDay', () => {
  it('groups by the date part as sent and keeps the order', () => {
    const items = [
      payment(3, '2026-10-04T00:30:00+07:00'),
      payment(2, '2026-10-03T23:59:00+07:00'),
      payment(1, '2026-10-03T08:00:00+07:00'),
    ];
    const groups = groupByDay(items, '2026-10-04');
    expect(groups.map((g) => [g.label, g.items.map((i) => i.id)])).toEqual([
      ['TODAY, 4 OCT', [3]],
      ['YESTERDAY, 3 OCT', [2, 1]],
    ]);
  });
});
