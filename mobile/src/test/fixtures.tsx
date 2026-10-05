import AsyncStorage from '@react-native-async-storage/async-storage';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render } from '@testing-library/react-native';
import type { ReactElement } from 'react';

import type { Campaign, CashbackSummary, History } from '../api/queries';
import { AttemptProvider } from '../attempts/AttemptProvider';
import { UserProvider } from '../user/UserProvider';

export const campaign: Campaign = {
  id: 'flash-cashback',
  name: 'Flash Cashback',
  status: 'ACTIVE',
  redemption_status: 'AVAILABLE',
  rules: { rate_bps: 500, min_payment: 20000, daily_cap: 50000, timezone: 'Asia/Jakarta' },
};

export const cashback: CashbackSummary = {
  balance: 15000,
  today: { date: '2026-10-03', earned: 47000, remaining: 3000, resets_at: '2026-10-04T00:00:00+07:00' },
};

export const history: History = {
  items: [
    {
      type: 'REDEMPTION',
      id: 3,
      reference: 'RDM-20261003-000003',
      amount: 42000,
      status: 'COMPLETED',
      destination: 'MAIN_ACCOUNT',
      created_at: '2026-10-03T15:00:00+07:00',
    },
    {
      type: 'PAYMENT',
      id: 42,
      reference: 'PAY-20261003-000042',
      amount: 500000,
      status: 'SUCCEEDED',
      created_at: '2026-10-03T14:32:00+07:00',
      cashback: { awarded: 25000, reason: 'AWARDED' },
    },
  ],
};

export type Served = { campaign: unknown; cashback: unknown; history: unknown };
type Answer = unknown | Error;

const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });

export const fetchMock = jest.fn<Promise<Response>, [string, RequestInit & { headers: Record<string, string> }]>();

/** Serves the three read endpoints. A value that is an Error answers 500; the history `limit` is in the URL. */
export function serve(overrides: Partial<Record<keyof Served, Answer>> = {}) {
  const served: Record<keyof Served, Answer> = { campaign, cashback, history, ...overrides };
  fetchMock.mockReset();
  fetchMock.mockImplementation(async (url) => {
    const key = url.includes('/me/history') ? 'history' : url.includes('/me/cashback') ? 'cashback' : 'campaign';
    const answer = served[key];
    return answer instanceof Error ? new Response('{}', { status: 500 }) : json(answer);
  });
  globalThis.fetch = fetchMock as unknown as typeof fetch;
}

export async function resetStorage() {
  await AsyncStorage.clear();
}

export function newClient() {
  // gcTime Infinity: no garbage-collection timer is left pending after a test, so Jest can exit.
  return new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } });
}

export async function renderScreen(ui: ReactElement, client = newClient()) {
  return render(
    <QueryClientProvider client={client}>
      <UserProvider>{ui}</UserProvider>
    </QueryClientProvider>,
  );
}

export const requestedUrls = () => fetchMock.mock.calls.map(([url]) => url);

/** Like `renderScreen`, with the AttemptProvider the money screens need. */
export async function renderApp(ui: ReactElement, client = newClient()) {
  return render(
    <QueryClientProvider client={client}>
      <UserProvider>
        <AttemptProvider>{ui}</AttemptProvider>
      </UserProvider>
    </QueryClientProvider>,
  );
}

export type MoneyAnswer = { status: number; body: unknown } | 'network' | 'hang';
export const PAID = {
  payment: { id: 42, reference: 'PAY-20261003-000042', amount: 100000, status: 'SUCCEEDED', created_at: '2026-10-03T14:32:00+07:00' },
  cashback: { awarded: 5000, reason: 'AWARDED' },
};
export const moneyOk = (body: unknown = PAID): MoneyAnswer => ({ status: 201, body });
export const moneyRejected = (status: number, code: string): MoneyAnswer => ({
  status,
  body: { error: { code, message: 'm', request_id: 'r' } },
});

export type Post = { url: string; user: string; key: string | undefined; body: unknown };
export const posts: Post[] = [];

/** Serves the reads like `serve`, and answers each POST from `answers` in turn (a 201 with PAID when it runs out). */
export function serveMoney(answers: MoneyAnswer[], overrides: Partial<Record<keyof Served, Answer>> = {}) {
  serve(overrides);
  posts.length = 0;
  const reads = fetchMock.getMockImplementation();
  fetchMock.mockImplementation(async (url, init) => {
    if (init.method !== 'POST') return reads?.(url, init) ?? new Response('{}', { status: 500 });
    posts.push({
      url,
      user: init.headers['X-User-ID'] ?? '',
      key: init.headers['Idempotency-Key'],
      body: JSON.parse(String(init.body)),
    });
    const answer = answers.shift() ?? moneyOk();
    if (answer === 'network') throw new TypeError('network');
    if (answer === 'hang') {
      return new Promise<Response>((_resolve, reject) => {
        init.signal?.addEventListener('abort', () => reject(new Error('aborted')));
      });
    }
    return new Response(JSON.stringify(answer.body), { status: answer.status });
  });
}
