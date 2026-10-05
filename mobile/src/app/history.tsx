import { useCallback } from 'react';
import { ScrollView, StyleSheet, Text, View } from 'react-native';

import { HISTORY_LIMIT, useCampaign, useCashback, useHistory, useRefetchOnFocus } from '../api/hooks';
import { reasonCopy } from '../copy/codes';
import { groupByDay, timeOf } from '../history/group';
import { formatRp } from '../money/format';
import { ActivityRow } from '../ui/ActivityRow';
import { LoadError } from '../ui/LoadError';

export default function History() {
  const history = useHistory(HISTORY_LIMIT);
  const cashback = useCashback();
  const campaign = useCampaign();

  const { refetch: refetchHistory } = history;
  const { refetch: refetchCashback } = cashback;
  const { refetch: refetchCampaign } = campaign;
  const refetchAll = useCallback(
    () => Promise.all([refetchHistory(), refetchCashback(), refetchCampaign()]),
    [refetchHistory, refetchCashback, refetchCampaign],
  );
  useRefetchOnFocus(refetchAll);

  const rules = campaign.data?.rules;
  // The newest 20 and stop, whatever the server sends (D08).
  const groups = history.data === undefined ? [] : groupByDay(history.data.items.slice(0, HISTORY_LIMIT), cashback.data?.today.date ?? null);

  return (
    <ScrollView contentContainerStyle={styles.content}>
      <View style={styles.header}>
        <Text>Current balance</Text>
        {cashback.data === undefined ? null : <Text style={styles.balance}>{formatRp(cashback.data.balance)}</Text>}
      </View>
      {history.data === undefined ? (
        history.isError ? (
          <LoadError message="Couldn't load your history." onRetry={() => void refetchAll()} />
        ) : (
          <Text>Loading…</Text>
        )
      ) : groups.length === 0 ? (
        <Text>No activity yet. Make a payment to start earning cashback.</Text>
      ) : (
        groups.map((group) => (
          <View key={group.day}>
            <Text accessibilityRole="header" style={styles.day}>
              {group.label}
            </Text>
            {group.items.map((item) => {
              const chip = item.type === 'PAYMENT' && rules !== undefined ? reasonCopy(item.cashback.reason, rules).chip : null;
              const detail = chip === null ? timeOf(item.created_at) : `${timeOf(item.created_at)} · ${chip}`;
              return <ActivityRow key={`${item.type}-${item.id}`} item={item} detail={detail} />;
            })}
          </View>
        ))
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  content: { padding: 16, gap: 12 },
  header: { padding: 16, borderRadius: 8, borderWidth: 1, borderColor: '#CCCCCC', gap: 4 },
  balance: { fontSize: 22, fontWeight: '700' },
  day: { fontSize: 13, fontWeight: '700', marginTop: 8 },
});
