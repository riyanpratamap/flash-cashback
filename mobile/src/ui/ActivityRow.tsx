import { StyleSheet, View } from 'react-native';

import type { Campaign, HistoryItem } from '@/api/queries';
import { reasonCopy } from '@/copy/codes';
import { formatSigned } from '@/money/format';
import { AppText } from '@/ui/AppText';
import { colors, spacing } from '@/ui/theme';

type Line = { title: string; summary: string; amount: string; positive: boolean };

/**
 * The row as a transaction (docs/ui-wireframe.md screens 1 and 6): the payment or redemption amount on the right,
 * the cashback or destination in the summary. Rp0 has no sign; only a redemption is positive.
 */
export function activityLine(item: HistoryItem): Line {
  if (item.type === 'REDEMPTION') {
    return { title: 'Cashback redeemed', summary: 'To main account', amount: formatSigned(item.amount, 'received'), positive: true };
  }
  const { awarded } = item.cashback;
  return {
    title: 'Payment',
    summary: awarded > 0 ? `${formatSigned(awarded, 'earned')} cashback` : 'No cashback',
    amount: formatSigned(item.amount, 'paid'),
    positive: false,
  };
}

/** The subtitle: payment "time · summary · chip", redemption "summary · time"; a null time or chip is left out. */
export function activityDetail(item: HistoryItem, time: string | null, chip: string | null): string {
  const { summary } = activityLine(item);
  const parts = item.type === 'REDEMPTION' ? [summary, time] : [time, summary, chip];
  return parts.filter((part) => part !== null).join(' · ');
}

/** The reason chip of a payment, or null when rules are not loaded, the item is a redemption, or the reason has none. */
export function chipOf(item: HistoryItem, rules: Campaign['rules'] | undefined): string | null {
  return item.type === 'PAYMENT' && rules !== undefined ? reasonCopy(item.cashback.reason, rules).chip : null;
}

type Props = { item: HistoryItem; detail: string; last?: boolean };

/** A row of a list on a white surface. A hairline separates it from the next row; `last` drops it. */
export function ActivityRow({ item, detail, last = false }: Props) {
  const { title, amount, positive } = activityLine(item);
  return (
    <View style={[styles.row, !last && styles.separator]}>
      <View style={styles.left}>
        <AppText>{title}</AppText>
        <AppText variant="caption" tone="muted">
          {detail}
        </AppText>
      </View>
      <AppText variant="headline" tone={positive ? 'positive' : 'text'} tabular style={styles.amount}>
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
