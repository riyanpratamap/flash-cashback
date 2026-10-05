import { type UseQueryResult, useQuery } from '@tanstack/react-query';
import { useFocusEffect } from 'expo-router';
import { useCallback, useEffect, useRef } from 'react';

import { useUser } from '@/user/UserProvider';
import { campaignQuery, cashbackQuery, historyQuery, queryKeys } from '@/api/queries';

export const HOME_ACTIVITY_LIMIT = 2;
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

type Refreshable = Pick<UseQueryResult, 'refetch' | 'isFetching' | 'dataUpdatedAt'>;

/**
 * Returns `refetchAll` (pull to refresh, the retry buttons). Each time the screen regains focus, not on the first
 * focus (the queries already load on mount), it refetches the queries that are not fetching and were not updated in
 * the last FOCUS_FRESH_MS; a query in error has `dataUpdatedAt` 0, so it counts as old.
 * An effect that survives: it synchronises with navigation focus.
 */
export function useRefreshOnFocus(...queries: Refreshable[]) {
  // Focus fires from navigation, not a render: it must read the latest state, not the render it was created in.
  const latest = useRef(queries);
  useEffect(() => {
    latest.current = queries;
  });
  const first = useRef(true);
  useFocusEffect(
    useCallback(() => {
      if (first.current) {
        first.current = false;
        return;
      }
      const now = Date.now();
      for (const q of latest.current) {
        if (!q.isFetching && now - q.dataUpdatedAt >= FOCUS_FRESH_MS) void q.refetch();
      }
    }, []),
  );
  return () => Promise.all(latest.current.map((q) => q.refetch()));
}
