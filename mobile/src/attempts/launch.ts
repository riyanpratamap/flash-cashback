import type { SavedAttempt } from './types';

/** An attempt younger than this is resent at launch; an older one waits for the user's choice (D48). */
export const RECENT_WINDOW_MS = 10 * 60 * 1000;

export type LaunchSplit = { recent: readonly SavedAttempt[]; unconfirmed: readonly SavedAttempt[] };

function byAge(a: SavedAttempt, b: SavedAttempt): number {
  return Date.parse(a.created_at) - Date.parse(b.created_at);
}

/**
 * Splits saved attempts by age, oldest first in each group. A negative age (the clock moved back) counts as recent;
 * an unreadable time counts as old, so nothing is resent on a guess.
 */
export function splitByAge(attempts: readonly SavedAttempt[], nowMs: number): LaunchSplit {
  const recent: SavedAttempt[] = [];
  const unconfirmed: SavedAttempt[] = [];
  for (const attempt of attempts) {
    const created = Date.parse(attempt.created_at);
    if (Number.isNaN(created) || nowMs - created >= RECENT_WINDOW_MS) unconfirmed.push(attempt);
    else recent.push(attempt);
  }
  return { recent: recent.sort(byAge), unconfirmed: unconfirmed.sort(byAge) };
}
