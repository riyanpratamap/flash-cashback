import AsyncStorage from '@react-native-async-storage/async-storage';
import { QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen } from '@testing-library/react-native';
import { randomUUID } from 'expo-crypto';
import { useEffect, useState } from 'react';
import { Pressable, Text } from 'react-native';

import { newClient } from '@/test/fixtures';
import { UserProvider, useUser } from '@/user/UserProvider';
import { AttemptProvider, useAttemptActions, useAttemptLaunch, useAttemptState } from '@/attempts/AttemptProvider';
import { ATTEMPTS_STORAGE_KEY, RESOLVED_STORAGE_KEY } from '@/attempts/store';
import type { SavedAttempt } from '@/attempts/types';

jest.mock('@react-native-async-storage/async-storage', () =>
  jest.requireActual('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
jest.mock('expo-crypto', () => ({ randomUUID: jest.fn() }));

type Call = { url: string; method: string; user: string; key: string | undefined; body: unknown; saved: SavedAttempt[] };
type Answer = { status: number; body: unknown } | 'network' | 'hang';

const NOW = Date.parse('2026-10-03T10:00:00.000Z');
const PAID = { payment: { id: 1, amount: 100000 }, cashback: { awarded: 5000, reason: 'AWARDED' } };
const REDEEMED = { redemption: { id: 3, amount: 18000 }, balance_after: 0 };
const ok = (body: unknown): Answer => ({ status: 201, body });
const rejected = (status: number, code: string): Answer => ({
  status,
  body: { error: { code, message: 'm', request_id: 'r' } },
});

let calls: Call[];
let answers: Answer[];
let keyCounter: number;
let client: ReturnType<typeof newClient>;
let onFetch: (() => void) | undefined;

/** Stubs fetch: records each call and what `fc:attempts` held at that moment, then plays the next answer. */
function installFetch() {
  globalThis.fetch = (async (url: string, init: RequestInit & { headers: Record<string, string> }) => {
    onFetch?.();
    const raw = await AsyncStorage.getItem(ATTEMPTS_STORAGE_KEY);
    calls.push({
      url,
      method: init.method ?? 'GET',
      user: init.headers['X-User-ID'] ?? '',
      key: init.headers['Idempotency-Key'],
      body: init.body === undefined ? undefined : JSON.parse(String(init.body)),
      saved: raw === null ? [] : (JSON.parse(raw) as SavedAttempt[]),
    });
    const answer = answers.shift() ?? ok(PAID);
    if (answer === 'network') throw new TypeError('network');
    if (answer === 'hang') {
      return new Promise<Response>((_resolve, reject) => {
        init.signal?.addEventListener('abort', () => reject(new Error('aborted')));
      });
    }
    return new Response(JSON.stringify(answer.body), { status: answer.status });
  }) as unknown as typeof fetch;
}

function Probe() {
  const a = { state: useAttemptState(), ...useAttemptLaunch(), ...useAttemptActions() };
  const { user, setUser } = useUser();
  const { press } = a;
  const [payOnSwitch, setPayOnSwitch] = useState(false);
  // A child's passive effect runs before the provider's own passive effects, in the commit that shows the new user.
  useEffect(() => {
    if (payOnSwitch && user === 'user_b') press('payment', 100000);
  }, [payOnSwitch, user, press]);
  return (
    <>
      <Pressable accessibilityRole="button" onPress={() => void setUser('user_b')}>
        <Text>switch to b</Text>
      </Pressable>
      <Pressable
        accessibilityRole="button"
        onPress={() => {
          setPayOnSwitch(true);
          void setUser('user_b');
        }}
      >
        <Text>switch to b and pay</Text>
      </Pressable>
      <Text>phase: {a.state.phase}</Text>
      <Text>selected: {user}</Text>
      <Text>launch: {a.launchChecked ? 'checked' : 'pending'}</Text>
      <Text>unconfirmed: {a.unconfirmed.map((u) => u.key).join(',')}</Text>
      <Text>code: {a.state.phase === 'rejected' ? a.state.code : '-'}</Text>
      <Pressable accessibilityRole="button" onPress={() => a.press('payment', 100000)}>
        <Text>pay</Text>
      </Pressable>
      <Pressable
        accessibilityRole="button"
        onPress={() => {
          a.press('payment', 100000);
          a.press('payment', 100000);
        }}
      >
        <Text>pay twice</Text>
      </Pressable>
      <Pressable accessibilityRole="button" onPress={() => a.press('redemption', 18000)}>
        <Text>redeem</Text>
      </Pressable>
      <Pressable accessibilityRole="button" onPress={() => a.acknowledge()}>
        <Text>acknowledge</Text>
      </Pressable>
      <Pressable accessibilityRole="button" onPress={() => a.acknowledge('rejected')}>
        <Text>acknowledge rejected</Text>
      </Pressable>
      <Pressable accessibilityRole="button" onPress={a.checkAgain}>
        <Text>check again</Text>
      </Pressable>
      <Pressable accessibilityRole="button" onPress={() => a.checkNow('OLD')}>
        <Text>check now</Text>
      </Pressable>
      <Pressable accessibilityRole="button" onPress={() => void a.dismiss('OLD')}>
        <Text>dismiss</Text>
      </Pressable>
    </>
  );
}

async function mount() {
  return render(
    <QueryClientProvider client={client}>
      <UserProvider>
        <AttemptProvider>
          <Probe />
        </AttemptProvider>
      </UserProvider>
    </QueryClientProvider>,
  );
}

const press = (label: string) => fireEvent.press(screen.getByRole('button', { name: label }));
const settle = (ms: number) => act(async () => { await jest.advanceTimersByTimeAsync(ms); });
const phase = (p: string) => screen.getByText(`phase: ${p}`);
const savedKeys = async () =>
  (JSON.parse((await AsyncStorage.getItem(ATTEMPTS_STORAGE_KEY)) ?? '[]') as SavedAttempt[]).map((a) => a.key);

function attemptAged(key: string, minutes: number, user = 'user_a'): SavedAttempt {
  return {
    user_id: user,
    kind: 'payment',
    amount: 100000,
    key,
    created_at: new Date(NOW - minutes * 60_000).toISOString(),
  };
}

beforeEach(async () => {
  jest.useFakeTimers({ now: NOW });
  await AsyncStorage.clear();
  calls = [];
  answers = [];
  onFetch = undefined;
  keyCounter = 0;
  (randomUUID as jest.Mock).mockImplementation(() => `K${++keyCounter}`);
  client = newClient();
  installFetch();
});

afterEach(() => {
  jest.useRealTimers();
});

describe('AC-59 double press', () => {
  it('sends exactly one request when Pay is pressed twice in the same frame', async () => {
    await mount();
    await press('pay twice');
    await settle(0);
    expect(calls).toHaveLength(1);
    expect(keyCounter).toBe(1);
    expect(phase('done')).toBeTruthy();
  });
});

describe('D48 attempt user', () => {
  it('carries the user selected at press time: the request and the saved attempt', async () => {
    await mount();
    await press('switch to b');
    await settle(0);
    await press('pay');
    await settle(0);
    expect(calls).toHaveLength(1);
    expect(calls[0]?.user).toBe('user_b');
    expect(calls[0]?.saved.map((a) => a.user_id)).toEqual(['user_b']);
  });

  it('carries the new user when the press happens in the commit that shows it', async () => {
    await mount();
    await press('switch to b and pay');
    await settle(0);
    expect(calls).toHaveLength(1);
    expect(calls[0]?.user).toBe('user_b');
    expect(calls[0]?.saved.map((a) => a.user_id)).toEqual(['user_b']);
  });
});

describe('AC-60 unknown outcome', () => {
  it('resends the same key three times about 2 s apart, then waits; Check again resends the same key', async () => {
    answers = ['hang', 'network', 'network', 'network'];
    await mount();
    await press('pay');
    await settle(0);
    expect(calls).toHaveLength(1);
    await settle(10_000); // the 10 s abort: unknown, never failed
    expect(phase('checking')).toBeTruthy();
    expect(calls).toHaveLength(1);
    await settle(1999);
    expect(calls).toHaveLength(1);
    await settle(1);
    expect(calls).toHaveLength(2);
    await settle(2000);
    expect(calls).toHaveLength(3);
    await settle(2000);
    expect(calls).toHaveLength(4);
    expect(phase('waiting')).toBeTruthy();
    await settle(20_000);
    expect(calls).toHaveLength(4);
    expect(new Set(calls.map((c) => c.key))).toEqual(new Set(['K1']));
    expect(await savedKeys()).toEqual(['K1']);

    answers = [ok(PAID)];
    await press('check again');
    await settle(0);
    expect(calls).toHaveLength(5);
    expect(calls[4]?.key).toBe('K1');
    expect(phase('done')).toBeTruthy();
    expect(await savedKeys()).toEqual([]);
  });

  it('a 5xx on the first send moves to checking and a later 2xx is done', async () => {
    answers = [{ status: 503, body: {} }, ok(PAID)];
    await mount();
    await press('pay');
    await settle(0);
    expect(phase('checking')).toBeTruthy();
    await settle(2000);
    expect(phase('done')).toBeTruthy();
    expect(calls.map((c) => c.key)).toEqual(['K1', 'K1']);
  });

  it('does the same for a redemption', async () => {
    answers = ['network', ok(REDEEMED)];
    await mount();
    await press('redeem');
    await settle(0);
    await settle(2000);
    expect(phase('done')).toBeTruthy();
    expect(calls.map((c) => [c.url.endsWith('/redemptions'), c.key, c.body])).toEqual([
      [true, 'K1', { amount: 18000 }],
      [true, 'K1', { amount: 18000 }],
    ]);
  });
});

describe('AC-61 rejection', () => {
  it('a 4xx is rejected, removes the attempt, and the next press uses a new key', async () => {
    answers = [rejected(422, 'INVALID_AMOUNT'), ok(PAID)];
    await mount();
    await press('pay');
    await settle(0);
    expect(phase('rejected')).toBeTruthy();
    expect(screen.getByText('code: INVALID_AMOUNT')).toBeTruthy();
    expect(await savedKeys()).toEqual([]);
    await press('pay');
    await settle(0);
    expect(calls.map((c) => c.key)).toEqual(['K1', 'K2']);
    expect(phase('done')).toBeTruthy();
  });

  it('a 4xx while checking is rejected', async () => {
    answers = ['network', rejected(409, 'REDEMPTION_PAUSED')];
    await mount();
    await press('redeem');
    await settle(2000);
    expect(phase('rejected')).toBeTruthy();
    expect(await savedKeys()).toEqual([]);
  });
});

describe('AC-72 saved before send', () => {
  it('finds the attempt in storage when fetch is called, and removes it on 2xx', async () => {
    await mount();
    await press('pay');
    await settle(0);
    expect(calls[0]?.saved).toEqual([
      { user_id: 'user_a', kind: 'payment', amount: 100000, key: 'K1', created_at: new Date(NOW).toISOString() },
    ]);
    expect(await savedKeys()).toEqual([]);
  });

  it('saves a redemption the same way', async () => {
    answers = [ok(REDEEMED)];
    await mount();
    await press('redeem');
    await settle(0);
    expect(calls[0]?.saved.map((a) => [a.kind, a.amount, a.key])).toEqual([['redemption', 18000, 'K1']]);
  });

  it('keeps the attempt on 5xx and on a network error, removes it on 4xx', async () => {
    answers = [{ status: 500, body: {} }, 'network', 'network', 'network'];
    await mount();
    await press('pay');
    await settle(6000);
    expect(phase('waiting')).toBeTruthy();
    expect(await savedKeys()).toEqual(['K1']);

    answers = [rejected(422, 'INSUFFICIENT_BALANCE')];
    await press('check again');
    await settle(0);
    expect(await savedKeys()).toEqual([]);
  });

  it('does not overwrite an unresolved attempt with a new one', async () => {
    await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify([attemptAged('OLD', 30)]));
    await mount();
    await settle(0);
    await press('pay');
    await settle(0);
    expect(calls[0]?.saved.map((a) => a.key)).toEqual(['OLD', 'K1']);
    expect(await savedKeys()).toEqual(['OLD']);
  });

  it('sends nothing and shows the generic error when the attempt cannot be saved', async () => {
    jest.spyOn(AsyncStorage, 'setItem').mockRejectedValueOnce(new Error('disk full'));
    await mount();
    await press('pay');
    await settle(0);
    expect(calls).toHaveLength(0);
    expect(phase('rejected')).toBeTruthy();
    expect(screen.getByText('code: LOCAL_ERROR')).toBeTruthy();
  });
});

describe('after an answer', () => {
  it('sends the attempt user on every resend', async () => {
    answers = ['network', ok(PAID)];
    await mount();
    await press('pay');
    await settle(2000);
    expect(calls.map((c) => c.user)).toEqual(['user_a', 'user_a']);
  });

  it('invalidates campaign, cashback, and history of the attempt user on 2xx', async () => {
    const invalidate = jest.spyOn(client, 'invalidateQueries');
    await mount();
    await press('pay');
    await settle(0);
    expect(invalidate.mock.calls.map(([f]) => f?.queryKey)).toEqual(
      expect.arrayContaining([['campaign'], ['cashback', 'user_a'], ['history', 'user_a']]),
    );
  });

  it('refetches cashback on INSUFFICIENT_BALANCE and campaign on REDEMPTION_PAUSED', async () => {
    const refetch = jest.spyOn(client, 'refetchQueries');
    answers = [rejected(422, 'INSUFFICIENT_BALANCE'), rejected(409, 'REDEMPTION_PAUSED')];
    await mount();
    await press('redeem');
    await settle(0);
    expect(refetch.mock.calls.map(([f]) => f?.queryKey)).toEqual([['cashback', 'user_a']]);
    await press('redeem');
    await settle(0);
    expect(refetch.mock.calls.map(([f]) => f?.queryKey)).toEqual([['cashback', 'user_a'], ['campaign']]);
  });
});

describe('AC-73 launch check, recent attempt', () => {
  it('resends the saved key, user, and amount; 2xx clears it; the selected user is untouched', async () => {
    await AsyncStorage.setItem('fc:user', 'user_b');
    await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify([attemptAged('K', 9)]));
    answers = ['network', ok(PAID)];
    await mount();
    await settle(0);
    expect(phase('checking')).toBeTruthy(); // Checking opens at launch
    expect(calls).toHaveLength(1);
    await settle(2000);
    expect(calls).toHaveLength(2);
    expect(calls.map((c) => c.key)).toEqual(['K', 'K']);
    expect(calls[0]).toMatchObject({ key: 'K', user: 'user_a', body: { amount: 100000 }, method: 'POST' });
    expect(calls[0]?.url.endsWith('/payments')).toBe(true);
    expect(phase('done')).toBeTruthy();
    expect(screen.getByText('selected: user_b')).toBeTruthy();
    expect(screen.getByText('launch: checked')).toBeTruthy();
    expect(await savedKeys()).toEqual([]);
  });

  it('resends several recent attempts one at a time, oldest first', async () => {
    await AsyncStorage.setItem(
      ATTEMPTS_STORAGE_KEY,
      JSON.stringify([attemptAged('NEWER', 2), attemptAged('OLDER', 8)]),
    );
    answers = ['hang'];
    await mount();
    await settle(0);
    expect(calls.map((c) => c.key)).toEqual(['OLDER']); // the second waits for the first
    await settle(10_000);
    await settle(2000);
    expect(calls.map((c) => c.key)).toEqual(['OLDER', 'OLDER']);
    await press('acknowledge'); // F5: the first result is seen before the second is sent
    await settle(0);
    expect(calls.map((c) => c.key)).toEqual(['OLDER', 'OLDER', 'NEWER']);
  });
});

