import { ScrollView, StyleSheet, Text, View } from 'react-native';

import { HISTORY_LIMIT, useCampaign, useCashback, useHistory, useRefreshOnFocus } from '@/api/hooks';
import { queryKeys } from '@/api/queries';
import { reasonCopy } from '@/copy/codes';
import { groupByDay, timeOf } from '@/history/group';
import { formatRp } from '@/money/format';
import { ActivityRow } from '@/ui/ActivityRow';
import { Card } from '@/ui/Card';
import { LoadError } from '@/ui/LoadError';
import { fontSize, spacing } from '@/ui/theme';
import { useUser } from '@/user/UserProvider';

export default function History() {
  const { user } = useUser();
  const history = useHistory(HISTORY_LIMIT);
  const cashback = useCashback();
  const campaign = useCampaign();

  const refetchAll = useRefreshOnFocus(queryKeys.history(user, HISTORY_LIMIT), queryKeys.cashback(user), queryKeys.campaign());

  const rules = campaign.data?.rules;
  // The newest 20 and stop, whatever the server sends (D08).
  const groups = history.data === undefined ? [] : groupByDay(history.data.items.slice(0, HISTORY_LIMIT), cashback.data?.today.date ?? null);

  return (
    <ScrollView contentContainerStyle={styles.content}>
      <Card style={styles.header}>
        <Text>Current balance</Text>
        {cashback.data === undefined ? null : <Text style={styles.balance}>{formatRp(cashback.data.balance)}</Text>}
      </Card>
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
  content: { padding: spacing.lg, gap: spacing.md },
  header: { gap: spacing.xs },
  balance: { fontSize: fontSize.title, fontWeight: '700' },
  day: { fontSize: 13, fontWeight: '700', marginTop: spacing.sm },
});
