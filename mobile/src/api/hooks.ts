import { type QueryKey, useQuery, useQueryClient } from '@tanstack/react-query';
import { useFocusEffect } from 'expo-router';
import { useCallback, useEffect, useRef } from 'react';

import { useUser } from '@/user/UserProvider';
import { campaignQuery, cashbackQuery, historyQuery, queryKeys } from '@/api/queries';

export const HOME_ACTIVITY_LIMIT = 5;
export const HISTORY_LIMIT = 20; // D08: the newest 20, no paging

export function useCampaign() {
  const { user } = useUser();
  return useQuery({ queryKey: queryKeys.campaign(), queryFn: () => campaignQuery(user) });
}

export function useCashback() {
  const { user } = useUser();
  return useQuery({ queryKey: queryKeys.cashback(user), queryFn: () => cashbackQuery(user) });
}

export function useHistory(limit: number) {
  const { user } = useUser();
  return useQuery({ queryKey: queryKeys.history(user, limit), queryFn: () => historyQuery(user, limit) });
}

/** A focus refetch skips a query updated within this window (a money answer has just refetched it). */
export const FOCUS_FRESH_MS = 2000;

/**
 * Returns `refetchAll` (pull to refresh, the retry buttons): it refetches every key. Each time the screen regains focus,
 * not on the first focus (the queries already load on mount), it refetches the keys that are not fetching and were not
 * updated in the last FOCUS_FRESH_MS; a query that never succeeded has `dataUpdatedAt` 0, so it counts as old.
 * The keys must be built with `queryKeys`, as the hooks do. The decision reads the live query cache, never a render's
 * result: a result read outside render is stale, and a tracked field read there would re-render the screen on every
 * fetch from then on.
 * An effect that survives: it synchronises with navigation focus.
 */
export function useRefreshOnFocus(...keys: QueryKey[]) {
  const client = useQueryClient();
  // Focus fires from navigation, not a render: it must read the latest keys (the user can change).
  const latest = useRef(keys);
  useEffect(() => {
    latest.current = keys;
  });
  const first = useRef(true);
  useFocusEffect(
    useCallback(() => {
      if (first.current) {
        first.current = false;
        return;
      }
      const now = Date.now();
      for (const queryKey of latest.current) {
        const state = client.getQueryState(queryKey);
        if (state !== undefined && state.fetchStatus !== 'fetching' && now - state.dataUpdatedAt >= FOCUS_FRESH_MS) {
          void client.refetchQueries({ queryKey, exact: true });
        }
      }
    }, [client]),
  );
  return () => Promise.all(latest.current.map((queryKey) => client.refetchQueries({ queryKey, exact: true })));
}
