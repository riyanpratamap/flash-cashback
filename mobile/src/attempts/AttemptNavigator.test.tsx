import AsyncStorage from '@react-native-async-storage/async-storage';
import { act, fireEvent, screen } from '@testing-library/react-native';
import { randomUUID } from 'expo-crypto';
import { Pressable, Text } from 'react-native';

import {
  moneyOk,
  moneyRejected,
  posts,
  renderApp,
  resetStorage,
  serveMoney,
} from '@/test/fixtures';
import { back, dismissTo, push, replace, resetRouter } from '@/test/router-mock';
import { AttemptNavigator } from '@/attempts/AttemptNavigator';
import { useAttempts } from '@/attempts/AttemptProvider';
import { ATTEMPTS_STORAGE_KEY } from '@/attempts/store';

jest.mock('@react-native-async-storage/async-storage', () =>
  jest.requireActual('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
jest.mock('expo-router', () => jest.requireActual('@/test/router-mock'));
jest.mock('expo-crypto', () => ({ randomUUID: jest.fn() }));

const NOW = Date.parse('2026-10-03T10:00:00.000Z');

function Buttons() {
  const a = useAttempts();
  return (
    <>
      <Text>phase: {a.state.phase}</Text>
      <Pressable accessibilityRole="button" onPress={() => a.press('payment', 100000)}>
        <Text>pay</Text>
      </Pressable>
      <Pressable accessibilityRole="button" onPress={() => a.press('redemption', 18000)}>
        <Text>redeem</Text>
      </Pressable>
    </>
  );
}

const mount = () =>
  renderApp(
    <>
      <AttemptNavigator />
      <Buttons />
    </>,
  );
const press = (label: string) => fireEvent.press(screen.getByRole('button', { name: label }));
const settle = (ms: number) =>
  act(async () => {
    await jest.advanceTimersByTimeAsync(ms);
  });

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

describe('AttemptNavigator: the attempt state drives the screens', () => {
  it('a 2xx from Pay replaces it with the Payment result', async () => {
    serveMoney([moneyOk()]);
    await mount();
    await settle(0);
    await press('pay');
    await settle(0);
    expect(replace).toHaveBeenCalledWith('/payment-result');
    expect(push).not.toHaveBeenCalled();
  });

  it('an unknown outcome opens Checking once, and a later 2xx replaces it with the result', async () => {
    serveMoney(['network', 'network', moneyOk()]);
    await mount();
    await settle(0);
    await press('pay');
    await settle(0);
    expect(push).toHaveBeenCalledTimes(1);
    expect(push).toHaveBeenCalledWith('/checking');
    expect(replace).not.toHaveBeenCalled();
    await settle(4000);
    expect(push).toHaveBeenCalledTimes(1);
    expect(replace).toHaveBeenCalledWith('/payment-result');
  });

  it('waiting stays on Checking', async () => {
    serveMoney(['network', 'network', 'network', 'network']);
    await mount();
    await settle(0);
    await press('pay');
    await settle(6000);
    expect(screen.getByText('phase: waiting')).toBeTruthy();
    expect(push).toHaveBeenCalledTimes(1);
    expect(replace).not.toHaveBeenCalled();
    expect(dismissTo).not.toHaveBeenCalled();
  });

  it('a 4xx on Pay stays on Pay: no navigation', async () => {
    serveMoney([moneyRejected(422, 'INVALID_AMOUNT')]);
    await mount();
    await settle(0);
    await press('pay');
    await settle(0);
    expect(screen.getByText('phase: rejected')).toBeTruthy();
    expect([push, replace, dismissTo, back].map((fn) => fn.mock.calls.length)).toEqual([0, 0, 0, 0]);
  });

  it('a 4xx found while checking returns to Pay', async () => {
    serveMoney(['network', moneyRejected(422, 'INVALID_AMOUNT')]);
    await mount();
    await settle(0);
    await press('pay');
    await settle(2000);
    expect(push).toHaveBeenCalledWith('/checking');
    expect(dismissTo).toHaveBeenCalledWith('/pay');
  });

  it('a launch resend of a recent attempt opens Checking, then the result (AC-73)', async () => {
    await AsyncStorage.setItem(
      ATTEMPTS_STORAGE_KEY,
      JSON.stringify([
        { user_id: 'user_a', kind: 'payment', amount: 100000, key: 'K', created_at: new Date(NOW - 60_000).toISOString() },
      ]),
    );
    serveMoney(['network', moneyOk()]);
    await mount();
    await settle(0);
    expect(push).toHaveBeenCalledWith('/checking');
    expect(posts).toHaveLength(1);
    await settle(2000);
    expect(replace).toHaveBeenCalledWith('/payment-result');
  });

  it('a redemption result found while checking opens Redeem and stays unacknowledged until the user leaves', async () => {
    serveMoney(['network', moneyOk({ redemption: { id: 3, reference: 'R', amount: 18000 }, balance_after: 0 })]);
    await mount();
    await settle(0);
    await press('redeem');
    await settle(2000);
    expect(dismissTo).toHaveBeenCalledWith('/redeem');
    expect(dismissTo).not.toHaveBeenCalledWith('/');
    expect(replace).not.toHaveBeenCalled();
    expect(screen.getByText('phase: done')).toBeTruthy();
  });

  it('a redemption rejection found while checking opens Redeem and stays unacknowledged', async () => {
    serveMoney(['network', moneyRejected(422, 'INSUFFICIENT_BALANCE')]);
    await mount();
    await settle(0);
    await press('redeem');
    await settle(2000);
    expect(dismissTo).toHaveBeenCalledWith('/redeem');
    expect(dismissTo).not.toHaveBeenCalledWith('/');
    expect(screen.getByText('phase: rejected')).toBeTruthy();
  });

  it('a redemption answered straight to Redeem needs no navigation', async () => {
    serveMoney([moneyOk({ redemption: { id: 3, reference: 'R', amount: 18000 }, balance_after: 0 })]);
    await mount();
    await settle(0);
    await press('redeem');
    await settle(0);
    expect([push, replace, dismissTo, back].map((fn) => fn.mock.calls.length)).toEqual([0, 0, 0, 0]);
    expect(screen.getByText('phase: done')).toBeTruthy();
  });

  it('a launch-resent redemption opens Redeem with its result (P5.5)', async () => {
    await AsyncStorage.setItem(
      ATTEMPTS_STORAGE_KEY,
      JSON.stringify([
        { user_id: 'user_a', kind: 'redemption', amount: 18000, key: 'K', created_at: new Date(NOW - 60_000).toISOString() },
      ]),
    );
    serveMoney(['network', moneyOk({ redemption: { id: 3, reference: 'R', amount: 18000 }, balance_after: 0 })]);
    await mount();
    await settle(2000);
    expect(push).toHaveBeenCalledWith('/checking');
    expect(replace).toHaveBeenCalledWith('/redeem');
    expect(dismissTo).not.toHaveBeenCalled();
    expect(screen.getByText('phase: done')).toBeTruthy();
  });
});
