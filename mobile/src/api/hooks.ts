import { useQuery } from '@tanstack/react-query';
import { useFocusEffect } from 'expo-router';
import { useCallback, useRef } from 'react';

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

/**
 * Runs `refetch` each time the screen regains focus, not on the first focus (the queries already load on mount).
 * An effect that survives: it synchronises with navigation focus.
 */
export function useRefetchOnFocus(refetch: () => unknown) {
  const first = useRef(true);
  useFocusEffect(
    useCallback(() => {
      if (first.current) {
        first.current = false;
        return;
      }
      void refetch();
    }, [refetch]),
  );
}
