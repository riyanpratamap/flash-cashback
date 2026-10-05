import AsyncStorage from '@react-native-async-storage/async-storage';

import { ATTEMPTS_STORAGE_KEY, loadAttempts, removeAttempt, saveAttempt } from '@/attempts/store';
import type { SavedAttempt } from '@/attempts/types';

jest.mock('@react-native-async-storage/async-storage', () =>
  jest.requireActual('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);

const make = (key: string, extra: Partial<SavedAttempt> = {}): SavedAttempt => ({
  user_id: 'user_a',
  kind: 'payment',
  amount: 100000,
  key,
  created_at: '2026-10-03T10:00:00.000Z',
  ...extra,
});

beforeEach(async () => {
  await AsyncStorage.clear();
});

describe('saved attempts store (D48)', () => {
  it('loads an empty list when nothing is saved', async () => {
    expect(await loadAttempts()).toEqual([]);
  });

  it('saves attempts as a list keyed by key and never overwrites another', async () => {
    await saveAttempt(make('K1'));
    await saveAttempt(make('K2', { kind: 'redemption', amount: 18000 }));
    expect((await loadAttempts()).map((a) => a.key)).toEqual(['K1', 'K2']);
    expect(JSON.parse((await AsyncStorage.getItem(ATTEMPTS_STORAGE_KEY)) ?? '[]')).toHaveLength(2);
  });

  it('upserts the same key instead of duplicating it', async () => {
    await saveAttempt(make('K1'));
    await saveAttempt(make('K1', { amount: 30000 }));
    expect(await loadAttempts()).toEqual([make('K1', { amount: 30000 })]);
  });

  it('removes only the given key', async () => {
    await saveAttempt(make('K1'));
    await saveAttempt(make('K2'));
    await removeAttempt('K1');
    expect((await loadAttempts()).map((a) => a.key)).toEqual(['K2']);
  });

  it('keeps every write when saves overlap', async () => {
    await Promise.all([saveAttempt(make('K1')), saveAttempt(make('K2')), removeAttempt('K0'), saveAttempt(make('K3'))]);
    expect((await loadAttempts()).map((a) => a.key).sort()).toEqual(['K1', 'K2', 'K3']);
  });

  it('drops entries of the wrong shape and survives unparseable storage', async () => {
    await AsyncStorage.setItem(
      ATTEMPTS_STORAGE_KEY,
      JSON.stringify([make('ok'), { key: 'bad' }, make('float', { amount: 1.5 }), make('kind', { kind: 'x' as never })]),
    );
    expect((await loadAttempts()).map((a) => a.key)).toEqual(['ok']);
    await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, '{not json');
    expect(await loadAttempts()).toEqual([]);
  });

  it('surfaces a storage error on save', async () => {
    jest.spyOn(AsyncStorage, 'setItem').mockRejectedValueOnce(new Error('disk full'));
    await expect(saveAttempt(make('K1'))).rejects.toThrow('disk full');
  });

  it('keeps working after a failed save', async () => {
    jest.spyOn(AsyncStorage, 'setItem').mockRejectedValueOnce(new Error('disk full'));
    await expect(saveAttempt(make('K1'))).rejects.toThrow();
    await saveAttempt(make('K2'));
    expect((await loadAttempts()).map((a) => a.key)).toEqual(['K2']);
  });
});
