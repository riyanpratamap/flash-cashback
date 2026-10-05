import { useQueryClient, type QueryClient } from '@tanstack/react-query';
import { randomUUID } from 'expo-crypto';
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';

import { queryKeys } from '../api/queries';
import { useUser } from '../user/UserProvider';
import { splitByAge } from './launch';
import { check, runPress, sendAttempt, type AttemptState, type MachineDeps } from './machine';
import { loadAttempts, markResolved, removeAttempt, saveAttempt } from './store';
import type { AttemptKind, SavedAttempt } from './types';

export type AttemptContextValue = {
  /** The current attempt. `idle` until a press or a launch resend. */
  state: AttemptState;
  /** True once the launch check has sorted the saved attempts (tech-spec §10). */
  launchChecked: boolean;
  /** Saved attempts too old to resend on their own: one card each on Home. */
  unconfirmed: readonly SavedAttempt[];
  press: (kind: AttemptKind, amount: number) => void;
  /** From `waiting`: resends the same key. */
  checkAgain: () => void;
  checkNow: (key: string) => void;
  /** Removes the saved attempt and sends nothing. */
  dismiss: (key: string) => Promise<void>;
  /**
   * The user has seen a definite answer (Done on the result, or leaving the rejection): the state goes back to
   * `idle` and a launch resend waiting on it carries on. With `only`, it acts when the state is in that phase.
   */
  acknowledge: (only?: 'done' | 'rejected') => void;
};

const AttemptContext = createContext<AttemptContextValue | null>(null);

const IDLE: AttemptState = { phase: 'idle' };
const wait = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

/** What a definite answer changes. The attempt's own user owns the data, not the user selected now. */
async function refreshAfter(final: AttemptState, client: QueryClient): Promise<void> {
  if (final.phase === 'done') {
    const user = final.attempt.user_id;
    void client.invalidateQueries({ queryKey: queryKeys.campaign() });
    void client.invalidateQueries({ queryKey: ['cashback', user] });
    void client.invalidateQueries({ queryKey: ['history', user] });
  } else if (final.phase === 'rejected' && final.code === 'INSUFFICIENT_BALANCE') {
    await client.refetchQueries({ queryKey: queryKeys.cashback(final.attempt.user_id) });
  } else if (final.phase === 'rejected' && final.code === 'REDEMPTION_PAUSED') {
    await client.refetchQueries({ queryKey: queryKeys.campaign() });
  }
}

/**
 * Owns the money attempt (tech-spec §10). The current attempt lives in a ref so a re-render or a refetch cannot touch
 * it (KP); state mirrors it for the screens. Must sit inside UserProvider and QueryClientProvider.
 */
