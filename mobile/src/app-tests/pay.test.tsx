import AsyncStorage from '@react-native-async-storage/async-storage';
import { act, fireEvent, screen } from '@testing-library/react-native';
import { randomUUID } from 'expo-crypto';

import { useAttemptState } from '@/attempts/AttemptProvider';
import { ATTEMPTS_STORAGE_KEY } from '@/attempts/store';
import { campaign, cashback, moneyOk, moneyRejected, posts, renderApp, resetStorage, serveMoney } from '@/test/fixtures';
import Home from '@/app/index';
import Pay from '@/app/pay';

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
const amountField = () => screen.getByLabelText('Amount (IDR)');
const type = (text: string) => fireEvent.changeText(amountField(), text);

async function mount(answers = [moneyOk()], overrides = {}) {
  serveMoney(answers, overrides);
  const view = await renderApp(<Pay />);
  await screen.findByLabelText('Amount (IDR)');
  await settle(0);
  return view;
}

beforeEach(async () => {
  jest.useFakeTimers({ now: NOW });
  await resetStorage();
  let n = 0;
  (randomUUID as jest.Mock).mockImplementation(() => `K${++n}`);
});

afterEach(() => {
  jest.useRealTimers();
});

describe('Pay amount field', () => {
  it('keeps digits only and formats as typed', async () => {
    await mount();
    await type('abc1000x00');
    expect(amountField().props.value).toBe('100.000');
    await type('');
    expect(amountField().props.value).toBe('');
  });

  it('the chips fill the amount', async () => {
    await mount();
    for (const [chip, shown] of [
      ['Rp20.000', '20.000'],
      ['Rp50.000', '50.000'],
      ['Rp100.000', '100.000'],
    ] as const) {
      await fireEvent.press(screen.getByRole('button', { name: chip }));
      expect(amountField().props.value).toBe(shown);
    }
  });

  it('shows the minimum line from the rules, and the Pay button names the amount', async () => {
    await mount();
    expect(screen.getByText('Payments under Rp20.000 earn no cashback.')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Pay' })).toBeDisabled();
    await type('100000');
    expect(screen.getByRole('button', { name: 'Pay Rp100.000' })).toBeEnabled();
  });

  it('above Rp10.000.000 shows the message and disables Pay', async () => {
    await mount();
    await type('10000001');
    expect(screen.getByText('Enter an amount up to Rp10.000.000.')).toBeTruthy();
    expect(screen.getByRole('button', { name: /^Pay/ })).toBeDisabled();
    await type('10000000');
    expect(screen.queryByText('Enter an amount up to Rp10.000.000.')).toBeNull();
    expect(screen.getByRole('button', { name: 'Pay Rp10.000.000' })).toBeEnabled();
  });
});

describe('Pay info line (AC-63)', () => {
  it('estimates Rp3.000 when 5% is more than what is left today', async () => {
    await mount(); // fixture: Rp3.000 left today
    await type('100000');
    expect(screen.getByText("Earn up to Rp3.000 cashback, the rest of today's Rp50.000 limit.")).toBeTruthy();
  });

  it('shows the plain estimate when it fits', async () => {
    await mount([], { cashback: { ...cashback, today: { ...cashback.today, remaining: 50000 } } });
    await type('100000');
    expect(screen.getByText('Earn up to Rp5.000 cashback. Final amount is confirmed after payment.')).toBeTruthy();
  });

  it.each([
    ['below the minimum', '19999', {}, "This payment won't earn cashback. Payments under Rp20.000 earn no cashback."],
    [
      'campaign ended',
      '100000',
      { campaign: { ...campaign, status: 'ENDED' } },
      "This payment won't earn cashback. Flash Cashback has ended.",
    ],
    [
      'awards paused',
      '100000',
      { campaign: { ...campaign, status: 'PAUSED' } },
      "This payment won't earn cashback. Cashback is temporarily unavailable.",
    ],
    [
      'nothing left today',
      '100000',
      { cashback: { ...cashback, today: { ...cashback.today, earned: 50000, remaining: 0 } } },
      "This payment won't earn cashback. You've reached today's limit.",
    ],
  ])('%s', async (_name, amount, overrides, line) => {
    await mount([], overrides);
    await type(amount);
    expect(screen.getByText(line)).toBeTruthy();
  });

  it('shows no line before an amount is typed', async () => {
    await mount();
    expect(screen.queryByText(/cashback\. Final|won't earn/)).toBeNull();
  });
});

describe('Pay press (AC-58, AC-61)', () => {
  it('sends one payment with a new key, as the selected user, and disables at once', async () => {
    await mount();
    await type('100000');
    serveMoney(['hang']);
    await fireEvent.press(screen.getByRole('button', { name: 'Pay Rp100.000' }));
    expect(screen.getByRole('button', { name: 'Pay' })).toBeDisabled(); // at once, before the request is answered
    await settle(0);
    expect(screen.getByRole('button', { name: 'Pay' })).toBeDisabled(); // and while the request hangs
    expect(posts).toHaveLength(1);
    expect(posts[0]).toMatchObject({ url: expect.stringMatching(/\/payments$/), user: 'user_a', key: 'K1', body: { amount: 100000 } });
  });

  it('a rejected amount shows the inline copy, and the next press uses a new key', async () => {
    await mount([moneyRejected(422, 'INVALID_AMOUNT'), moneyOk()]);
    await type('100000');
    await fireEvent.press(screen.getByRole('button', { name: 'Pay Rp100.000' }));
    await settle(0);
    expect(screen.getByText('Enter an amount up to Rp10.000.000.')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Pay Rp100.000' })).toBeEnabled();
    await fireEvent.press(screen.getByRole('button', { name: 'Pay Rp100.000' }));
    await settle(0);
    expect(posts.map((p) => p.key)).toEqual(['K1', 'K2']);
  });

  it('any other rejection shows the generic copy', async () => {
    await mount([moneyRejected(409, 'SOMETHING_NEW')]);
    await type('100000');
    await fireEvent.press(screen.getByRole('button', { name: 'Pay Rp100.000' }));
    await settle(0);
    expect(screen.getByText('Something went wrong. Please try again.')).toBeTruthy();
    expect(screen.queryByText(/failed/i)).toBeNull();
  });

  it('an unsaved attempt shows the generic copy and sends nothing', async () => {
    await mount();
    jest.spyOn(AsyncStorage, 'setItem').mockRejectedValueOnce(new Error('disk'));
    await type('100000');
    await fireEvent.press(screen.getByRole('button', { name: 'Pay Rp100.000' }));
    await settle(0);
    expect(posts).toHaveLength(0);
    expect(screen.getByText('Something went wrong. Please try again.')).toBeTruthy();
  });
});

describe('Pay after a rejection found at launch (P5.3 F5)', () => {
  const seed = (...keys: [string, number][]) =>
    AsyncStorage.setItem(
      ATTEMPTS_STORAGE_KEY,
      JSON.stringify(
        keys.map(([key, minutes]) => ({
          user_id: 'user_a',
          kind: 'payment',
          amount: 100000,
          key,
          created_at: new Date(NOW - minutes * 60_000).toISOString(),
        })),
      ),
    );

  it('shows the rejection with its amount, and the next resend waits until Pay is left', async () => {
    await seed(['NEWER', 2], ['OLDER', 8]);
    serveMoney([moneyRejected(422, 'INVALID_AMOUNT'), moneyOk()]);
    // The navigator opens Pay once the rejection is known; this stands in for it.
    const AfterRejection = () => (useAttemptState().phase === 'rejected' ? <Pay /> : null);
    const view = await renderApp(<AfterRejection />);
    await settle(5000);
    expect(screen.getByText('Enter an amount up to Rp10.000.000.')).toBeTruthy();
    expect(amountField().props.value).toBe('100.000');
    expect(posts.map((p) => p.key)).toEqual(['OLDER']);
    await view.unmount();
    await settle(0);
    expect(posts.map((p) => p.key)).toEqual(['OLDER', 'NEWER']);
  });

  const rejectedAtLaunch = async (view: 'pay' | 'both') => {
    await seed(['NEWER', 2], ['OLDER', 8]);
    serveMoney([moneyRejected(422, 'INVALID_AMOUNT'), moneyOk(), moneyOk()]);
    await renderApp(
      view === 'both' ? (
        <>
          <Pay />
          <Home />
        </>
      ) : (
        <Pay />
      ),
    );
    await settle(5000);
  };

  it('F3: a rejection that arrives while Pay is already open fills the amount and shows the message', async () => {
    await rejectedAtLaunch('pay'); // Pay was mounted before the rejection existed
    expect(amountField().props.value).toBe('100.000');
    expect(screen.getByText('Enter an amount up to Rp10.000.000.')).toBeTruthy();
  });

  it('F2: pressing Pay while the launch queue waits sends a new payment; the second attempt becomes a card', async () => {
    await rejectedAtLaunch('both');
    expect(posts.map((p) => p.key)).toEqual(['OLDER']);
    await fireEvent.press(screen.getByRole('button', { name: 'Pay Rp100.000' }));
    await settle(0);
    await settle(20_000);
    expect(posts.map((p) => p.key)).toEqual(['OLDER', 'K1']); // sent once, with a new key; NEWER not resent by the launch
    await fireEvent.press(await screen.findByRole('button', { name: 'Check now' }));
    await settle(0);
    expect(posts.map((p) => p.key)).toEqual(['OLDER', 'K1', 'NEWER']); // the card kept its original key
  });
});
