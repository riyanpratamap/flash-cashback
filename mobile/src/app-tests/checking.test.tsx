import { act, fireEvent, screen } from '@testing-library/react-native';
import { randomUUID } from 'expo-crypto';
import { BackHandler, Pressable, Text } from 'react-native';

import { useAttempts } from '@/attempts/AttemptProvider';
import { moneyOk, moneyRejected, posts, renderApp, resetStorage, serveMoney, type MoneyAnswer } from '@/test/fixtures';
import Checking from '@/app/checking';

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
  const a = useAttempts();
  return (
    <>
      <Pressable accessibilityRole="button" onPress={() => a.press('payment', 100000)}>
        <Text>pay</Text>
      </Pressable>
      <Pressable accessibilityRole="button" onPress={() => a.press('redemption', 18000)}>
        <Text>redeem</Text>
      </Pressable>
      <Text>phase: {a.state.phase}</Text>
      <Checking />
    </>
  );
}

async function start(answers: MoneyAnswer[], button = 'pay') {
  serveMoney(answers);
  await renderApp(<Harness />);
  await settle(0);
  await fireEvent.press(screen.getByRole('button', { name: button }));
  await settle(0);
}

/** The listener the screen registered last, the one Android would call first. */
function lastBackListener(spy: jest.SpyInstance): () => boolean {
  const call = spy.mock.calls.at(-1) as [string, () => boolean] | undefined;
  if (call === undefined) throw new Error('no back listener registered');
  return call[1];
}

let backSpy: jest.SpyInstance;

beforeEach(async () => {
  jest.useFakeTimers({ now: NOW });
  await resetStorage();
  let n = 0;
  (randomUUID as jest.Mock).mockImplementation(() => `K${++n}`);
  backSpy = jest.spyOn(BackHandler, 'addEventListener');
});

afterEach(() => {
  backSpy.mockRestore();
  jest.useRealTimers();
});

describe('Checking a payment (AC-60)', () => {
  it('shows the payment copy with the spinner while resending the same key', async () => {
    await start(['network', 'network', 'network', 'network']);
    expect(screen.getByText('Checking your payment...')).toBeTruthy();
    expect(screen.getByText('Rp100.000')).toBeTruthy();
    expect(
      screen.getByText(
        "This is taking longer than usual. Please don't pay again. This screen updates as soon as we have the result.",
      ),
    ).toBeTruthy();
    expect(screen.getByTestId('checking-spinner')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Check again' })).toBeNull();
    await settle(2000);
    await settle(2000);
    expect(posts.map((p) => p.key)).toEqual(['K1', 'K1', 'K1']);
  });

  it('after three resends the spinner stops and Check again resends the same key', async () => {
    await start(['network', 'network', 'network', 'network']);
    await settle(6000);
    expect(posts).toHaveLength(4);
    expect(screen.getByText('phase: waiting')).toBeTruthy();
    expect(screen.queryByTestId('checking-spinner')).toBeNull();
    expect(screen.queryByText(/failed/i)).toBeNull();
    expect(screen.getByText('Checking your payment...')).toBeTruthy();
    await fireEvent.press(screen.getByRole('button', { name: 'Check again' }));
    await settle(0);
    expect(posts).toHaveLength(5);
    expect(new Set(posts.map((p) => p.key))).toEqual(new Set(['K1']));
  });

  it('never says "failed", in any phase of an unknown outcome', async () => {
    await start(['hang', 'network', 'network', 'network']);
    expect(screen.queryByText(/failed/i)).toBeNull();
    await settle(10_000);
    expect(screen.queryByText(/failed/i)).toBeNull();
    await settle(6000);
    expect(screen.getByText('phase: waiting')).toBeTruthy();
    expect(screen.queryByText(/failed/i)).toBeNull();
  });

  it('a 2xx while checking leaves the screen blank (the navigator opens the result)', async () => {
    await start(['network', moneyOk()]);
    await settle(2000);
    expect(screen.getByText('phase: done')).toBeTruthy();
    expect(screen.queryByText('Checking your payment...')).toBeNull();
  });
});

describe('Checking a redemption', () => {
  it('shows the redemption copy', async () => {
    await start(['network', 'network', 'network', 'network'], 'redeem');
    expect(screen.getByText('Checking your redemption...')).toBeTruthy();
    expect(screen.getByText('Rp18.000')).toBeTruthy();
    expect(
      screen.getByText(
        "This is taking longer than usual. Please don't redeem again. This screen updates as soon as we have the result.",
      ),
    ).toBeTruthy();
    await settle(6000);
    expect(posts.map((p) => p.key)).toEqual(['K1', 'K1', 'K1', 'K1']);
    expect(posts.every((p) => p.url.endsWith('/redemptions'))).toBe(true);
  });
});

describe('Checking blocks leaving (AC-60)', () => {
  it('the Android back button is swallowed while checking and while waiting', async () => {
    await start(['network', 'network', 'network', 'network']);
    expect(lastBackListener(backSpy)()).toBe(true);
    await settle(6000);
    expect(screen.getByText('phase: waiting')).toBeTruthy();
    expect(lastBackListener(backSpy)()).toBe(true);
  });

  it('lets back through once the answer is definite', async () => {
    await start(['network', moneyRejected(422, 'INVALID_AMOUNT')]);
    await settle(2000);
    expect(screen.getByText('phase: rejected')).toBeTruthy();
    expect(lastBackListener(backSpy)()).toBe(false);
  });

  it('removes its listener when it unmounts', async () => {
    const remove = jest.fn();
    backSpy.mockReturnValue({ remove });
    serveMoney([]);
    const view = await renderApp(<Harness />);
    await settle(0);
    await view.unmount();
    expect(remove).toHaveBeenCalled();
  });
});
