import { useRouter } from 'expo-router';
import { useEffect } from 'react';
import { ScrollView, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { useCampaign } from '@/api/hooks';
import { parsePaymentResult } from '@/api/payments';
import { useAttemptState, useAttemptActions } from '@/attempts/AttemptProvider';
import { reasonCopy, zoneLabel } from '@/copy/codes';
import { formatStamp } from '@/copy/stamp';
import { formatRp, formatSigned } from '@/money/format';
import { AppText } from '@/ui/AppText';
import { Button, LinkText } from '@/ui/Button';
import { Card } from '@/ui/Card';
import { SuccessMark } from '@/ui/SuccessMark';
import { colors, layout, radius, spacing } from '@/ui/theme';

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
  const copy = result === null || rules === undefined ? null : reasonCopy(result.reason, rules);

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
          <Card>
            <AppText variant="subhead" tone="muted">
              Cashback earned
            </AppText>
            <AppText variant="headline" tone={result.awarded > 0 ? 'positive' : 'text'} tabular>
              {formatSigned(result.awarded, 'earned')}
            </AppText>
            {copy === null ? null : (
              <>
                {copy.chip === null ? null : (
                  <View style={styles.chip}>
                    <AppText variant="caption" tone="warning">
                      {copy.chip}
                    </AppText>
                  </View>
                )}
                <AppText variant="subhead">{copy.text}</AppText>
                {copy.howItWorks ? <LinkText label="How it works" onPress={() => router.push('/how-it-works')} /> : null}
              </>
            )}
          </Card>
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
  content: { padding: layout.margin, gap: layout.section },
  top: { gap: spacing.sm, alignItems: 'center' },
  caption: { alignItems: 'center' },
  chip: { alignSelf: 'flex-start', borderRadius: radius.pill, backgroundColor: colors.warningTint, paddingHorizontal: spacing.md, paddingVertical: spacing.xs },
  footer: { padding: layout.margin, gap: spacing.sm },
});
