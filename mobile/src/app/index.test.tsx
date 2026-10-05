import AsyncStorage from '@react-native-async-storage/async-storage';
import { act, fireEvent, screen, waitFor } from '@testing-library/react-native';

import { USER_STORAGE_KEY } from '../user/UserProvider';
import { campaign, cashback, fetchMock, renderScreen, requestedUrls, resetStorage, serve } from '../test/fixtures';
import { push, regainFocus } from '../test/router-mock';
import Home from './index';

jest.mock('@react-native-async-storage/async-storage', () =>
  jest.requireActual('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
jest.mock('expo-router', () => jest.requireActual('../test/router-mock'));

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
    await renderScreen(<Home />);
    expect(await screen.findByText('Loading…')).toBeTruthy();
    expect(screen.queryByText('Rp0')).toBeNull();
    // Answer after the assertions so the client's request timer is cleared and Jest can exit.
    await act(async () => release.forEach((resolve) => resolve(new Response('{}', { status: 500 }))));
  });

  it('shows the load error with Try again and no zero values when the cashback fails', async () => {
    serve({ cashback: new Error('boom') });
    await renderScreen(<Home />);
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
    await renderScreen(<Home />);
    expect(await screen.findByText(/Couldn't load your cashback/)).toBeTruthy();
  });

  it('shows the ACTIVE banner with the numbers of the rules', async () => {
    await renderScreen(<Home />);
    await loaded();
    expect(screen.getByText('Campaign active')).toBeTruthy();
    expect(
      screen.getByText('Cashback up to 5% on payments of Rp20.000 or more, max Rp50.000 per day, while cashback lasts.'),
    ).toBeTruthy();
    await fireEvent.press(screen.getByRole('link', { name: 'How it works' }));
    expect(push).toHaveBeenCalledWith('/how-it-works');
  });

  it('shows the balance, Earned today, what is left and the reset time', async () => {
    await renderScreen(<Home />);
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
    await renderScreen(<Home />);
    expect(await screen.findByText("You've reached today's limit. Resets at 00:00 WIB.")).toBeTruthy();
  });

  it('shows the PAUSED banner, neutral, with the Earned today card kept', async () => {
    serve({ campaign: { ...campaign, status: 'PAUSED' } });
    await renderScreen(<Home />);
    expect(
      await screen.findByText('Cashback is temporarily unavailable. Payments still work as usual.'),
    ).toBeTruthy();
    expect(screen.getByText('Earned today')).toBeTruthy();
  });

  it('shows the ENDED banner, removes Earned today, and keeps Redeem usable', async () => {
    serve({ campaign: { ...campaign, status: 'ENDED' } });
    await renderScreen(<Home />);
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
    await renderScreen(<Home />);
    expect(
      await screen.findByText('Flash Cashback has ended. All cashback has been claimed. Payments still work as usual.'),
    ).toBeTruthy();
  });

  it('disables Redeem and says the balance is safe when redemptions are paused', async () => {
    serve({ campaign: { ...campaign, redemption_status: 'PAUSED' } });
    await renderScreen(<Home />);
    expect(await screen.findByText('Redemption is on hold and your balance is safe.')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Redeem' })).toBeDisabled();
  });

  it('disables Redeem at balance 0 and shows Rp0 as a real zero', async () => {
    serve({ cashback: { ...cashback, balance: 0 } });
    await renderScreen(<Home />);
    expect(await screen.findByText('Rp0')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Redeem' })).toBeDisabled();
  });

  it('navigates from Redeem, Make a payment, and See all', async () => {
    await renderScreen(<Home />);
    await loaded();
    await fireEvent.press(screen.getByRole('button', { name: 'Redeem' }));
    await fireEvent.press(screen.getByRole('button', { name: 'Make a payment' }));
    await fireEvent.press(screen.getByRole('link', { name: 'See all' }));
    expect(push.mock.calls).toEqual([['/redeem'], ['/pay'], ['/history']]);
  });

  it('lists the two newest activities', async () => {
    await renderScreen(<Home />);
    expect(await screen.findByText('Redeemed to main account')).toBeTruthy();
    expect(screen.getByText('−Rp42.000')).toBeTruthy();
    expect(screen.getByText('Payment Rp500.000')).toBeTruthy();
    expect(screen.getByText('+Rp25.000')).toBeTruthy();
    expect(requestedUrls().some((u) => u.endsWith('/me/history?limit=2'))).toBe(true);
  });

  it('shows the empty activity copy', async () => {
    serve({ history: { items: [] } });
    await renderScreen(<Home />);
    expect(await screen.findByText('No activity yet.')).toBeTruthy();
  });

  it('shows an error in the activity section only when history fails', async () => {
    serve({ history: new Error('boom') });
    await renderScreen(<Home />);
    expect(await loaded()).toBeTruthy();
    expect(await screen.findByText("Couldn't load your recent activity.")).toBeTruthy();
    expect(screen.queryByText('No activity yet.')).toBeNull();
    expect(screen.queryByText(/Couldn't load your cashback/)).toBeNull();
  });

  it('refetches on focus and on pull to refresh', async () => {
    await renderScreen(<Home />);
    await loaded();
    const before = fetchMock.mock.calls.length;

    await act(async () => regainFocus());
    await waitFor(() => expect(fetchMock.mock.calls.length).toBe(before + 3));

    const after = fetchMock.mock.calls.length;
    await act(async () => screen.getByTestId('home-scroll').props.refreshControl.props.onRefresh());
    await waitFor(() => expect(fetchMock.mock.calls.length).toBe(after + 3));
  });
});

describe('Home demo user switcher (AC-67)', () => {
  it('marks DEMO, reloads as the picked user, and remembers the pick', async () => {
    await renderScreen(<Home />);
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

