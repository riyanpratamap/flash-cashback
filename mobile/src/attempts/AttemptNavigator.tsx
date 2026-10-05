import { useRouter } from 'expo-router';
import { useEffect, useRef } from 'react';

import { useAttempts } from '@/attempts/AttemptProvider';
import type { AttemptState } from '@/attempts/machine';

/**
 * Opens the screen each attempt state calls for (tech-spec §10), wherever the user is: a launch resend reaches Checking
 * from Home too. Renders nothing. Must sit inside AttemptProvider.
 */
export function AttemptNavigator() {
  const router = useRouter();
  const { state } = useAttempts();
  const handled = useRef<AttemptState | null>(null);
  const previous = useRef<AttemptState['phase']>('idle');
  /** True when Checking was opened by a press on a money screen, which is then still open beneath it. */
  const pressed = useRef(false);

  useEffect(() => {
    if (handled.current === state) return; // the router object may change between renders; each state is acted on once
    handled.current = state;
    const fromChecking = previous.current === 'checking' || previous.current === 'waiting';
    const wasSending = previous.current === 'sending' || previous.current === 'saving';
    previous.current = state.phase;

    if (state.phase === 'checking') {
      if (!fromChecking) {
        pressed.current = wasSending;
        router.push('/checking');
      }
      return;
    }
    if (state.phase !== 'done' && state.phase !== 'rejected') return;

    if (state.attempt.kind === 'payment') {
      if (state.phase === 'done') router.replace('/payment-result');
      else if (fromChecking) router.dismissTo('/pay'); // back on the originating screen, which shows the error
      return;
    }
    // A redemption answer is shown on Redeem and acknowledged when the user leaves it (Done or back). Redeem is
    // already open after a press; after a launch resend only Checking is open, so Redeem takes its place.
    if (!fromChecking) return;
    if (pressed.current) router.dismissTo('/redeem');
    else router.replace('/redeem');
  }, [state, router]);

  return null;
}