describe('AC-74 launch check, old attempt', () => {
  beforeEach(async () => {
    await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify([attemptAged('OLD', 11)]));
  });

  it('sends nothing and exposes the attempt as unconfirmed', async () => {
    await mount();
    await settle(0);
    expect(calls).toHaveLength(0);
    expect(screen.getByText('unconfirmed: OLD')).toBeTruthy();
    expect(screen.getByText('launch: checked')).toBeTruthy();
    expect(phase('idle')).toBeTruthy();
  });

  it('Check now resends the saved key and user', async () => {
    await AsyncStorage.setItem('fc:user', 'user_b');
    await mount();
    await settle(0);
    await press('check now');
    await settle(0);
    expect(calls).toHaveLength(1);
    expect(calls[0]).toMatchObject({ key: 'OLD', user: 'user_a', body: { amount: 100000 } });
    expect(phase('done')).toBeTruthy();
    expect(screen.getByText('unconfirmed: ')).toBeTruthy();
    expect(await savedKeys()).toEqual([]);
  });

  it('Dismiss removes the attempt and sends nothing', async () => {
    await mount();
    await settle(0);
    await press('dismiss');
    await settle(0);
    expect(calls).toHaveLength(0);
    expect(screen.getByText('unconfirmed: ')).toBeTruthy();
    expect(await savedKeys()).toEqual([]);
  });
});

