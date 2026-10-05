import { useRouter } from 'expo-router';
import { useEffect, useRef } from 'react';

import { useAttempts } from './AttemptProvider';
import type { AttemptState } from './machine';

/**
 * Opens the screen each attempt state calls for (tech-spec §10), wherever the user is: a launch resend reaches Checking
 * from Home too. Renders nothing. Must sit inside AttemptProvider.
 */
export function AttemptNavigator() {
  const router = useRouter();
  const { state, acknowledge } = useAttempts();
  const handled = useRef<AttemptState | null>(null);
  const previous = useRef<AttemptState['phase']>('idle');

  useEffect(() => {
    if (handled.current === state) return; // the router object may change between renders; each state is acted on once
    handled.current = state;
    const fromChecking = previous.current === 'checking' || previous.current === 'waiting';
    previous.current = state.phase;

    if (state.phase === 'checking') {
      if (!fromChecking) router.push('/checking');
      return;
    }
    if (state.phase !== 'done' && state.phase !== 'rejected') return;

    if (state.attempt.kind === 'payment') {
      if (state.phase === 'done') router.replace('/payment-result');
      else if (fromChecking) router.dismissTo('/pay'); // back on the originating screen, which shows the error
      return;
    }
    // Assumption: until P5.5 adds the Redeem screens, a redemption answer returns Home and is acknowledged.
    router.dismissTo('/');
    acknowledge();
  }, [state, router, acknowledge]);

  return null;
}
