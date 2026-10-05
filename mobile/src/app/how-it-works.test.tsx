import { fireEvent, screen } from '@testing-library/react-native';

import { campaign, renderScreen, resetStorage, serve } from '../test/fixtures';
import HowItWorks from './how-it-works';

jest.mock('@react-native-async-storage/async-storage', () =>
  jest.requireActual('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
jest.mock('expo-router', () => jest.requireActual('../test/router-mock'));

beforeEach(async () => {
  await resetStorage();
  serve();
});

describe('How Flash Cashback works (AC-75)', () => {
  it('shows the rate, minimum, daily cap, and reset time of the rules', async () => {
    await renderScreen(<HowItWorks />);
    expect(await screen.findByText('Up to 5% cashback')).toBeTruthy();
    expect(
      screen.getByText("On every payment of Rp20.000 or more. Payments under Rp20.000 don't earn cashback."),
    ).toBeTruthy();
    expect(screen.getByText('Up to Rp50.000 per day')).toBeTruthy();
    expect(
      screen.getByText(
        'The limit resets at 00:00 WIB. A payment that reaches the limit earns what is left of it, so it can earn less than 5%.',
      ),
    ).toBeTruthy();
    expect(screen.getByText('Pay Rp100.000')).toBeTruthy();
    expect(screen.getByText('earn Rp5.000')).toBeTruthy();
    expect(screen.getByText('While cashback lasts')).toBeTruthy();
    expect(screen.getByText('Redeem anytime')).toBeTruthy();
    expect(screen.getByText('Rounded down')).toBeTruthy();
  });

  it('shows 10%, Rp30.000 and Rp70.000 when the server serves those rules, with no app change', async () => {
    serve({
      campaign: { ...campaign, rules: { rate_bps: 1000, min_payment: 30000, daily_cap: 70000, timezone: 'Asia/Jakarta' } },
    });
    await renderScreen(<HowItWorks />);
    expect(await screen.findByText('Up to 10% cashback')).toBeTruthy();
    expect(
      screen.getByText("On every payment of Rp30.000 or more. Payments under Rp30.000 don't earn cashback."),
    ).toBeTruthy();
    expect(screen.getByText('Up to Rp70.000 per day')).toBeTruthy();
    expect(screen.getByText(/so it can earn less than 10%\.$/)).toBeTruthy();
    expect(screen.getByText('earn Rp10.000')).toBeTruthy();
    expect(screen.queryByText(/5%/)).toBeNull();
  });

  it('shows an error with Try again, never default numbers, when the campaign fails', async () => {
    serve({ campaign: new Error('boom') });
    await renderScreen(<HowItWorks />);
    expect(await screen.findByText("Couldn't load the campaign rules.")).toBeTruthy();
    expect(screen.queryByText(/Rp\d/)).toBeNull();

    serve();
    await fireEvent.press(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('Up to 5% cashback')).toBeTruthy();
  });
});
