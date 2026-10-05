import { StyleSheet, Text, View } from 'react-native';

import type { SavedAttempt } from '../attempts/types';
import { formatDeviceStamp } from '../copy/stamp';
import { formatRp } from '../money/format';
import { Button } from './Button';

type Props = { attempt: SavedAttempt; onCheckNow: () => void; onDismiss: () => void };

/** A saved attempt too old to resend on its own (D48, wireframe screen 4). Nothing is sent until the user chooses. */
export function UnconfirmedCard({ attempt, onCheckNow, onDismiss }: Props) {
  const noun = attempt.kind === 'payment' ? 'payment' : 'redemption';
  return (
    <View style={styles.card}>
      <Text>
        {`A ${noun} of ${formatRp(attempt.amount)} from ${formatDeviceStamp(attempt.created_at)} wasn't confirmed.`}
      </Text>
      <View style={styles.row}>
        <Button label="Check now" onPress={onCheckNow} />
        <Button label="Dismiss" variant="secondary" onPress={onDismiss} />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  card: { padding: 16, gap: 8, borderRadius: 8, borderWidth: 1, borderColor: '#CCCCCC' },
  row: { flexDirection: 'row', gap: 8, flexWrap: 'wrap' },
});
