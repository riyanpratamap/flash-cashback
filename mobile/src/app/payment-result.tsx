import { useRouter } from 'expo-router';
import { useEffect } from 'react';
import { ScrollView, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { useCampaign } from '@/api/hooks';
import { parsePaymentResult } from '@/api/payments';
import { useAttemptState, useAttemptActions } from '@/attempts/AttemptProvider';
import { reasonCopy, zoneLabel, type ReasonCopy } from '@/copy/codes';
import { formatStamp } from '@/copy/stamp';
import { formatRp } from '@/money/format';
import { AppText } from '@/ui/AppText';
import { Button, LinkText } from '@/ui/Button';
import { SuccessMark } from '@/ui/SuccessMark';
import { colors, layout, radius, spacing } from '@/ui/theme';

/** The one line of the cashback pill (wireframe screen 3): the amount, then the reason's chip when it has one. */
function pillText(awarded: number, copy: ReasonCopy | null): string {
  const head = awarded > 0 ? `+${formatRp(awarded)} cashback` : 'No cashback';
  return copy === null || copy.chip === null ? head : `${head} · ${copy.chip}`;
}

/** Screen 3. The payment always shows as successful and leads; the cashback is the bonus below it (D17, wireframe). */

export default function PaymentResult() {
  const router = useRouter();
  const state = useAttemptState();
  const { acknowledge } = useAttemptActions();
  const campaign = useCampaign();

  // Leaving by any route (Android back included) ends the result: a launch resend waiting behind it can carry on.
  useEffect(() => () => acknowledge('done'), [acknowledge]);

  if (state.phase !== 'done' || state.attempt.kind !== 'payment') return null;
  const result = parsePaymentResult(state.body);
  const rules = campaign.data?.rules;
  const copy = result === null || rules === undefined ? null : reasonCopy(result.reason);

  const done = () => {
    router.dismissTo('/');
    acknowledge();
  };
  const another = () => {
    router.dismissTo('/');
    router.push('/pay');
    acknowledge();
  };

  return (
    <SafeAreaView style={styles.screen}>
      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.top}>
          <SuccessMark />
          <AppText variant="title" accessibilityRole="header">
            Payment successful
          </AppText>
          <AppText variant="display" tabular>
            {formatRp(result === null ? state.attempt.amount : result.amount)}
          </AppText>
          {result === null ? null : (
            <View style={styles.caption}>
              <AppText variant="caption" tone="muted" tabular>
                {`${formatStamp(result.createdAt)}${rules === undefined ? '' : ` ${zoneLabel(rules.timezone)}`}`}
              </AppText>
              <AppText variant="caption" tone="muted" tabular>
                {result.reference}
              </AppText>
            </View>
          )}
        </View>
        {result === null ? null : (
          <View style={styles.cashback}>
            <View style={[styles.pill, { backgroundColor: result.awarded > 0 ? colors.positiveTint : colors.track }]}>
              <AppText variant="caption" tone={result.awarded > 0 ? 'positive' : 'muted'} tabular>
                {pillText(result.awarded, copy)}
              </AppText>
            </View>
            {copy?.howItWorks ? <LinkText label="How it works" onPress={() => router.push('/how-it-works')} /> : null}
          </View>
        )}
      </ScrollView>
      <View style={styles.footer}>
        <Button label="Done" onPress={done} />
        <Button label="Make another payment" variant="secondary" onPress={another} />
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1 },
  content: { flexGrow: 1, justifyContent: 'center', padding: layout.margin, gap: layout.section },
  top: { gap: spacing.sm, alignItems: 'center' },
  caption: { alignItems: 'center' },
  cashback: { alignItems: 'center', gap: spacing.sm },
  pill: { alignSelf: 'center', borderRadius: radius.pill, paddingHorizontal: spacing.md, paddingVertical: spacing.xs },
  footer: { padding: layout.margin, gap: spacing.sm },
});
