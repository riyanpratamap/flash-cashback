import { act, fireEvent, screen, waitFor } from '@testing-library/react-native';

import { FOCUS_FRESH_MS } from '@/api/hooks';
import type { HistoryItem } from '@/api/queries';
import { cashback, fetchMock, newClient, renderScreen, requestedUrls, resetStorage, serve } from '@/test/fixtures';
import { regainFocus } from '@/test/router-mock';
import History from '@/app/history';

jest.mock('@react-native-async-storage/async-storage', () =>
  jest.requireActual('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
jest.mock('expo-router', () => jest.requireActual('@/test/router-mock'));

beforeEach(async () => {
  await resetStorage();
  serve();
});
afterEach(() => {
  jest.restoreAllMocks(); // the Date.now spy of a focus test
});

const payment = (id: number, created_at: string, amount: number, awarded: number, reason: string): HistoryItem => ({
  type: 'PAYMENT',
  id,
  reference: `PAY-${id}`,
  amount,
  status: 'SUCCEEDED',
  created_at,
  cashback: { awarded, reason },
});

const redemption = (id: number, created_at: string, amount: number): HistoryItem => ({
  type: 'REDEMPTION',
  id,
  reference: `RDM-${id}`,
  amount,
  status: 'COMPLETED',
  destination: 'MAIN_ACCOUNT',
  created_at,
});

describe('History (AC-66)', () => {
  it('runs with the device in UTC', () => {
    expect(new Date(2026, 0, 1).getTimezoneOffset()).toBe(0);
  });

  it('groups an item at 00:30 WIB under 4 Oct, not the UTC day before', async () => {
    serve({
      cashback: { ...cashback, today: { ...cashback.today, date: '2026-10-10' } },
      history: { items: [payment(9, '2026-10-04T00:30:00+07:00', 100000, 5000, 'AWARDED')] },
    });
    await renderScreen(<History />);
    expect(await screen.findByText('4 OCT')).toBeTruthy();
    expect(screen.queryByText('3 OCT')).toBeNull();
    expect(screen.getByText('00:30 · Earned Rp5.000 cashback')).toBeTruthy();
  });

  it('has no balance header, while the TODAY label still comes from the cashback summary', async () => {
    serve({
      cashback: { ...cashback, balance: 18000 },
      history: { items: [payment(1, '2026-10-03T14:32:00+07:00', 100000, 5000, 'AWARDED')] },
    });
    await renderScreen(<History />);
    expect(await screen.findByText('TODAY, 3 OCT')).toBeTruthy();
    expect(screen.queryByText('Cashback balance')).toBeNull();
    expect(screen.queryByText('Rp18.000')).toBeNull();
  });

  it('labels TODAY and YESTERDAY from the campaign day the API reports', async () => {
    serve({
      history: {
        items: [
          payment(3, '2026-10-03T14:32:00+07:00', 100000, 5000, 'AWARDED'),
          redemption(2, '2026-10-03T11:20:00+07:00', 42000),
          payment(1, '2026-10-02T19:45:00+07:00', 200000, 10000, 'AWARDED'),
        ],
      },
    });
    await renderScreen(<History />);
    expect(await screen.findByText('TODAY, 3 OCT')).toBeTruthy();
    expect(screen.getByText('YESTERDAY, 2 OCT')).toBeTruthy();
  });

  it('shows each row as a transaction: payment out, cashback in the subtitle, redemption in', async () => {
    serve({
      history: {
        items: [
          payment(4, '2026-10-03T14:32:00+07:00', 100000, 5000, 'AWARDED'),
          payment(3, '2026-10-03T14:00:00+07:00', 60000, 2000, 'PARTIAL_DAILY_CAP'),
          payment(2, '2026-10-03T13:05:00+07:00', 15000, 0, 'BELOW_MINIMUM'),
          redemption(1, '2026-10-03T11:20:00+07:00', 42000),
        ],
      },
    });
    await renderScreen(<History />);
    expect(await screen.findByText('14:32 · Earned Rp5.000 cashback')).toBeTruthy();
    expect(screen.getByText('−Rp100.000')).toBeTruthy();
    expect(screen.getByText('14:00 · Earned Rp2.000 cashback · Daily limit reached')).toBeTruthy();
    expect(screen.getByText('−Rp60.000')).toBeTruthy();
    expect(screen.getByText('13:05')).toBeTruthy();
    expect(screen.queryByText(/No cashback/)).toBeNull();
    expect(screen.queryByText(/Below minimum/)).toBeNull();
    expect(screen.getByText('−Rp15.000')).toBeTruthy();
    expect(screen.getAllByText('Payment')).toHaveLength(3);
    expect(screen.getByText('Cashback redeemed')).toBeTruthy();
    expect(screen.getByText('11:20 · To main account')).toBeTruthy();
    expect(screen.getByText('+Rp42.000')).toBeTruthy();
  });

  it('shows the empty copy', async () => {
    serve({ history: { items: [] } });
    await renderScreen(<History />);
    expect(await screen.findByText('No activity yet. Make a payment to start earning cashback.')).toBeTruthy();
  });

  it('shows the error with Try again, never an empty list', async () => {
    serve({ history: new Error('boom') });
    await renderScreen(<History />);
    expect(await screen.findByText("Couldn't load your history.")).toBeTruthy();
    expect(screen.queryByText(/No activity yet/)).toBeNull();

    serve();
    await fireEvent.press(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('Cashback redeemed')).toBeTruthy();
    expect(screen.queryByText("Couldn't load your history.")).toBeNull();
  });

  it('refetches when the screen regains focus', async () => {
    await renderScreen(<History />);
    await screen.findByText('Cashback redeemed');
    const before = fetchMock.mock.calls.length;
    // The data is fresh for FOCUS_FRESH_MS; the clock moves past it, as when the user was away.
    const now = Date.now();
    jest.spyOn(Date, 'now').mockReturnValue(now + FOCUS_FRESH_MS + 1);
    await act(async () => regainFocus());
    expect(fetchMock.mock.calls.length).toBeGreaterThan(before);
    // Let the refetch's notification land inside this test, not in the next one.
    await act(async () => new Promise((resolve) => setTimeout(resolve, 10)));
  });
});

describe('History paging (AC-66c)', () => {
  // 25 items: ids 100..76, the newest 21 on 3 Oct, the last 4 on 2 Oct. Page 1 is 20 items with cursor C1.
  const all = Array.from({ length: 25 }, (_, i) =>
    payment(100 - i, i < 21 ? '2026-10-03T10:00:00+07:00' : '2026-10-02T10:00:00+07:00', 100000, 5000, 'AWARDED'),
  );
  const PAGE_ONE = { items: all.slice(0, 20), next_cursor: 'C1' };
  const PAGE_TWO = { items: all.slice(20), next_cursor: null };
  const historyUrls = () => requestedUrls().filter((u) => u.includes('/me/history'));

  /** Page 1 without a cursor, page 2 for C1; `second` can answer the second page differently (an Error is a 500). */
  function servePages(second: unknown = PAGE_TWO) {
    serve();
    const base = fetchMock.getMockImplementation();
    fetchMock.mockImplementation(async (url, init) => {
      if (!url.includes('/me/history')) return base!(url, init);
      if (!url.includes('cursor=')) return new Response(JSON.stringify(PAGE_ONE), { status: 200 });
      return second instanceof Error ? new Response('{}', { status: 500 }) : new Response(JSON.stringify(second), { status: 200 });
    });
  }
  const reachEnd = async () => fireEvent(await screen.findByTestId('history-list'), 'endReached');

  it('requests limit 20 with no cursor and shows 20 rows', async () => {
    servePages();
    await renderScreen(<History />);
    await screen.findAllByText('Payment');
    expect(historyUrls()).toEqual(['http://localhost:8080/v1/me/history?limit=20']);
    expect(screen.getAllByText('Payment')).toHaveLength(20);
  });

  it('sends the returned cursor at the end and appends the next 5 rows', async () => {
    servePages();
    await renderScreen(<History />);
    await screen.findAllByText('Payment');
    await reachEnd();
    await waitFor(() => expect(screen.getAllByText('Payment')).toHaveLength(25));
    expect(historyUrls()).toEqual([
      'http://localhost:8080/v1/me/history?limit=20',
      'http://localhost:8080/v1/me/history?limit=20&cursor=C1',
    ]);
  });

  it('shows one 3 Oct header across both pages and a spinner while loading more', async () => {
    servePages();
    await renderScreen(<History />);
    await screen.findAllByText('Payment');
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => (release = resolve));
    const paged = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation(async (url, init) => {
      if (url.includes('cursor=')) await gate;
      return paged(url, init);
    });
    await reachEnd();
    expect(await screen.findByLabelText('Loading more')).toBeTruthy();
    release();
    await waitFor(() => expect(screen.getAllByText('Payment')).toHaveLength(25));
    expect(screen.queryByLabelText('Loading more')).toBeNull();
    expect(screen.getAllByText('TODAY, 3 OCT')).toHaveLength(1);
    expect(screen.getAllByText('YESTERDAY, 2 OCT')).toHaveLength(1);
  });

  it('keeps the loaded rows on a failed next page and Try again resends the same cursor', async () => {
    servePages(new Error('boom'));
    await renderScreen(<History />);
    await screen.findAllByText('Payment');
    await reachEnd();
    expect(await screen.findByText("Couldn't load more.")).toBeTruthy();
    expect(screen.getAllByText('Payment')).toHaveLength(20);
    expect(screen.queryByText("Couldn't load your history.")).toBeNull();

    servePages();
    await fireEvent.press(screen.getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(screen.getAllByText('Payment')).toHaveLength(25));
    expect(historyUrls().at(-1)).toBe('http://localhost:8080/v1/me/history?limit=20&cursor=C1');
    expect(screen.queryByText("Couldn't load more.")).toBeNull();
  });

  it('does not request again while the failed footer shows', async () => {
    servePages(new Error('boom'));
    await renderScreen(<History />);
    await screen.findAllByText('Payment');
    await reachEnd();
    await screen.findByText("Couldn't load more.");
    const before = historyUrls().length;
    await reachEnd();
    expect(historyUrls()).toHaveLength(before);
  });

  it('sends nothing and shows an empty footer after next_cursor is null', async () => {
    servePages();
    const client = newClient();
    await renderScreen(<History />, client);
    await screen.findAllByText('Payment');
    await reachEnd();
    await waitFor(() => expect(screen.getAllByText('Payment')).toHaveLength(25));
    const before = historyUrls().length;
    // Even a no-op fetchNextPage starts a fetch in the cache; the guard must not call it at all.
    let fetches = 0;
    const stop = client.getQueryCache().subscribe((event) => {
      if (event.type === 'updated' && event.action.type === 'fetch') fetches += 1;
    });
    await reachEnd();
    stop();
    expect(fetches).toBe(0);
    expect(historyUrls()).toHaveLength(before);
    expect(screen.queryByLabelText('Loading more')).toBeNull();
    expect(screen.queryByText("Couldn't load more.")).toBeNull();
  });
});
