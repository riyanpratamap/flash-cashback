import AsyncStorage from '@react-native-async-storage/async-storage';
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react-native';
import { Pressable, Text } from 'react-native';

import { cashbackQuery, queryKeys } from '@/api/queries';
import { USER_STORAGE_KEY, UserProvider, useUser } from '@/user/UserProvider';

jest.mock('@react-native-async-storage/async-storage', () =>
  jest.requireActual('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);

const fetchMock = jest.fn<Promise<Response>, [string, RequestInit & { headers: Record<string, string> }]>();

beforeEach(async () => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } });
  await AsyncStorage.clear();
  fetchMock.mockReset();
  fetchMock.mockImplementation(async () =>
    new Response(
      JSON.stringify({
        balance: 1,
        today: { date: '2026-10-03', earned: 0, remaining: 50000, resets_at: '2026-10-04T00:00:00+07:00' },
      }),
      { status: 200 },
    ),
  );
  globalThis.fetch = fetchMock as unknown as typeof fetch;
});

function Probe() {
  const { user, setUser } = useUser();
  const query = useQuery({ queryKey: queryKeys.cashback(user), queryFn: () => cashbackQuery(user) });
  return (
    <>
      <Text>current: {user}</Text>
      <Text>{query.isSuccess ? 'loaded' : 'loading'}</Text>
      <Pressable accessibilityRole="button" onPress={() => void setUser('user_b')}>
        <Text>pick b</Text>
      </Pressable>
    </>
  );
}

// gcTime Infinity: no garbage-collection timer is left pending after a test, so Jest can exit.
let client = new QueryClient();
function App() {
  return (
    <QueryClientProvider client={client}>
      <UserProvider>
        <Probe />
      </UserProvider>
    </QueryClientProvider>
  );
}

describe('UserProvider (AC-67)', () => {
  it('starts as user_a when nothing is stored', async () => {
    await render(<App />);
    expect(await screen.findByText('current: user_a')).toBeTruthy();
  });

  it('renders nothing until the stored user is read', async () => {
    let release: (value: string | null) => void = () => undefined;
    jest.spyOn(AsyncStorage, 'getItem').mockReturnValueOnce(new Promise((resolve) => (release = resolve)));
    await render(<App />);
    expect(screen.queryByText(/current:/)).toBeNull();
    release('user_c');
    expect(await screen.findByText('current: user_c')).toBeTruthy();
  });

  it('every request carries the selected user as X-User-ID', async () => {
    await render(<App />);
    await screen.findByText('loaded');
    expect(fetchMock.mock.calls.at(-1)?.[1].headers['X-User-ID']).toBe('user_a');

    await fireEvent.press(screen.getByRole('button', { name: 'pick b' }));
    await screen.findByText('current: user_b');
    await waitFor(() => expect(fetchMock.mock.calls.at(-1)?.[1].headers['X-User-ID']).toBe('user_b'));
  });

  it('keeps the chosen user after a remount over the same storage', async () => {
    const first = await render(<App />);
    await fireEvent.press(await screen.findByRole('button', { name: 'pick b' }));
    await waitFor(async () => expect(await AsyncStorage.getItem(USER_STORAGE_KEY)).toBe('user_b'));
    await first.unmount();

    await render(<App />);
    expect(await screen.findByText('current: user_b')).toBeTruthy();
  });

  it('ignores a stored value that is not a demo user', async () => {
    await AsyncStorage.setItem(USER_STORAGE_KEY, 'mallory');
    await render(<App />);
    expect(await screen.findByText('current: user_a')).toBeTruthy();
  });
});
