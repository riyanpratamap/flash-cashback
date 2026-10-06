import { campaignQuery, cashbackQuery, historyQuery, queryKeys } from '@/api/queries';

const fetchMock = jest.fn<Promise<Response>, [string, RequestInit & { headers: Record<string, string> }]>();

beforeEach(() => {
  fetchMock.mockReset();
  globalThis.fetch = fetchMock as unknown as typeof fetch;
});

const ok = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });

const campaign = {
  id: 'flash-cashback',
  name: 'Flash Cashback',
  status: 'ACTIVE',
  redemption_status: 'AVAILABLE',
  rules: { rate_bps: 500, min_payment: 20000, daily_cap: 50000, timezone: 'Asia/Jakarta' },
};
const cashback = {
  balance: 15000,
  today: { date: '2026-10-03', earned: 47000, remaining: 3000, resets_at: '2026-10-04T00:00:00+07:00' },
};
const history = {
  next_cursor: null,
  items: [
    {
      type: 'PAYMENT',
      id: 42,
      reference: 'PAY-20261003-000042',
      amount: 100000,
      status: 'SUCCEEDED',
      created_at: '2026-10-03T14:32:00+07:00',
      cashback: { awarded: 5000, reason: 'AWARDED' },
    },
    {
      type: 'REDEMPTION',
      id: 3,
      reference: 'RDM-20261003-000003',
      amount: 42000,
      status: 'COMPLETED',
      destination: 'MAIN_ACCOUNT',
      created_at: '2026-10-03T11:20:00+07:00',
    },
  ],
};

describe('query keys', () => {
  it('match the spec', () => {
    expect(queryKeys.campaign()).toEqual(['campaign']);
    expect(queryKeys.cashback('user_a')).toEqual(['cashback', 'user_a']);
    expect(queryKeys.history('user_a', 20)).toEqual(['history', 'user_a', 20]);
  });
});

describe('query functions', () => {
  it('campaign reads GET /campaign', async () => {
    fetchMock.mockResolvedValue(ok(campaign));
    expect(await campaignQuery('user_a')).toEqual(campaign);
    expect(fetchMock.mock.calls[0]?.[0]).toBe('http://localhost:8080/v1/campaign');
  });

  it('cashback reads GET /me/cashback as the given user', async () => {
    fetchMock.mockResolvedValue(ok(cashback));
    expect(await cashbackQuery('user_c')).toEqual(cashback);
    expect(fetchMock.mock.calls[0]?.[0]).toBe('http://localhost:8080/v1/me/cashback');
    expect(fetchMock.mock.calls[0]?.[1].headers['X-User-ID']).toBe('user_c');
  });

  it('history reads GET /me/history?limit=', async () => {
    fetchMock.mockResolvedValue(ok(history));
    expect(await historyQuery('user_a', 5)).toEqual(history);
    expect(fetchMock.mock.calls[0]?.[0]).toBe('http://localhost:8080/v1/me/history?limit=5');
  });

  it('history sends the cursor when given and omits it when absent', async () => {
    fetchMock.mockImplementation(async () => ok({ ...history, next_cursor: 'abc_-' }));
    expect((await historyQuery('user_a', 20)).next_cursor).toBe('abc_-');
    expect(fetchMock.mock.calls[0]?.[0]).toBe('http://localhost:8080/v1/me/history?limit=20');
    await historyQuery('user_a', 20, 'abc_-');
    expect(fetchMock.mock.calls[1]?.[0]).toBe('http://localhost:8080/v1/me/history?limit=20&cursor=abc_-');
  });

  it.each([['missing', undefined], ['a number', 7], ['a bool', false]])(
    'history with next_cursor %s is a parse error',
    async (_name, value) => {
      const { next_cursor: _drop, ...rest } = history;
      fetchMock.mockResolvedValue(ok(value === undefined ? rest : { ...rest, next_cursor: value }));
      await expect(historyQuery('user_a', 20)).rejects.toThrow();
    },
  );

  it('a body of the wrong shape is an error, not a value', async () => {
    fetchMock.mockResolvedValue(ok({ balance: '15000' }));
    await expect(cashbackQuery('user_a')).rejects.toThrow();
  });

  it('a rejected or unknown answer is an error the query layer can retry', async () => {
    fetchMock.mockResolvedValueOnce(new Response('{}', { status: 400 }));
    await expect(campaignQuery('user_a')).rejects.toThrow();
    fetchMock.mockResolvedValueOnce(new Response('{}', { status: 503 }));
    await expect(campaignQuery('user_a')).rejects.toThrow();
  });
});
