import type { AttemptState } from './machine';

/** While the outcome is unknown, or a request is on its way, the user must not leave Checking (AC-60). */
export function blocksLeaving(phase: AttemptState['phase']): boolean {
  return phase === 'sending' || phase === 'checking' || phase === 'waiting';
}

/** Checking has no header and no swipe-back: the saved key is not lost and the user is not led to pay again. */
export const CHECKING_SCREEN_OPTIONS = { headerShown: false, gestureEnabled: false } as const;
