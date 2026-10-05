import { request, type ApiResult } from '../api/client';
import type { SavedAttempt } from './types';

export const RESEND_INTERVAL_MS = 2000;
export const RESENDS_PER_ROUND = 3;
/** Shown when the attempt could not be saved, so nothing was sent. Copy for it comes from the generic fallback. */
export const LOCAL_ERROR = 'LOCAL_ERROR';

export type Rejection = { code: string; message: string; requestId: string };

/** The state of one money attempt (tech-spec §10). `done` and `rejected` are definite answers; the rest are not. */
export type AttemptState =
  | { phase: 'idle' }
  | { phase: 'saving'; attempt: SavedAttempt }
  | { phase: 'sending'; attempt: SavedAttempt }
  | { phase: 'checking'; attempt: SavedAttempt }
  | { phase: 'waiting'; attempt: SavedAttempt }
  | { phase: 'done'; attempt: SavedAttempt; body: unknown }
  | ({ phase: 'rejected'; attempt: SavedAttempt } & Rejection);

export type MachineDeps = {
  save: (attempt: SavedAttempt) => Promise<void>;
  remove: (key: string) => Promise<void>;
  /** Fallback after a 4xx when `remove` keeps failing: the launch check must never resend this key. */
  markResolved: (key: string) => Promise<void>;
  send: (attempt: SavedAttempt) => Promise<ApiResult>;
  wait: (ms: number) => Promise<void>;
  report: (state: AttemptState) => void;
};

/** The one request of an attempt. Always the saved user and the saved key, never the selected user. */
export function sendAttempt(attempt: SavedAttempt): Promise<ApiResult> {
  return request({
    method: 'POST',
    path: attempt.kind === 'payment' ? '/payments' : '/redemptions',
    user: attempt.user_id,
    idempotencyKey: attempt.key,
    body: { amount: attempt.amount },
  });
}

async function finish(attempt: SavedAttempt, result: ApiResult, deps: MachineDeps): Promise<AttemptState | null> {
  if (result.kind === 'unknown') return null;
  if (result.kind === 'ok') {
    // If the removal fails the attempt is resent at the next launch, and the server replays the stored answer for the
    // same key.
    await deps.remove(attempt.key).catch(() => undefined);
    return { phase: 'done', attempt, body: result.body };
  }
  // A 4xx created no row, so the key is still unused: a resend could succeed and move money the user saw rejected.
  // Remove twice, then fall back to a marker the launch check skips.
  const removed = await deps.remove(attempt.key).then(
    () => true,
    () => false,
  );
  if (!removed) {
    await deps.remove(attempt.key).catch(() => deps.markResolved(attempt.key)).catch(() => undefined);
  }
  return { phase: 'rejected', attempt, code: result.code, message: result.message, requestId: result.requestId };
}

/**
 * Checking: resend the same key up to three times, about 2 s apart. `immediate` sends the first resend at once (a
 * launch check or "Check again"); otherwise it waits first, because the original request has just failed.
 */
export async function check(attempt: SavedAttempt, deps: MachineDeps, immediate: boolean): Promise<AttemptState> {
  deps.report({ phase: 'checking', attempt });
  for (let i = 0; i < RESENDS_PER_ROUND; i++) {
    if (i > 0 || !immediate) await deps.wait(RESEND_INTERVAL_MS);
    const final = await finish(attempt, await deps.send(attempt), deps);
    if (final !== null) return final;
  }
  return { phase: 'waiting', attempt };
}

/** One press: save before send, send once, and on an unknown outcome move to checking. Returns the settled state. */
export async function runPress(attempt: SavedAttempt, deps: MachineDeps): Promise<AttemptState> {
  deps.report({ phase: 'saving', attempt });
  try {
    await deps.save(attempt);
  } catch {
    // The write may have landed before it failed; a saved attempt the user was told was not sent must not be resent.
    await deps.remove(attempt.key).catch(() => undefined);
    return { phase: 'rejected', attempt, code: LOCAL_ERROR, message: '', requestId: '' };
  }
  deps.report({ phase: 'sending', attempt });
  const final = await finish(attempt, await deps.send(attempt), deps);
  return final ?? check(attempt, deps, false);
}
