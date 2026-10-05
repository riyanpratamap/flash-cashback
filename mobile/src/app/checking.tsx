import { useEffect } from 'react';
import { ActivityIndicator, BackHandler, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { useAttemptState, useAttemptActions } from '@/attempts/AttemptProvider';
import { blocksLeaving } from '@/attempts/blockBack';
import { formatRp } from '@/money/format';
import { AppText } from '@/ui/AppText';
import { Button } from '@/ui/Button';
import { colors, layout, spacing } from '@/ui/theme';

/**
 * Screen 4: an unknown outcome. The attempt provider resends the same key; this screen only shows it. The wording
 * never says "failed" (AC-60). Leaving is blocked while the outcome is unknown or a request is on its way.
 */
export default function Checking() {
  const state = useAttemptState();
  const { checkAgain } = useAttemptActions();
  const phase = state.phase;

  useEffect(() => {
    const subscription = BackHandler.addEventListener('hardwareBackPress', () => blocksLeaving(phase));
    return () => subscription.remove();
  }, [phase]);

  if (state.phase !== 'checking' && state.phase !== 'waiting') return null;
  const noun = state.attempt.kind === 'payment' ? 'payment' : 'redemption';
  const verb = state.attempt.kind === 'payment' ? 'pay' : 'redeem';

  return (
    <SafeAreaView style={styles.screen}>
      <View style={styles.center}>
        {state.phase === 'checking' ? <ActivityIndicator testID="checking-spinner" size="large" color={colors.primary} /> : null}
        <AppText variant="headline" accessibilityRole="header" style={styles.centered}>
          Checking your {noun}...
        </AppText>
        <AppText variant="title" tabular style={styles.centered}>
          {formatRp(state.attempt.amount)}
        </AppText>
        <AppText tone="muted" style={styles.centered}>
          {`This is taking longer than usual. Please don't ${verb} again. This screen updates as soon as we have the result.`}
        </AppText>
      </View>
      {state.phase === 'waiting' ? (
        <View style={styles.footer}>
          <Button label="Check again" onPress={checkAgain} />
        </View>
      ) : null}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1 },
  center: { flex: 1, padding: layout.margin, gap: spacing.md, alignItems: 'center', justifyContent: 'center' },
  centered: { textAlign: 'center' },
  footer: { padding: layout.margin },
});
