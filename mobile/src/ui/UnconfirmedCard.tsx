import { StyleSheet, Text, View } from 'react-native';

import type { SavedAttempt } from '@/attempts/types';
import { formatDeviceStamp } from '@/copy/stamp';
import { formatRp } from '@/money/format';
import { Button } from '@/ui/Button';
import { Card } from '@/ui/Card';
import { spacing } from '@/ui/theme';

type Props = { attempt: SavedAttempt; onCheckNow: () => void; onDismiss: () => void };

/** A saved attempt too old to resend on its own (D48, wireframe screen 4). Nothing is sent until the user chooses. */
export function UnconfirmedCard({ attempt, onCheckNow, onDismiss }: Props) {
  const noun = attempt.kind === 'payment' ? 'payment' : 'redemption';
  return (
    <Card>
      <Text>
        {`A ${noun} of ${formatRp(attempt.amount)} from ${formatDeviceStamp(attempt.created_at)} wasn't confirmed.`}
      </Text>
      <View style={styles.row}>
        <Button label="Check now" onPress={onCheckNow} />
        <Button label="Dismiss" variant="secondary" onPress={onDismiss} />
      </View>
    </Card>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', gap: spacing.sm, flexWrap: 'wrap' },
});
