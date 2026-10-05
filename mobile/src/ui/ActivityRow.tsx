import { StyleSheet, Text, View } from 'react-native';

import type { HistoryItem } from '@/api/queries';
import { formatRp, formatSigned } from '@/money/format';

/** The title and amount of a history item (docs/ui-wireframe.md screens 1 and 6). Rp0 has no sign; redemptions use −. */
export function activityLine(item: HistoryItem): { title: string; amount: string } {
  if (item.type === 'REDEMPTION') {
    return { title: 'Redeemed to main account', amount: formatSigned(item.amount, 'redeemed') };
  }
  return { title: `Payment ${formatRp(item.amount)}`, amount: formatSigned(item.cashback.awarded, 'earned') };
}

export function ActivityRow({ item, detail }: { item: HistoryItem; detail?: string }) {
  const { title, amount } = activityLine(item);
  return (
    <View style={styles.row}>
      <View style={styles.left}>
        <Text style={styles.title}>{title}</Text>
        {detail === undefined ? null : <Text style={styles.detail}>{detail}</Text>}
      </View>
      <Text style={styles.amount}>{amount}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', justifyContent: 'space-between', paddingVertical: 8 },
  left: { flex: 1 },
  title: { fontSize: 16 },
  detail: { fontSize: 13, color: '#555555' },
  amount: { fontSize: 16, fontWeight: '600' },
});
