import { act, fireEvent, screen } from '@testing-library/react-native';

import { FOCUS_FRESH_MS } from '@/api/hooks';
import type { HistoryItem } from '@/api/queries';
import { cashback, fetchMock, renderScreen, requestedUrls, resetStorage, serve } from '@/test/fixtures';
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
    expect(screen.getByText('00:30')).toBeTruthy();
  });

  it('shows the current balance header from the cashback summary', async () => {
    serve({ cashback: { ...cashback, balance: 18000 } });
    await renderScreen(<History />);
    expect(await screen.findByText('Rp18.000')).toBeTruthy();
    expect(screen.getByText('Current balance')).toBeTruthy();
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

  it('shows rows: earned with +, redemption neutral with −, Rp0 unsigned', async () => {
    serve({
      history: {
        items: [
          payment(3, '2026-10-03T14:32:00+07:00', 100000, 5000, 'AWARDED'),
          payment(2, '2026-10-03T13:05:00+07:00', 15000, 0, 'BELOW_MINIMUM'),
          redemption(1, '2026-10-03T11:20:00+07:00', 42000),
        ],
      },
    });
    await renderScreen(<History />);
    expect(await screen.findByText('Payment Rp100.000')).toBeTruthy();
    expect(screen.getByText('+Rp5.000')).toBeTruthy();
    expect(screen.getByText('Payment Rp15.000')).toBeTruthy();
    expect(screen.getByText('Rp0')).toBeTruthy();
    expect(screen.getByText('Redeemed to main account')).toBeTruthy();
    expect(screen.getByText('−Rp42.000')).toBeTruthy();
    expect(screen.getByText('11:20')).toBeTruthy();
  });

  it('shows the reason for a partial and for a Rp0 payment, not for a full award', async () => {
    serve({
      history: {
        items: [
          payment(4, '2026-10-03T15:00:00+07:00', 100000, 5000, 'AWARDED'),
          payment(3, '2026-10-03T14:00:00+07:00', 100000, 3000, 'PARTIAL_DAILY_CAP'),
          payment(2, '2026-10-03T13:05:00+07:00', 15000, 0, 'BELOW_MINIMUM'),
          payment(1, '2026-10-03T12:00:00+07:00', 100000, 0, 'DAILY_CAP_REACHED'),
        ],
      },
    });
    await renderScreen(<History />);
    expect(await screen.findByText('14:00 · Daily limit reached')).toBeTruthy();
    expect(screen.getByText('13:05 · Below minimum')).toBeTruthy();
    expect(screen.getByText('12:00 · Daily limit reached')).toBeTruthy();
    expect(screen.getByText('15:00')).toBeTruthy();
  });

  it('asks for 20 and shows at most 20 rows', async () => {
    const items = Array.from({ length: 25 }, (_, i) =>
      payment(100 - i, '2026-10-03T10:00:00+07:00', 100000, 5000, 'AWARDED'),
    );
    serve({ history: { items } });
    await renderScreen(<History />);
    await screen.findAllByText('Payment Rp100.000');
    expect(requestedUrls().some((u) => u.endsWith('/me/history?limit=20'))).toBe(true);
    expect(screen.getAllByText('Payment Rp100.000')).toHaveLength(20);
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
    expect(await screen.findByText('Redeemed to main account')).toBeTruthy();
    expect(screen.queryByText("Couldn't load your history.")).toBeNull();
  });

  it('refetches when the screen regains focus', async () => {
    await renderScreen(<History />);
    await screen.findByText('Redeemed to main account');
    const before = fetchMock.mock.calls.length;
    // The data is fresh for FOCUS_FRESH_MS; the clock moves past it, as when the user was away.
    const now = Date.now();
    const clock = jest.spyOn(Date, 'now').mockReturnValue(now + FOCUS_FRESH_MS + 1);
    await act(async () => regainFocus());
    clock.mockRestore();
    expect(fetchMock.mock.calls.length).toBeGreaterThan(before);
  });
});
