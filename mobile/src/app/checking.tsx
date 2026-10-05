import { useEffect } from 'react';
import { ActivityIndicator, BackHandler, StyleSheet, Text, View } from 'react-native';

import { useAttempts } from '../attempts/AttemptProvider';
import { blocksLeaving } from '../attempts/blockBack';
import { formatRp } from '../money/format';
import { Button } from '../ui/Button';

/**
 * Screen 4: an unknown outcome. The attempt provider resends the same key; this screen only shows it. The wording
 * never says "failed" (AC-60). Leaving is blocked while the outcome is unknown or a request is on its way.
 */
export default function Checking() {
  const { state, checkAgain } = useAttempts();
  const phase = state.phase;

  useEffect(() => {
    const subscription = BackHandler.addEventListener('hardwareBackPress', () => blocksLeaving(phase));
    return () => subscription.remove();
  }, [phase]);

  if (state.phase !== 'checking' && state.phase !== 'waiting') return null;
  const noun = state.attempt.kind === 'payment' ? 'payment' : 'redemption';
  const verb = state.attempt.kind === 'payment' ? 'pay' : 'redeem';

  return (
    <View style={styles.content}>
      {state.phase === 'checking' ? <ActivityIndicator testID="checking-spinner" size="large" /> : null}
      <Text accessibilityRole="header" style={styles.title}>
        Checking your {noun}...
      </Text>
      <Text style={styles.amount}>{formatRp(state.attempt.amount)}</Text>
      <Text style={styles.text}>
        {`This is taking longer than usual. Please don't ${verb} again. This screen updates as soon as we have the result.`}
      </Text>
      {state.phase === 'waiting' ? <Button label="Check again" onPress={checkAgain} /> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  content: { padding: 16, gap: 12, alignItems: 'stretch' },
  title: { fontSize: 22, fontWeight: '700', textAlign: 'center' },
  amount: { fontSize: 28, fontWeight: '700', textAlign: 'center' },
  text: { textAlign: 'center' },
});
