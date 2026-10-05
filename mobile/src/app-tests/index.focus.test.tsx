import { act, fireEvent, screen } from '@testing-library/react-native';
import { randomUUID } from 'expo-crypto';
import { type ComponentType, useState } from 'react';
import { Pressable, Text } from 'react-native';

import { FOCUS_FRESH_MS } from '@/api/hooks';
import { useAttemptActions, useAttemptLaunch, useAttemptState } from '@/attempts/AttemptProvider';
import { fetchMock, newClient, renderApp, requestedUrls, resetStorage, serve, serveMoney } from '@/test/fixtures';
import { regainFocus } from '@/test/router-mock';
import Home from '@/app/index';

jest.mock('@react-native-async-storage/async-storage', () =>
  jest.requireActual('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
jest.mock('expo-router', () => jest.requireActual('@/test/router-mock'));
jest.mock('expo-crypto', () => ({ randomUUID: jest.fn() }));

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
    </>
  );
}

const cashbackGets = () => requestedUrls().filter((url) => url.includes('/me/cashback')).length;
const allGets = () => fetchMock.mock.calls.filter(([, init]) => init.method !== 'POST').length;

beforeEach(async () => {
  jest.useFakeTimers({ now: Date.parse('2026-10-03T10:00:00.000Z') });
  await resetStorage();
  (randomUUID as jest.Mock).mockReturnValue('K1');
});
afterEach(() => {
  jest.useRealTimers();
});

/** Holds every later `/me/cashback` answer until the returned function is called. */
function holdCashback() {
  const answer = fetchMock.getMockImplementation()!;
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  fetchMock.mockImplementation(async (url, init) => {
    if (url.includes('/me/cashback')) await gate;
    return answer(url, init);
  });
  return release;
}

// A new prop re-renders Home from outside, as a parent state change would (a prop-less element could be cached).
const HomeWithTick: ComponentType<{ tick: number }> = Home;

function Parent() {
  const [n, setN] = useState(0);
  return (
    <>
      <HomeWithTick tick={n} />
      <Pressable accessibilityRole="button" onPress={() => setN(n + 1)}>
        <Text>bump {n}</Text>
      </Pressable>
    </>
  );
}

async function mountHome(client = newClient()) {
  await renderApp(
    <>
      <Home />
      <Driver />
    </>,
    client,
  );
  await settle(0);
  await screen.findByText('Rp15.000');
  await settle(0);
}

describe('Home focus refetch skips fresh queries (C5)', () => {
  it('sends no second balance GET when Home regains focus right after a done payment', async () => {
    serveMoney([]);
    await mountHome();
    await fireEvent.press(screen.getByRole('button', { name: 'pay' }));
    await settle(0);
    expect(screen.getByText('phase: done')).toBeTruthy();
    const afterRefetch = cashbackGets();
    expect(afterRefetch).toBe(2); // mount and the invalidation after the payment

    await act(async () => regainFocus()); // Done on the result returns Home to focus
    await settle(0);

    expect(cashbackGets()).toBe(afterRefetch);
  });

  it('refetches on focus once the last update is older than the freshness window', async () => {
    serve();
    await mountHome();
    const before = allGets();

    await settle(FOCUS_FRESH_MS + 1);
    await act(async () => regainFocus());
    await settle(0);

    expect(allGets()).toBe(before + 3);
  });

  it('pull to refresh refetches all three even when they are fresh', async () => {
    serve();
    await mountHome();
    const before = allGets();

    await act(async () => screen.getByTestId('home-scroll').props.refreshControl.props.onRefresh());
    await settle(0);

    expect(allGets()).toBe(before + 3);
  });

  it('refetches a query in error on focus', async () => {
    serve({ cashback: new Error('down') });
    await renderApp(<Home />);
    await settle(0);
    await screen.findByText(/Couldn't load your cashback/);
    const before = cashbackGets();

    await act(async () => regainFocus());
    await settle(0);

    expect(cashbackGets()).toBe(before + 1);
  });

  it('refetches on focus after a refetch that ended with the same data while Home rendered mid-fetch (F1)', async () => {
    serve();
    const client = newClient();
    await renderApp(<Parent />, client);
    await settle(0);
    await screen.findByText('Rp15.000');
    await settle(0);

    const release = holdCashback();
    void client.invalidateQueries({ queryKey: ['cashback'] });
    await settle(0);
    await fireEvent.press(screen.getByRole('button', { name: 'bump 0' })); // Home renders while the fetch is in flight
    release(); // the same data: nothing changes on screen, so no render follows
    await settle(FOCUS_FRESH_MS + 1);
    const before = cashbackGets();

    await act(async () => regainFocus());
    await settle(0);

    expect(cashbackGets()).toBe(before + 1);
  });

  it('sends no second balance GET on focus while one is in flight, however old the data (F4)', async () => {
    serve();
    const client = newClient();
    await mountHome(client);
    await settle(FOCUS_FRESH_MS + 1);
    const release = holdCashback();
    void client.invalidateQueries({ queryKey: ['cashback'] });
    await settle(0);
    const before = cashbackGets();

    await act(async () => regainFocus());
    await settle(0);

    expect(cashbackGets()).toBe(before);
    release();
    await settle(0);
  });
});
