import { ScrollView, StyleSheet, View } from 'react-native';

import { HISTORY_LIMIT, useCampaign, useCashback, useHistory, useRefreshOnFocus } from '@/api/hooks';
import { queryKeys } from '@/api/queries';
import { groupByDay, timeOf } from '@/history/group';
import { formatRp } from '@/money/format';
import { ActivityRow, activityDetail, chipOf } from '@/ui/ActivityRow';
import { AppText } from '@/ui/AppText';
import { Card } from '@/ui/Card';
import { LoadError } from '@/ui/LoadError';
import { layout, spacing } from '@/ui/theme';
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
      <View style={styles.header}>
        <AppText variant="subhead" tone="muted">
          Cashback balance
        </AppText>
        {cashback.data === undefined ? null : (
          <AppText variant="title" tabular>
            {formatRp(cashback.data.balance)}
          </AppText>
        )}
      </View>
      {history.data === undefined ? (
        history.isError ? (
          <LoadError message="Couldn't load your history." onRetry={() => void refetchAll()} />
        ) : (
          <AppText tone="muted">Loading…</AppText>
        )
      ) : groups.length === 0 ? (
        <AppText tone="muted">No activity yet. Make a payment to start earning cashback.</AppText>
      ) : (
        groups.map((group) => (
          <View key={group.day} style={styles.group}>
            <AppText variant="caption" tone="muted" accessibilityRole="header" style={styles.day}>
              {group.label}
            </AppText>
            <Card style={styles.list}>
              {group.items.map((item, index) => {
                const detail = activityDetail(item, timeOf(item.created_at), chipOf(item, rules));
                return (
                  <ActivityRow
                    key={`${item.type}-${item.id}`}
                    item={item}
                    detail={detail}
                    last={index === group.items.length - 1}
                  />
                );
              })}
            </Card>
          </View>
        ))
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  content: { padding: layout.margin, gap: layout.section },
  header: { gap: spacing.xs },
  group: { gap: spacing.sm },
  day: { textTransform: 'uppercase', letterSpacing: 0.5 },
  list: { paddingVertical: spacing.xs, gap: 0 },
});
