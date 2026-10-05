import AsyncStorage from '@react-native-async-storage/async-storage';
import { act, fireEvent, screen } from '@testing-library/react-native';
import { randomUUID } from 'expo-crypto';
import { useState } from 'react';
import { Pressable, Text } from 'react-native';

import { useAttemptActions, useAttemptLaunch, useAttemptState } from '@/attempts/AttemptProvider';
import { ATTEMPTS_STORAGE_KEY } from '@/attempts/store';
import { moneyOk, PAID, posts, renderApp, resetStorage, serveMoney } from '@/test/fixtures';
import { dismissTo, push, resetRouter } from '@/test/router-mock';
import PaymentResult from '@/app/payment-result';

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

function Harness() {
  const a = { state: useAttemptState(), ...useAttemptLaunch(), ...useAttemptActions() };
  return (
    <>
      <Pressable accessibilityRole="button" onPress={() => a.press('payment', 100000)}>
        <Text>pay</Text>
      </Pressable>
      <Text>phase: {a.state.phase}</Text>
      <PaymentResult />
    </>
  );
}

async function pay(cashbackPart: unknown, payment: unknown = PAID.payment) {
  serveMoney([moneyOk({ payment, cashback: cashbackPart })]);
  await renderApp(<Harness />);
  await settle(0);
  await fireEvent.press(screen.getByRole('button', { name: 'pay' }));
  await settle(0);
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

describe('Payment result (AC-58)', () => {
  it('shows the payment as successful with amount, reference and time in WIB, and the cashback card', async () => {
    await pay({ awarded: 5000, reason: 'AWARDED' });
    expect(screen.getByText('Payment successful')).toBeTruthy();
    expect(screen.getByText('Rp100.000')).toBeTruthy();
    expect(screen.getByText('Amount')).toBeTruthy();
    expect(screen.getByText('PAY-20261003-000042')).toBeTruthy();
    expect(screen.getByText('3 Oct, 14:32 WIB')).toBeTruthy();
    expect(screen.getByText('Cashback earned')).toBeTruthy();
    expect(screen.getByText('+Rp5.000')).toBeTruthy();
    expect(screen.getByText('5% cashback added to your balance.')).toBeTruthy();
    expect(screen.queryByRole('link', { name: 'How it works' })).toBeNull();
    expect(screen.queryByText(/failed/i)).toBeNull();
  });

  it('Done returns Home and acknowledges the result', async () => {
    await pay({ awarded: 5000, reason: 'AWARDED' });
    await fireEvent.press(screen.getByRole('button', { name: 'Done' }));
    expect(dismissTo).toHaveBeenCalledWith('/');
    expect(screen.getByText('phase: idle')).toBeTruthy();
    expect(screen.queryByText('Payment successful')).toBeNull();
  });

  it('Make another payment returns to Home and opens Pay again', async () => {
    await pay({ awarded: 5000, reason: 'AWARDED' });
    await fireEvent.press(screen.getByRole('button', { name: 'Make another payment' }));
    expect(dismissTo).toHaveBeenCalledWith('/');
    expect(push).toHaveBeenCalledWith('/pay');
    expect(screen.getByText('phase: idle')).toBeTruthy();
  });

  it('shows nothing before a payment is done', async () => {
    serveMoney([]);
    await renderApp(<Harness />);
    await settle(0);
    expect(screen.queryByText('Payment successful')).toBeNull();
  });
});

describe('Payment result, each reason (AC-62)', () => {
  it.each([
    ['PARTIAL_DAILY_CAP', 3000, '+Rp3.000', 'Daily limit reached', "You've reached today's Rp50.000 cashback limit.", true],
    [
      'PARTIAL_BUDGET',
      1200,
      '+Rp1.200',
      'Last of the cashback',
      'This was the last of the campaign cashback. Flash Cashback has now ended.',
      true,
    ],
    ['DAILY_CAP_REACHED', 0, 'Rp0', 'Daily limit reached', "You've already reached today's Rp50.000 cashback limit.", true],
    ['BELOW_MINIMUM', 0, 'Rp0', 'Below minimum', "Payments under Rp20.000 don't earn cashback.", true],
    ['CAMPAIGN_ENDED', 0, 'Rp0', 'Campaign ended', 'Flash Cashback has ended. All cashback has been claimed.', true],
    [
      'CAMPAIGN_PAUSED',
      0,
      'Rp0',
      'Unavailable',
      'Cashback is temporarily unavailable. Payments still work as usual.',
      true,
    ],
  ])('%s', async (reason, awarded, amountText, chip, text, howItWorks) => {
    await pay({ awarded, reason });
    expect(screen.getByText('Payment successful')).toBeTruthy();
    expect(screen.getByText(amountText)).toBeTruthy();
    expect(screen.getByText(chip)).toBeTruthy();
    expect(screen.getByText(text)).toBeTruthy();
    expect(screen.queryByRole('link', { name: 'How it works' }) !== null).toBe(howItWorks);
  });

  it('an unknown code shows the amount as sent and the generic text', async () => {
    await pay({ awarded: 700, reason: 'BRAND_NEW_REASON' });
    expect(screen.getByText('+Rp700')).toBeTruthy();
    expect(screen.getByText('See How it works for the cashback rules.')).toBeTruthy();
    expect(screen.getByRole('link', { name: 'How it works' })).toBeTruthy();
  });

  it('the How it works link opens the rules', async () => {
    await pay({ awarded: 0, reason: 'BELOW_MINIMUM' });
    await fireEvent.press(screen.getByRole('link', { name: 'How it works' }));
    expect(push).toHaveBeenCalledWith('/how-it-works');
  });

  it('a body it cannot read still shows the payment as successful, with the amount sent', async () => {
    serveMoney([moneyOk({ unexpected: true })]);
    await renderApp(<Harness />);
    await settle(0);
    await fireEvent.press(screen.getByRole('button', { name: 'pay' }));
    await settle(0);
    expect(screen.getByText('Payment successful')).toBeTruthy();
    expect(screen.getByText('Rp100.000')).toBeTruthy();
    expect(screen.queryByText('Cashback earned')).toBeNull();
  });
});

describe('two recent attempts at launch (P5.3 F5)', () => {
  it('shows the first result until Done, and only then resends the second', async () => {
    const aged = (key: string, minutes: number) => ({
      user_id: 'user_a',
      kind: 'payment',
      amount: 100000,
      key,
      created_at: new Date(NOW - minutes * 60_000).toISOString(),
    });
    await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify([aged('NEWER', 2), aged('OLDER', 8)]));
    serveMoney([moneyOk(), moneyOk()]);
    await renderApp(<Harness />);
    await settle(5000);
    expect(screen.getByText('Payment successful')).toBeTruthy();
    expect(posts.map((p) => p.key)).toEqual(['OLDER']);
    await fireEvent.press(screen.getByRole('button', { name: 'Done' }));
    await settle(0);
    expect(posts.map((p) => p.key)).toEqual(['OLDER', 'NEWER']);
    expect(screen.getByText('Payment successful')).toBeTruthy(); // the second one's result
  });

  it('leaving the result without Done (Android back) still acknowledges it, so the second is resent', async () => {
    const aged = (key: string, minutes: number) => ({
      user_id: 'user_a',
      kind: 'payment',
      amount: 100000,
      key,
      created_at: new Date(NOW - minutes * 60_000).toISOString(),
    });
    await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify([aged('NEWER', 2), aged('OLDER', 8)]));
    serveMoney([moneyOk(), moneyOk()]);
    // The result screen is popped by the system, not by its own buttons.
    const Popped = () => {
      const [shown, setShown] = useState(true);
      return (
        <>
          <Pressable accessibilityRole="button" onPress={() => setShown(false)}>
            <Text>pop</Text>
          </Pressable>
          {shown ? <PaymentResult /> : null}
        </>
      );
    };
    await renderApp(<Popped />);
    await settle(5000);
    expect(screen.getByText('Payment successful')).toBeTruthy();
    expect(posts.map((p) => p.key)).toEqual(['OLDER']);
    await fireEvent.press(screen.getByRole('button', { name: 'pop' }));
    await settle(0);
    expect(posts.map((p) => p.key)).toEqual(['OLDER', 'NEWER']);
  });
});
