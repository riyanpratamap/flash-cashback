import AsyncStorage from '@react-native-async-storage/async-storage';

import type { SavedAttempt } from './types';

export const ATTEMPTS_STORAGE_KEY = 'fc:attempts';
/** Keys the server rejected (4xx) whose removal failed. The launch check skips them: a 4xx created no row. */
export const RESOLVED_STORAGE_KEY = 'fc:attempts:resolved';

function isSavedAttempt(value: unknown): value is SavedAttempt {
  if (typeof value !== 'object' || value === null) return false;
  const v = value as Record<string, unknown>;
  return (
    typeof v.user_id === 'string' &&
    (v.kind === 'payment' || v.kind === 'redemption') &&
    typeof v.amount === 'number' &&
    Number.isInteger(v.amount) &&
    typeof v.key === 'string' &&
    typeof v.created_at === 'string'
  );
}

function parseAttempts(raw: string | null): SavedAttempt[] {
  if (raw === null) return [];
  try {
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter(isSavedAttempt) : [];
  } catch {
    return []; // unreadable content holds no attempt we could safely resend
  }
}

async function loadResolved(): Promise<string[]> {
  const raw = await AsyncStorage.getItem(RESOLVED_STORAGE_KEY);
  if (raw === null) return [];
  try {
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((k): k is string => typeof k === 'string') : [];
  } catch {
    return [];
  }
}

/**
 * Reads the saved attempts, without the ones marked resolved. A storage read error is thrown: the caller decides what
 * an unreadable store means.
 */
export async function loadAttempts(): Promise<SavedAttempt[]> {
  const attempts = parseAttempts(await AsyncStorage.getItem(ATTEMPTS_STORAGE_KEY));
  const resolved = await loadResolved();
  return resolved.length === 0 ? attempts : attempts.filter((a) => !resolved.includes(a.key));
}

// Read-modify-write on one key: writes run one after another so two overlapping calls cannot lose an update.
let queue: Promise<unknown> = Promise.resolve();
function serialised<T>(task: () => Promise<T>): Promise<T> {
  const run = queue.then(task, task);
  queue = run.catch(() => undefined);
  return run;
}

async function write(attempts: readonly SavedAttempt[]): Promise<void> {
  await AsyncStorage.setItem(ATTEMPTS_STORAGE_KEY, JSON.stringify(attempts));
}

/** Adds the attempt, or replaces the one with the same key; never touches another key. Throws on a storage error. */
export function saveAttempt(attempt: SavedAttempt): Promise<void> {
  return serialised(async () => {
    const others = (await loadAttempts()).filter((a) => a.key !== attempt.key);
    await write([...others, attempt]);
  });
}

export function removeAttempt(key: string): Promise<void> {
  return serialised(async () => {
    const all = await loadAttempts();
    const rest = all.filter((a) => a.key !== key);
    if (rest.length !== all.length) await write(rest);
  });
}

/** Marks the key as answered for good, for when `removeAttempt` keeps failing. Throws on a storage error. */
export function markResolved(key: string): Promise<void> {
  return serialised(async () => {
    const resolved = await loadResolved();
    if (!resolved.includes(key)) await AsyncStorage.setItem(RESOLVED_STORAGE_KEY, JSON.stringify([...resolved, key]));
  });
}
