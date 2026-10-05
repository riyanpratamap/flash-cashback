import { useEffect, useState } from 'react';

import { useAttempts } from '@/attempts/AttemptProvider';
import type { AttemptKind } from '@/attempts/types';
import { formatAsTyped, parseDigits } from '@/money/format';

/**
 * The amount form Pay and Redeem share: the field text, the rejection a launch resend left for this kind, the
 * rejection that matches the typed amount, and the acknowledgement when the screen is left.
 */
export function useAmountForm(kind: AttemptKind) {
  const { state, acknowledge } = useAttempts();
  const rejectedAmount = state.phase === 'rejected' && state.attempt.kind === kind ? state.attempt.amount : null;
  // A rejection found at launch lands here with the amount it was for.
  const [text, setTextRaw] = useState(() => (rejectedAmount === null ? '' : formatAsTyped(String(rejectedAmount))));

  // A rejection that arrives while the screen is open (a launch resend) fills in the amount it was for. Keyed on the
  // state object, adjusted during render: it acts once per new state and never overwrites what is typed afterwards.
  const [seen, setSeen] = useState(state);
  if (seen !== state) {
    setSeen(state);
    if (rejectedAmount !== null) setTextRaw(formatAsTyped(String(rejectedAmount)));
  }

  // Leaving the screen ends the answer it showed: a launch resend waiting behind it can carry on.
  useEffect(
    () => () => {
      if (kind === 'redemption') acknowledge('done');
      acknowledge('rejected');
    },
    [acknowledge, kind],
  );

  const amount = parseDigits(text);
  const inFlight = state.phase === 'saving' || state.phase === 'sending';
  const rejection =
    state.phase === 'rejected' && state.attempt.kind === kind && state.attempt.amount === amount ? state : null;

  return {
    text,
    setText: (next: string) => setTextRaw(formatAsTyped(next)),
    amount,
    inFlight,
    rejection,
  };
}
