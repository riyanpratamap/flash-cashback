import type { ApiResult } from '@/api/client';
import { check, runPress, type AttemptState, type MachineDeps } from '@/attempts/machine';
import type { SavedAttempt } from '@/attempts/types';

const attempt: SavedAttempt = {
  user_id: 'user_a',
  kind: 'redemption',
  amount: 1000,
  key: 'K1',
  created_at: '2026-10-03T10:00:00.000Z',
};

const deps = (result: ApiResult): MachineDeps => ({
  save: async () => undefined,
  remove: async () => undefined,
  markResolved: async () => undefined,
  send: async () => result,
  wait: async () => undefined,
  report: () => undefined,
});

describe('done carries replayed (AC-65a, AC-65b)', () => {
  it.each([true, false])('an ok result with replayed %s gives done.replayed %s', async (replayed) => {
    const final: AttemptState = await runPress(attempt, deps({ kind: 'ok', status: replayed ? 200 : 201, body: {}, replayed }));
    expect(final).toMatchObject({ phase: 'done', replayed });
  });
});

describe('a replayed answer on the resend path (AC-65b)', () => {
  it('check() gives done.replayed true for a replayed ok answer', async () => {
    const final = await check(attempt, deps({ kind: 'ok', status: 200, body: {}, replayed: true }), true);
    expect(final).toMatchObject({ phase: 'done', replayed: true });
  });
});