describe('guards and storage failures', () => {
  it('AC-61: when removal fails twice after a 4xx, the key is marked resolved', async () => {
    const setItem = jest.spyOn(AsyncStorage, 'setItem');
    answers = [rejected(422, 'INVALID_AMOUNT')];
    onFetch = () => {
      // The save has landed; the removal and its retry both fail.
      setItem.mockRejectedValueOnce(new Error('disk')).mockRejectedValueOnce(new Error('disk'));
    };
    await mount();
    await settle(0);
    await press('pay');
    await settle(0);
    expect(phase('rejected')).toBeTruthy();
    expect(await savedKeys()).toEqual(['K1']);
    expect(JSON.parse((await AsyncStorage.getItem(RESOLVED_STORAGE_KEY)) ?? '[]')).toEqual(['K1']);
  });

  it('AC-61: a relaunch within 10 minutes never resends a key marked resolved', async () => {
    await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify([attemptAged('K1', 1)]));
    await AsyncStorage.setItem(RESOLVED_STORAGE_KEY, JSON.stringify(['K1']));
    await mount();
    await settle(0);
    expect(calls).toHaveLength(0);
    expect(screen.getByText('launch: checked')).toBeTruthy();
    expect(screen.getByText('unconfirmed: ')).toBeTruthy();
    expect(phase('idle')).toBeTruthy();
  });

  it('a failed removal after a 2xx is swallowed: the attempt is still done', async () => {
    const setItem = jest.spyOn(AsyncStorage, 'setItem');
    onFetch = () => {
      setItem.mockRejectedValueOnce(new Error('disk'));
    };
    await mount();
    await settle(0);
    await press('pay');
    await settle(0);
    expect(phase('done')).toBeTruthy();
    expect(await savedKeys()).toEqual(['K1']); // the server replays K1 if it is resent
  });

  it('an unreadable store at launch counts as no attempts and sends nothing', async () => {
    await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify([attemptAged('K', 5)]));
    const getItem = jest.spyOn(AsyncStorage, 'getItem');
    const read = async (key: string) => (await AsyncStorage.multiGet([key]))[0]?.[1] ?? null;
    getItem.mockImplementation(async (key) => {
      if (key === ATTEMPTS_STORAGE_KEY) throw new Error('unreadable');
      return read(key);
    });
    try {
      await mount();
      await settle(0);
      expect(calls).toHaveLength(0);
      expect(phase('idle')).toBeTruthy();
      expect(screen.getByText('launch: checked')).toBeTruthy();
    } finally {
      getItem.mockImplementation(read); // the spy lives in the shared AsyncStorage mock: leave it working
    }
  });

  it('a save error removes the key best-effort, in case the write landed', async () => {
    const setItem = jest.spyOn(AsyncStorage, 'setItem');
    // The write persists, then reports an error.
    setItem.mockImplementationOnce(async (key, value) => {
      await AsyncStorage.multiSet([[key, value]]);
      throw new Error('late failure');
    });
    await mount();
    await settle(0);
    await press('pay');
    await settle(0);
    expect(calls).toHaveLength(0);
    expect(screen.getByText('code: LOCAL_ERROR')).toBeTruthy();
    expect(await savedKeys()).toEqual([]);
  });

  it('ignores a press while waiting', async () => {
    answers = ['network', 'network', 'network', 'network'];
    await mount();
    await settle(0);
    await press('pay');
    await settle(6000);
    expect(phase('waiting')).toBeTruthy();
    expect(calls).toHaveLength(4);
    await press('pay');
    await settle(0);
    expect(calls).toHaveLength(4);
    expect(keyCounter).toBe(1);
    expect(phase('waiting')).toBeTruthy();
  });

  it('ignores a press and Check now while the launch check holds the guard', async () => {
    await AsyncStorage.setItem(
      ATTEMPTS_STORAGE_KEY,
      JSON.stringify([attemptAged('OLD', 11), attemptAged('RECENT', 5)]),
    );
    answers = ['hang'];
    await mount();
    await settle(0);
    expect(calls.map((c) => c.key)).toEqual(['RECENT']);
    await press('pay');
    await press('check now');
    await settle(0);
    expect(calls.map((c) => c.key)).toEqual(['RECENT']);
    expect(keyCounter).toBe(0);
  });

  it('a launch resend that ends in waiting turns the rest into cards, all calls on the first key', async () => {
    await AsyncStorage.setItem(
      ATTEMPTS_STORAGE_KEY,
      JSON.stringify([attemptAged('NEWER', 2), attemptAged('OLDER', 8)]),
    );
    answers = ['network', 'network', 'network'];
    await mount();
    await settle(4000);
    expect(phase('waiting')).toBeTruthy();
    expect(calls.map((c) => c.key)).toEqual(['OLDER', 'OLDER', 'OLDER']);
    expect(screen.getByText('unconfirmed: NEWER')).toBeTruthy();
    expect(await savedKeys()).toEqual(expect.arrayContaining(['OLDER', 'NEWER']));
  });

  it('Dismiss only removes a card, not the attempt in flight', async () => {
    await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify([attemptAged('OLD', 5)]));
    answers = ['hang'];
    await mount();
    await settle(0);
    expect(phase('checking')).toBeTruthy();
    await press('dismiss');
    await settle(0);
    expect(await savedKeys()).toEqual(['OLD']);
  });
});