export function AttemptProvider({ children }: { children: ReactNode }) {
  const client = useQueryClient();
  const { user } = useUser();
  const [state, setState] = useState<AttemptState>(IDLE);
  const [launchChecked, setLaunchChecked] = useState(false);
  const [unconfirmed, setUnconfirmed] = useState<readonly SavedAttempt[]>([]);
  const stateRef = useRef<AttemptState>(IDLE);
  const busyRef = useRef(false);
  const launchStarted = useRef(false);
  const ackWaiter = useRef<(() => void) | null>(null);
  /** Recent attempts the launch queue has not sent yet while it waits for an acknowledgement. */
  const queuedRest = useRef<readonly SavedAttempt[]>([]);
  /** Set when a press took over the launch queue: the loop must send nothing more and leave the guard alone. */
  const queueTakenOver = useRef(false);

  const report = useCallback((next: AttemptState) => {
    stateRef.current = next;
    setState(next);
  }, []);

  const deps = useMemo<MachineDeps>(
    () => ({ save: saveAttempt, remove: removeAttempt, markResolved, send: sendAttempt, wait, report }),
    [report],
  );

  /** Runs one attempt to its settled state. The caller has already taken the busy guard. */
  const settle = useCallback(
    async (run: () => Promise<AttemptState>): Promise<AttemptState> => {
      try {
        const final = await run();
        await refreshAfter(final, client);
        report(final);
        return final;
      } finally {
        busyRef.current = false;
      }
    },
    [client, report],
  );

  /** The synchronous guard: set in the handler, before anything awaits. */
  const begin = useCallback((): boolean => {
    if (busyRef.current || stateRef.current.phase === 'waiting') return false;
    busyRef.current = true;
    return true;
  }, []);

  const press = useCallback(
    (kind: AttemptKind, amount: number) => {
      const release = ackWaiter.current;
      if (release !== null) {
        // The launch queue waits for the user to see an answer; a new press ends the wait. The unsent attempts become
        // cards (keys kept) before the queue is released, so the loop cannot resend them behind the press.
        const rest = queuedRest.current;
        queuedRest.current = [];
        ackWaiter.current = null;
        queueTakenOver.current = true;
        setUnconfirmed((list) => [...list, ...rest]);
        busyRef.current = false; // the press below takes the guard itself, before anything awaits
        release();
      }
      if (!begin()) return;
      const attempt: SavedAttempt = {
        user_id: user,
        kind,
        amount,
        key: randomUUID(),
        created_at: new Date(Date.now()).toISOString(),
      };
      void settle(() => runPress(attempt, deps));
    },
    [begin, deps, settle, user],
  );

  const checkAgain = useCallback(() => {
    const current = stateRef.current;
    if (busyRef.current || current.phase !== 'waiting') return;
    busyRef.current = true;
    void settle(() => check(current.attempt, deps, true));
  }, [deps, settle]);

  const checkNow = useCallback(
    (key: string) => {
      const attempt = unconfirmed.find((a) => a.key === key);
      if (attempt === undefined || !begin()) return;
      setUnconfirmed((list) => list.filter((a) => a.key !== key));
      void settle(() => check(attempt, deps, true));
    },
    [begin, deps, settle, unconfirmed],
  );

  const dismiss = useCallback(
    async (key: string) => {
      // Only a card can be dismissed: the attempt in flight is never one, so it cannot be removed from under the machine.
      if (!unconfirmed.some((a) => a.key === key)) return;
      setUnconfirmed((list) => list.filter((a) => a.key !== key));
      await removeAttempt(key).catch(() => undefined); // still saved at worst: shown again at the next launch
    },
    [unconfirmed],
  );

  const acknowledge = useCallback(
    (only?: 'done' | 'rejected') => {
      const phase = stateRef.current.phase;
      if (phase !== 'done' && phase !== 'rejected') return;
      if (only !== undefined && only !== phase) return;
      report(IDLE);
      ackWaiter.current?.();
      ackWaiter.current = null;
    },
    [report],
  );

  useEffect(() => {
    if (launchStarted.current) return;
    launchStarted.current = true;
    busyRef.current = true;
    void (async () => {
      const saved = await loadAttempts().catch((): SavedAttempt[] => []);
      const { recent, unconfirmed: old } = splitByAge(saved, Date.now());
      setUnconfirmed(old);
      setLaunchChecked(true);
      for (const [i, attempt] of recent.entries()) {
        const final = await settle(() => check(attempt, deps, true));
        busyRef.current = true; // settle released it; the launch check still holds it until the last attempt
        if (final.phase === 'waiting') {
          // Still unknown: the rest are offered as cards instead of being sent behind an unanswered one.
          setUnconfirmed((list) => [...list, ...recent.slice(i + 1)]);
          break;
        }
        if (i < recent.length - 1) {
          // F5: the user sees this answer before the next attempt is sent.
          queuedRest.current = recent.slice(i + 1);
          await new Promise<void>((resolve) => {
            ackWaiter.current = resolve;
          });
          if (queueTakenOver.current) return; // a press owns the guard now, and the rest are cards
        }
      }
      busyRef.current = false;
    })();
  }, [deps, settle]);

  const value = useMemo<AttemptContextValue>(
    () => ({ state, launchChecked, unconfirmed, press, checkAgain, checkNow, dismiss, acknowledge }),
    [state, launchChecked, unconfirmed, press, checkAgain, checkNow, dismiss, acknowledge],
  );
  return <AttemptContext.Provider value={value}>{children}</AttemptContext.Provider>;
}

export function useAttempts(): AttemptContextValue {
  const value = useContext(AttemptContext);
  if (value === null) throw new Error('useAttempts must be used inside AttemptProvider');
  return value;
}
