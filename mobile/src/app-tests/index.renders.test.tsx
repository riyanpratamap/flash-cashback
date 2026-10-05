import { act, fireEvent, screen } from '@testing-library/react-native';
import { randomUUID } from 'expo-crypto';
import { Pressable, Text } from 'react-native';

import { useAttemptActions, useAttemptLaunch, useAttemptState } from '@/attempts/AttemptProvider';
import { posts, renderApp, resetStorage, serveMoney, type MoneyAnswer } from '@/test/fixtures';
import Home from '@/app/index';
import { useCampaign } from '@/api/hooks';

jest.mock('@react-native-async-storage/async-storage', () =>
  jest.requireActual('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
jest.mock('expo-router', () => jest.requireActual('@/test/router-mock'));
jest.mock('expo-crypto', () => ({ randomUUID: jest.fn() }));
// Home calls `useCampaign` once per render, so its call count is its render count (a Profiler reports only the mount
// under this renderer).
jest.mock('@/api/hooks', () => {
  const actual = jest.requireActual('@/api/hooks');
  return { ...actual, useCampaign: jest.fn(actual.useCampaign) };
});

const settle = (ms: number) =>
  act(async () => {
    await jest.advanceTimersByTimeAsync(ms);
  });

function Driver() {
  const a = { state: useAttemptState(), ...useAttemptLaunch(), ...useAttemptActions() };
  return (
    <>
      <Text>phase: {a.state.phase}</Text>
      <Pressable accessibilityRole="button" onPress={() => a.press('payment', 100000)}>
        <Text>pay</Text>
      </Pressable>
      <Pressable accessibilityRole="button" onPress={() => a.acknowledge()}>
        <Text>acknowledge</Text>
      </Pressable>
    </>
  );
}

describe('Home renders during an attempt (C4)', () => {
  beforeEach(async () => {
    jest.useFakeTimers({ now: Date.parse('2026-10-03T10:00:00.000Z') });
    await resetStorage();
    (randomUUID as jest.Mock).mockReturnValue('K1');
  });
  afterEach(() => {
    jest.useRealTimers();
  });

  const homeRenders = () => jest.mocked(useCampaign).mock.calls.length;

  async function mountHome(answers: MoneyAnswer[]) {
    serveMoney(answers);
    await renderApp(
      <>
        <Home />
        <Driver />
      </>,
    );
    await settle(0);
    await screen.findByText('Rp15.000');
    await settle(0);
  }

  it('does not render Home while a payment goes from saving to sending', async () => {
    await mountHome(['hang']);
    const before = homeRenders();

    await fireEvent.press(screen.getByRole('button', { name: 'pay' }));
    await settle(0);
    expect(screen.getByText('phase: sending')).toBeTruthy();

    expect(homeRenders() - before).toBe(0);
    await settle(10_000); // the request times out; no timer is left pending
  });

  it('renders Home once at most for a whole payment: the refetch after done, not the phases', async () => {
    await mountHome([]);
    const before = homeRenders();

    await fireEvent.press(screen.getByRole('button', { name: 'pay' }));
    await settle(0);
    expect(screen.getByText('phase: done')).toBeTruthy();
    expect(posts).toHaveLength(1);

    expect(homeRenders() - before).toBeLessThanOrEqual(1);
    await settle(100); // the invalidated reads finish inside act
  });
});