describe('P5.3 review F5: each launch result is shown before the next resend', () => {
  const twoRecent = () =>
    AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify([attemptAged('NEWER', 2), attemptAged('OLDER', 8)]));

  it('holds a done result until it is acknowledged, then resends the next', async () => {
    await twoRecent();
    answers = [ok(PAID), ok(PAID)];
    await mount();
    await settle(0);
    await settle(5000);
    expect(phase('done')).toBeTruthy();
    expect(calls.map((c) => c.key)).toEqual(['OLDER']);
    await press('acknowledge');
    await settle(0);
    expect(calls.map((c) => c.key)).toEqual(['OLDER', 'NEWER']);
    expect(phase('done')).toBeTruthy();
  });

  it('holds a rejected result the same way', async () => {
    await twoRecent();
    answers = [rejected(422, 'INVALID_AMOUNT'), ok(PAID)];
    await mount();
    await settle(5000);
    expect(screen.getByText('code: INVALID_AMOUNT')).toBeTruthy();
    expect(calls.map((c) => c.key)).toEqual(['OLDER']);
    await press('acknowledge');
    await settle(0);
    expect(calls.map((c) => c.key)).toEqual(['OLDER', 'NEWER']);
  });

  it('acknowledging resets the state to idle; the typed variant only acts on its own phase', async () => {
    answers = [ok(PAID)];
    await mount();
    await press('pay');
    await settle(0);
    expect(phase('done')).toBeTruthy();
    await press('acknowledge rejected'); // not rejected: no effect
    expect(phase('done')).toBeTruthy();
    await press('acknowledge');
    expect(phase('idle')).toBeTruthy();
  });

  it('the last attempt needs no acknowledgement and releases the guard', async () => {
    await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify([attemptAged('ONLY', 2)]));
    answers = [ok(PAID), ok(PAID)];
    await mount();
    await settle(0);
    expect(phase('done')).toBeTruthy();
    await press('pay');
    await settle(0);
    expect(calls.map((c) => c.key)).toEqual(['ONLY', 'K1']);
  });

  it('F2: a press while the launch queue waits runs with a new key, and the rest become cards, never resent', async () => {
    await twoRecent();
    answers = [rejected(422, 'INVALID_AMOUNT'), ok(PAID), ok(PAID)];
    await mount();
    await settle(5000);
    expect(screen.getByText('code: INVALID_AMOUNT')).toBeTruthy();
    expect(calls.map((c) => c.key)).toEqual(['OLDER']);
    await press('pay');
    await settle(0);
    await settle(20_000);
    expect(calls.map((c) => c.key)).toEqual(['OLDER', 'K1']); // NEWER is not resent behind the press
    expect(calls[1]).toMatchObject({ url: expect.stringMatching(/\/payments$/), body: { amount: 100000 } });
    expect(screen.getByText('unconfirmed: NEWER')).toBeTruthy();
    expect(await savedKeys()).toEqual(['NEWER']); // original key kept
    expect(phase('done')).toBeTruthy();
    await press('acknowledge'); // a late acknowledgement does not wake a queue that is gone
    await settle(0);
    expect(calls.map((c) => c.key)).toEqual(['OLDER', 'K1']);
  });
});
