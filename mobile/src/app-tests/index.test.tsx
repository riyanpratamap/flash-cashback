import AsyncStorage from '@react-native-async-storage/async-storage';
import { act, fireEvent, screen, waitFor } from '@testing-library/react-native';

import { FOCUS_FRESH_MS } from '@/api/hooks';
import { USER_STORAGE_KEY } from '@/user/UserProvider';
import { ATTEMPTS_STORAGE_KEY } from '@/attempts/store';
import { AttemptNavigator } from '@/attempts/AttemptNavigator';
import {
  campaign,
  cashback,
  fetchMock,
  moneyOk,
  posts,
  renderApp,
  requestedUrls,
  resetStorage,
  serve,
  serveMoney,
} from '@/test/fixtures';
import { push, regainFocus } from '@/test/router-mock';
import Home from '@/app/index';

jest.mock('@react-native-async-storage/async-storage', () =>
  jest.requireActual('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
jest.mock('expo-router', () => jest.requireActual('@/test/router-mock'));

beforeEach(async () => {
  await resetStorage();
  push.mockReset();
  serve();
});

const loaded = () => screen.findByText('Rp15.000');

describe('Home (AC-64)', () => {
  it('shows loading and no zero while the first answers are pending', async () => {
    const release: ((response: Response) => void)[] = [];
    fetchMock.mockImplementation(() => new Promise<Response>((resolve) => release.push(resolve)));
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    await renderApp(<Home />);
    expect(await screen.findByText('Loading…')).toBeTruthy();
    expect(screen.queryByText('Rp0')).toBeNull();
    // Answer after the assertions so the client's request timer is cleared and Jest can exit.
    await act(async () => release.forEach((resolve) => resolve(new Response('{}', { status: 500 }))));
  });

  it('shows the load error with Try again and no zero values when the cashback fails', async () => {
    serve({ cashback: new Error('boom') });
    await renderApp(<Home />);
    expect(
      await screen.findByText("Couldn't load your cashback. Your balance is safe. Check your connection and try again."),
    ).toBeTruthy();
    expect(screen.queryByText(/Rp0/)).toBeNull();
    expect(screen.queryByText('Cashback balance')).toBeNull();

    serve();
    await fireEvent.press(screen.getByRole('button', { name: 'Try again' }));
    expect(await loaded()).toBeTruthy();
    expect(screen.queryByText(/Couldn't load your cashback/)).toBeNull();
  });

  it('shows the load error when the campaign fails', async () => {
    serve({ campaign: new Error('boom') });
    await renderApp(<Home />);
    expect(await screen.findByText(/Couldn't load your cashback/)).toBeTruthy();
  });

  it('shows the ACTIVE banner with the numbers of the rules', async () => {
    await renderApp(<Home />);
    await loaded();
    expect(screen.getByText('Campaign active')).toBeTruthy();
    expect(
      screen.getByText('Cashback up to 5% on payments of Rp20.000 or more, max Rp50.000 per day, while cashback lasts.'),
    ).toBeTruthy();
    await fireEvent.press(screen.getByRole('link', { name: 'How it works' }));
    expect(push).toHaveBeenCalledWith('/how-it-works');
  });

  it('shows the balance, Earned today, what is left and the reset time', async () => {
    await renderApp(<Home />);
    await loaded();
    expect(screen.getByText('Cashback balance')).toBeTruthy();
    expect(screen.getByText('Earned today')).toBeTruthy();
    expect(screen.getByText('Rp47.000 / Rp50.000')).toBeTruthy();
    expect(screen.getByText('Rp3.000 left to earn today. Resets at 00:00 WIB.')).toBeTruthy();
    expect(screen.getByRole('progressbar')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Redeem' })).toBeEnabled();
    expect(screen.queryByText(/budget/i)).toBeNull();
    expect(screen.queryByText(/ending soon/i)).toBeNull();
  });

  it('shows the limit-reached line at nothing left', async () => {
    serve({ cashback: { ...cashback, today: { ...cashback.today, earned: 50000, remaining: 0 } } });
    await renderApp(<Home />);
    expect(await screen.findByText("You've reached today's limit. Resets at 00:00 WIB.")).toBeTruthy();
  });

  it('shows the PAUSED banner, neutral, with the Earned today card kept', async () => {
    serve({ campaign: { ...campaign, status: 'PAUSED' } });
    await renderApp(<Home />);
    expect(
      await screen.findByText('Cashback is temporarily unavailable. Payments still work as usual.'),
    ).toBeTruthy();
    expect(screen.getByText('Earned today')).toBeTruthy();
  });

  it('shows the ENDED banner, removes Earned today, and keeps Redeem usable', async () => {
    serve({ campaign: { ...campaign, status: 'ENDED' } });
    await renderApp(<Home />);
    expect(
      await screen.findByText(
        'Flash Cashback has ended. All cashback has been claimed. Payments still work as usual, and you can still redeem your balance.',
      ),
    ).toBeTruthy();
    expect(screen.queryByText('Earned today')).toBeNull();
    expect(screen.getByRole('button', { name: 'Redeem' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Make a payment' })).toBeTruthy();
  });

  it('drops the redeem clause from the ENDED banner while redemptions are paused', async () => {
    serve({ campaign: { ...campaign, status: 'ENDED', redemption_status: 'PAUSED' } });
    await renderApp(<Home />);
    expect(
      await screen.findByText('Flash Cashback has ended. All cashback has been claimed. Payments still work as usual.'),
    ).toBeTruthy();
  });

  it('disables Redeem and says the balance is safe when redemptions are paused', async () => {
    serve({ campaign: { ...campaign, redemption_status: 'PAUSED' } });
    await renderApp(<Home />);
    expect(await screen.findByText('Redemption is on hold and your balance is safe.')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Redeem' })).toBeDisabled();
  });

  it('disables Redeem at balance 0 and shows Rp0 as a real zero', async () => {
    serve({ cashback: { ...cashback, balance: 0 } });
    await renderApp(<Home />);
    expect(await screen.findByText('Rp0')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Redeem' })).toBeDisabled();
  });

  it('navigates from Redeem, Make a payment, and See all', async () => {
    await renderApp(<Home />);
    await loaded();
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem' }));
    await fireEvent.press(screen.getByRole('button', { name: 'Make a payment' }));
    await fireEvent.press(screen.getByRole('link', { name: 'See all' }));
    expect(push.mock.calls).toEqual([['/redeem'], ['/pay'], ['/history']]);
  });

  it('lists the two newest activities', async () => {
    await renderApp(<Home />);
    expect(await screen.findByText('Redeemed to main account')).toBeTruthy();
    expect(screen.getByText('−Rp42.000')).toBeTruthy();
    expect(screen.getByText('Payment Rp500.000')).toBeTruthy();
    expect(screen.getByText('+Rp25.000')).toBeTruthy();
    expect(requestedUrls().some((u) => u.endsWith('/me/history?limit=2'))).toBe(true);
  });

  it('shows the empty activity copy', async () => {
    serve({ history: { items: [] } });
    await renderApp(<Home />);
    expect(await screen.findByText('No activity yet.')).toBeTruthy();
  });

  it('shows an error in the activity section only when history fails', async () => {
    serve({ history: new Error('boom') });
    await renderApp(<Home />);
    expect(await loaded()).toBeTruthy();
    expect(await screen.findByText("Couldn't load your recent activity.")).toBeTruthy();
    expect(screen.queryByText('No activity yet.')).toBeNull();
    expect(screen.queryByText(/Couldn't load your cashback/)).toBeNull();
  });

  it('refetches on focus and on pull to refresh', async () => {
    await renderApp(<Home />);
    await loaded();
    const before = fetchMock.mock.calls.length;

    // The data is fresh for FOCUS_FRESH_MS; the clock moves past it, as when the user was away.
    const now = Date.now();
    const clock = jest.spyOn(Date, 'now').mockReturnValue(now + FOCUS_FRESH_MS + 1);
    await act(async () => regainFocus());
    await waitFor(() => expect(fetchMock.mock.calls.length).toBe(before + 3));

    const after = fetchMock.mock.calls.length;
    await act(async () => screen.getByTestId('home-scroll').props.refreshControl.props.onRefresh());
    await waitFor(() => expect(fetchMock.mock.calls.length).toBe(after + 3));
    clock.mockRestore();
  });
});

describe('Home demo user switcher (AC-67)', () => {
  it('marks DEMO, reloads as the picked user, and remembers the pick', async () => {
    await renderApp(<Home />);
    await loaded();
    expect(screen.getByText('DEMO')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'User A' })).toBeSelected();

    await fireEvent.press(screen.getByRole('button', { name: 'User B' }));
    await waitFor(() => expect(fetchMock.mock.calls.at(-1)?.[1].headers['X-User-ID']).toBe('user_b'));
    await waitFor(async () => expect(await AsyncStorage.getItem(USER_STORAGE_KEY)).toBe('user_b'));
    expect(screen.getByRole('button', { name: 'User B' })).toBeSelected();
    expect(await screen.findByText('Cashback balance')).toBeTruthy();
  });
});


describe('Home unconfirmed card (AC-74)', () => {
  const OLD = (kind: 'payment' | 'redemption' = 'payment', user = 'user_b') => ({
    user_id: user,
    kind,
    amount: kind === 'payment' ? 100000 : 18000,
    key: 'OLD-KEY',
    created_at: '2026-10-03T14:32:00.000Z',
  });
  const seed = (...attempts: ReturnType<typeof OLD>[]) =>
    AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify(attempts));
  // 11 minutes after the saved time: too old to resend on its own
  const realNow = Date.now;
  beforeEach(() => {
    Date.now = () => Date.parse('2026-10-03T14:43:00.000Z');
  });
  afterEach(() => {
    Date.now = realNow;
  });

  it('shows the payment card with the amount and the time, and sends nothing', async () => {
    await seed(OLD());
    serveMoney([]);
    await renderApp(<Home />);
    expect(await screen.findByText("A payment of Rp100.000 from 3 Oct, 14:32 wasn't confirmed.")).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Check now' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Dismiss' })).toBeTruthy();
    expect(posts).toHaveLength(0);
  });

  it('says redemption on a redemption card', async () => {
    await seed(OLD('redemption'));
    serveMoney([]);
    await renderApp(<Home />);
    expect(await screen.findByText("A redemption of Rp18.000 from 3 Oct, 14:32 wasn't confirmed.")).toBeTruthy();
  });

  it('Dismiss sends nothing, removes the card and the saved attempt, and shows the history hint', async () => {
    await seed(OLD());
    serveMoney([]);
    await renderApp(<Home />);
    await fireEvent.press(await screen.findByRole('button', { name: 'Dismiss' }));
    expect(await screen.findByText('Check your history before paying again.')).toBeTruthy();
    expect(screen.queryByText(/wasn't confirmed/)).toBeNull();
    expect(posts).toHaveLength(0);
    expect(await AsyncStorage.getItem(ATTEMPTS_STORAGE_KEY)).toBe('[]');
  });

  it('shows no hint before a Dismiss', async () => {
    await seed(OLD());
    serveMoney([]);
    await renderApp(<Home />);
    await screen.findByText(/wasn't confirmed/);
    expect(screen.queryByText('Check your history before paying again.')).toBeNull();
  });

  it('Check now resends the same key as the saved user and Checking opens', async () => {
    await seed(OLD());
    serveMoney([moneyOk()]);
    await renderApp(
      <>
        <AttemptNavigator />
        <Home />
      </>,
    );
    await fireEvent.press(await screen.findByRole('button', { name: 'Check now' }));
    await waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]).toMatchObject({ key: 'OLD-KEY', user: 'user_b', body: { amount: 100000 } });
    expect(push).toHaveBeenCalledWith('/checking');
    expect(screen.queryByText(/wasn't confirmed/)).toBeNull();
  });
});
