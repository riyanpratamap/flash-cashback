import AsyncStorage from '@react-native-async-storage/async-storage';
import { act, fireEvent, screen } from '@testing-library/react-native';
import { randomUUID } from 'expo-crypto';
import { useState } from 'react';
import { Pressable, Text } from 'react-native';

import { useAttemptActions, useAttemptLaunch, useAttemptState } from '@/attempts/AttemptProvider';
import { ATTEMPTS_STORAGE_KEY } from '@/attempts/store';
import {
  campaign,
  cashback,
  fetchMock,
  moneyOk,
  moneyRejected,
  moneyReplayed,
  newClient,
  posts,
  renderApp,
  requestedUrls,
  resetStorage,
  serveMoney,
  type MoneyAnswer,
} from '@/test/fixtures';
import { dismissTo, resetRouter } from '@/test/router-mock';
import Redeem from '@/app/redeem';

jest.mock('@react-native-async-storage/async-storage', () =>
  jest.requireActual('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
jest.mock('expo-router', () => jest.requireActual('@/test/router-mock'));
jest.mock('expo-crypto', () => ({ randomUUID: jest.fn() }));

const NOW = Date.parse('2026-10-03T10:00:00.000Z');
const settle = (ms: number) =>
  act(async () => {
    await jest.advanceTimersByTimeAsync(ms);
  });
const amountField = () => screen.getByLabelText('Amount to redeem (IDR)');
const type = (text: string) => fireEvent.changeText(amountField(), text);
const redeemed = (amount: number, balanceAfter: number) =>
  moneyOk({
    redemption: { id: 3, reference: 'RDM-20261003-000003', amount, status: 'COMPLETED', destination: 'MAIN_ACCOUNT' },
    balance_after: balanceAfter,
  });
const STORED = {
  redemption: { id: 3, reference: 'RDM-20261003-000003', amount: 10000, status: 'COMPLETED', destination: 'MAIN_ACCOUNT' },
  balance_after: 3000,
};
const getCount = (path: string) => requestedUrls().filter((url) => url.includes(path)).length;

function Harness() {
  const a = { state: useAttemptState(), ...useAttemptLaunch(), ...useAttemptActions() };
  return (
    <>
      <Text>phase: {a.state.phase}</Text>
      <Redeem />
    </>
  );
}

/** Serves the reads and POSTs; `reads` can change what the next GET answers (a balance that moved). */
async function mount(
  answers: MoneyAnswer[] = [],
  served: { cashback?: unknown; campaign?: unknown } = {},
  client = newClient(),
) {
  const reads = { cashback: served.cashback ?? cashback, campaign: served.campaign ?? campaign };
  serveMoney(answers, reads);
  const view = await renderApp(<Harness />, client);
  await screen.findByText('Available to redeem');
  await settle(0);
  return { view, reads, serveReads: (next: Partial<typeof reads>) => serveMoney(answers, { ...reads, ...next }) };
}

beforeEach(async () => {
  jest.useFakeTimers({ now: NOW });
  await resetStorage();
  resetRouter();
  let n = 0;
  (randomUUID as jest.Mock).mockImplementation(() => `K${++n}`);
});

afterEach(() => {
  jest.useRealTimers();
});

describe('Redeem form (AC-65)', () => {
  it('shows the balance, the limit line and the destination, with the button disabled until an amount is typed', async () => {
    await mount();
    expect(screen.getByText('Available to redeem')).toBeTruthy();
    expect(screen.getAllByText('Rp15.000').length).toBeGreaterThan(0);
    expect(screen.getByText('Up to Rp15.000')).toBeTruthy();
    expect(screen.getByText('Sent to')).toBeTruthy();
    expect(screen.getByText('Main account')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Redeem' })).toBeDisabled();
    await type('10000');
    expect(amountField().props.value).toBe('10.000');
    expect(screen.getByRole('button', { name: 'Redeem Rp10.000' })).toBeEnabled();
  });

  it('refetches the balance when it opens, and shows the fresh one rather than a cached one', async () => {
    const client = newClient();
    client.setQueryData(['cashback', 'user_a'], { ...cashback, balance: 1000 });
    await mount([], {}, client);
    expect(screen.getByText('Up to Rp15.000')).toBeTruthy();
    expect(getCount('/me/cashback')).toBe(1);
  });

  it('Redeem all fills the whole balance', async () => {
    await mount();
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem all' }));
    expect(amountField().props.value).toBe('15.000');
    expect(screen.getByRole('button', { name: 'Redeem Rp15.000' })).toBeEnabled();
  });

  it('zero amount keeps the button disabled', async () => {
    await mount();
    await type('0');
    expect(screen.getByRole('button', { name: 'Redeem' })).toBeDisabled();
  });

  it('a balance of 0 disables the button and Redeem all', async () => {
    await mount([], { cashback: { ...cashback, balance: 0 } });
    expect(screen.getByText('Up to Rp0')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Redeem all' })).toBeDisabled();
    await type('1000');
    expect(screen.getByRole('button', { name: /^Redeem Rp/ })).toBeDisabled();
  });

  it('paused redemptions: the button is disabled and the screen says so', async () => {
    await mount([], { campaign: { ...campaign, redemption_status: 'PAUSED' } });
    expect(screen.getByText('Redemption is temporarily unavailable. Your balance is safe.')).toBeTruthy();
    await type('1000');
    expect(screen.getByRole('button', { name: 'Redeem Rp1.000' })).toBeDisabled();
  });

  it('an ended campaign still lets the user redeem', async () => {
    await mount([], { campaign: { ...campaign, status: 'ENDED' } });
    await type('1000');
    expect(screen.getByRole('button', { name: 'Redeem Rp1.000' })).toBeEnabled();
  });

  it('a balance that failed to load is not shown as Rp0 and nothing can be sent', async () => {
    serveMoney([], { cashback: new Error('down') });
    await renderApp(<Harness />);
    await screen.findByText("Couldn't load your balance.");
    expect(screen.queryByText('Up to Rp0')).toBeNull();
    expect(screen.queryByText('Available to redeem')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Redeem' })).toBeNull();
  });
});

describe('Redeem press and success (AC-65)', () => {
  it('sends the amount with a key, and disables the button at once while it is in flight', async () => {
    await mount(['hang']);
    await type('10000');
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem Rp10.000' }));
    expect(screen.getByRole('button', { name: 'Redeem Rp10.000' })).toBeDisabled();
    await settle(0);
    expect(posts).toHaveLength(1);
    expect(posts[0]).toMatchObject({ url: expect.stringContaining('/redemptions'), key: 'K1', body: { amount: 10000 } });
  });

  it('a fresh 201 shows the success screen with the balance after, and Done returns Home and acknowledges', async () => {
    await mount([redeemed(10000, 5000)]);
    await type('10000');
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem Rp10.000' }));
    await settle(0);
    expect(screen.getByLabelText('Success')).toBeTruthy();
    expect(screen.getByRole('header', { name: 'Redemption successful' })).toBeTruthy();
    expect(screen.getByText('Rp10.000')).toBeTruthy();
    expect(screen.getByText('Sent to your main account')).toBeTruthy();
    expect(screen.getByText('Reference')).toBeTruthy();
    expect(screen.getByText('RDM-20261003-000003')).toBeTruthy();
    expect(screen.getByText('Sent to')).toBeTruthy();
    expect(screen.getByText('Main account')).toBeTruthy();
    expect(screen.getByText('Cashback balance')).toBeTruthy();
    expect(screen.getByText('Rp5.000')).toBeTruthy();
    expect(screen.queryByLabelText('Amount to redeem (IDR)')).toBeNull();
    await fireEvent.press(screen.getByRole('button', { name: 'Done' }));
    expect(dismissTo).toHaveBeenCalledWith('/');
    expect(screen.getByText('phase: idle')).toBeTruthy();
  });

  it('a replay (200, Idempotent-Replayed) shows the success screen with no balance, which may be stale', async () => {
    await mount([moneyReplayed(STORED)]);
    await type('10000');
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem Rp10.000' }));
    await settle(0);
    expect(screen.getByRole('header', { name: 'Redemption successful' })).toBeTruthy();
    expect(screen.getByText('Rp10.000')).toBeTruthy();
    expect(screen.getByText('RDM-20261003-000003')).toBeTruthy();
    expect(screen.queryByText('Cashback balance')).toBeNull();
    expect(screen.queryByText('Rp3.000')).toBeNull();
  });

  it('a body that cannot be parsed says it went through, points Home, and shows no balance', async () => {
    await mount([moneyOk({ unexpected: true })]);
    await type('10000');
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem Rp10.000' }));
    await settle(0);
    expect(screen.getByLabelText('Success')).toBeTruthy();
    expect(screen.getByRole('header', { name: 'Your redemption went through.' })).toBeTruthy();
    expect(screen.getByText('Check your balance on the home screen.')).toBeTruthy();
    expect(screen.queryByText('Cashback balance')).toBeNull();
    expect(screen.getByRole('button', { name: 'Done' })).toBeTruthy();
  });
});

describe('Redeem rejections (AC-65)', () => {
  it('INSUFFICIENT_BALANCE refetches the balance first, then shows the limit from it', async () => {
    const m = await mount([moneyRejected(422, 'INSUFFICIENT_BALANCE')]);
    m.serveReads({ cashback: { ...cashback, balance: 9000 } });
    const before = getCount('/me/cashback');
    await type('15000');
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem Rp15.000' }));
    await settle(0);
    expect(getCount('/me/cashback')).toBe(before + 1);
    expect(screen.getByText('You can redeem up to Rp9.000.')).toBeTruthy();
    expect(screen.getByText('Up to Rp9.000')).toBeTruthy();
  });

  it('does not show the limit until the slow balance refetch answers, and never the stale one', async () => {
    await mount([moneyRejected(422, 'INSUFFICIENT_BALANCE')]);
    const reads = fetchMock.getMockImplementation();
    fetchMock.mockImplementation(async (url, init) => {
      if (init.method === 'POST' || !url.includes('/me/cashback')) return reads?.(url, init) ?? new Response('{}');
      await new Promise((resolve) => setTimeout(resolve, 500));
      return new Response(JSON.stringify({ ...cashback, balance: 9000 }), { status: 200 });
    });
    await type('15000');
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem Rp15.000' }));
    await settle(100);
    expect(screen.queryByText(/You can redeem up to/)).toBeNull();
    await settle(500);
    expect(screen.getByText('You can redeem up to Rp9.000.')).toBeTruthy();
    expect(screen.queryByText('You can redeem up to Rp15.000.')).toBeNull();
  });

  it('a press after a rejection uses a new key', async () => {
    await mount([moneyRejected(422, 'INSUFFICIENT_BALANCE'), redeemed(10000, 5000)]);
    await type('15000');
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem Rp15.000' }));
    await settle(0);
    await type('10000');
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem Rp10.000' }));
    await settle(0);
    expect(posts.map((p) => p.key)).toEqual(['K1', 'K2']);
  });

  it('REDEMPTION_PAUSED refetches the campaign and shows the paused state', async () => {
    const m = await mount([moneyRejected(409, 'REDEMPTION_PAUSED')]);
    m.serveReads({ campaign: { ...campaign, redemption_status: 'PAUSED' } });
    const before = getCount('/campaign');
    await type('10000');
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem Rp10.000' }));
    await settle(0);
    expect(getCount('/campaign')).toBe(before + 1);
    expect(screen.getByText('Redemption is temporarily unavailable. Your balance is safe.')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Redeem Rp10.000' })).toBeDisabled();
  });

  it('any other rejection shows the generic message', async () => {
    await mount([moneyRejected(422, 'WHATEVER')]);
    await type('10000');
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem Rp10.000' }));
    await settle(0);
    expect(screen.getByText('Something went wrong. Please try again.')).toBeTruthy();
  });

  it('a rejection reached after the screen is open fills in its amount (launch resend)', async () => {
    await AsyncStorage.setItem(
      ATTEMPTS_STORAGE_KEY,
      JSON.stringify([
        { user_id: 'user_a', kind: 'redemption', amount: 9000, key: 'L', created_at: new Date(NOW - 60_000).toISOString() },
      ]),
    );
    serveMoney(['network', moneyRejected(422, 'INSUFFICIENT_BALANCE')], { cashback: { ...cashback, balance: 4000 } });
    await renderApp(<Harness />);
    await settle(2000);
    expect(amountField().props.value).toBe('9.000');
    expect(screen.getByText('You can redeem up to Rp4.000.')).toBeTruthy();
  });
});

describe('Redeem with a launch-resent redemption (AC-60)', () => {
  const saved = (key: string, minutes: number, amount = 18000) => ({
    user_id: 'user_a',
    kind: 'redemption',
    amount,
    key,
    created_at: new Date(NOW - minutes * 60_000).toISOString(),
  });

  it('shows the result, and leaving the screen acknowledges it so the next resend carries on', async () => {
    await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify([saved('NEWER', 2), saved('OLDER', 8)]));
    serveMoney([redeemed(18000, 0), redeemed(18000, 0)]);
    const Popped = () => {
      const [shown, setShown] = useState(true);
      return (
        <>
          <Pressable accessibilityRole="button" onPress={() => setShown(false)}>
            <Text>pop</Text>
          </Pressable>
          {shown ? <Redeem /> : null}
        </>
      );
    };
    await renderApp(<Popped />);
    await settle(5000);
    expect(screen.getByRole('header', { name: 'Redemption successful' })).toBeTruthy();
    expect(screen.getByText('Rp18.000')).toBeTruthy();
    expect(screen.getByText('Cashback balance')).toBeTruthy();
    expect(screen.getByText('Rp0')).toBeTruthy();
    expect(posts.map((p) => p.key)).toEqual(['OLDER']);
    await fireEvent.press(screen.getByRole('button', { name: 'pop' }));
    await settle(0);
    expect(posts.map((p) => p.key)).toEqual(['OLDER', 'NEWER']);
  });

  it('leaving a rejection acknowledges it too', async () => {
    await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify([saved('NEWER', 2), saved('OLDER', 8)]));
    serveMoney([moneyRejected(422, 'INSUFFICIENT_BALANCE'), redeemed(18000, 0)]);
    const Popped = () => {
      const [shown, setShown] = useState(true);
      return (
        <>
          <Pressable accessibilityRole="button" onPress={() => setShown(false)}>
            <Text>pop</Text>
          </Pressable>
          {shown ? <Redeem /> : null}
        </>
      );
    };
    await renderApp(<Popped />);
    await settle(5000);
    expect(posts.map((p) => p.key)).toEqual(['OLDER']);
    await fireEvent.press(screen.getByRole('button', { name: 'pop' }));
    await settle(0);
    expect(posts.map((p) => p.key)).toEqual(['OLDER', 'NEWER']);
  });
});
