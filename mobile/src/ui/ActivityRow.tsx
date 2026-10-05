import { StyleSheet, View } from 'react-native';

import type { HistoryItem } from '@/api/queries';
import { formatRp, formatSigned } from '@/money/format';
import { AppText } from '@/ui/AppText';
import { colors, spacing } from '@/ui/theme';

/** The title and amount of a history item (docs/ui-wireframe.md screens 1 and 6). Rp0 has no sign; redemptions use −. */
export function activityLine(item: HistoryItem): { title: string; amount: string } {
  if (item.type === 'REDEMPTION') {
    return { title: 'Redeemed to main account', amount: formatSigned(item.amount, 'redeemed') };
  }
  return { title: `Payment ${formatRp(item.amount)}`, amount: formatSigned(item.cashback.awarded, 'earned') };
}

type Props = { item: HistoryItem; detail?: string; last?: boolean };

/** A row of a list on a white surface. A hairline separates it from the next row; `last` drops it. */
export function ActivityRow({ item, detail, last = false }: Props) {
  const { title, amount } = activityLine(item);
  const earned = item.type === 'PAYMENT' && item.cashback.awarded > 0;
  return (
    <View style={[styles.row, !last && styles.separator]}>
      <View style={styles.left}>
        <AppText>{title}</AppText>
        {detail === undefined ? null : (
          <AppText variant="caption" tone="muted">
            {detail}
          </AppText>
        )}
      </View>
      <AppText variant="headline" tone={earned ? 'positive' : 'text'} tabular style={styles.amount}>
        {amount}
      </AppText>
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', gap: spacing.md, paddingVertical: spacing.md },
  separator: { borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: colors.separator },
  left: { flex: 1, gap: 2 },
  amount: { textAlign: 'right' },
});
