import { render, waitFor } from '@testing-library/react-native';
import type { ReactNode } from 'react';

import RootLayout, * as layout from '../app/_layout';

jest.mock('@react-native-async-storage/async-storage', () =>
  jest.requireActual('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);
jest.mock('expo-crypto', () => ({ randomUUID: jest.fn() }));

const mockScreens: Record<string, unknown> = {};
let mockStackOptions: unknown;
jest.mock('expo-router', () => {
  function Screen({ name, options }: { name: string; options: unknown }) {
    mockScreens[name] = options;
    return null;
  }
  function Stack({ children, screenOptions }: { children: ReactNode; screenOptions?: unknown }) {
    mockStackOptions = screenOptions;
    return children;
  }
  Stack.Screen = Screen;
  return { ...jest.requireActual('../test/router-mock'), Stack };
});

beforeEach(() => {
  Object.keys(mockScreens).forEach((name) => delete mockScreens[name]);
  mockStackOptions = undefined;
});

describe('root layout screen options (AC-60)', () => {
  it('Checking has no header and no swipe-back, so the user cannot leave it', async () => {
    await render(<RootLayout />);
    await waitFor(() => expect(mockScreens['checking']).toBeDefined()); // the providers load the user first
    expect(mockScreens['checking']).toEqual({ headerShown: false, gestureEnabled: false });
  });

  it('the Payment result cannot be swiped away either', async () => {
    await render(<RootLayout />);
    await waitFor(() => expect(mockScreens['payment-result']).toBeDefined());
    expect(mockScreens['payment-result']).toMatchObject({ headerShown: false, gestureEnabled: false });
  });

  it('Redeem has a header titled Redeem cashback', async () => {
    await render(<RootLayout />);
    await waitFor(() => expect(mockScreens['redeem']).toBeDefined());
    expect(mockScreens['redeem']).toEqual({ title: 'Redeem cashback' });
  });

  it('every back button shows the arrow only, without the previous title', async () => {
    await render(<RootLayout />);
    await waitFor(() => expect(mockScreens['redeem']).toBeDefined());
    expect(mockStackOptions).toMatchObject({ headerBackButtonDisplayMode: 'minimal' });
  });

  it('a screen opened directly (deep link) still has Home beneath it, so back always works', () => {
    expect(layout.unstable_settings).toEqual({ initialRouteName: 'index' });
  });
});
