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
import { colors, layout, radius, spacing } from '@/ui/theme';

/** Screen 3. The payment always shows as successful; the cashback is the number that leads (wireframe). */
const DETAIL_LABELS = { amount: 'Amount', reference: 'Reference', time: 'Time' } as const;

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
        <AppText variant="title" accessibilityRole="header">
          Payment successful
        </AppText>
        {result === null ? (
          <AppText variant="display" tabular>
            {formatRp(state.attempt.amount)}
          </AppText>
        ) : (
          <View style={styles.hero}>
            <AppText variant="subhead" tone="muted">
              Cashback earned
            </AppText>
            <AppText variant="display" tone={result.awarded > 0 ? 'positive' : 'text'} tabular>
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
                <AppText>{copy.text}</AppText>
                {copy.howItWorks ? <LinkText label="How it works" onPress={() => router.push('/how-it-works')} /> : null}
              </>
            )}
          </View>
        )}
        {result === null ? null : (
          <Card style={styles.details}>
            <DetailRow label={DETAIL_LABELS.amount} value={formatRp(result.amount)} separated />
            <DetailRow label={DETAIL_LABELS.reference} value={result.reference} separated />
            <DetailRow
              label={DETAIL_LABELS.time}
              value={`${formatStamp(result.createdAt)}${rules === undefined ? '' : ` ${zoneLabel(rules.timezone)}`}`}
            />
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

function DetailRow({ label, value, separated = false }: { label: string; value: string; separated?: boolean }) {
  return (
    <View style={[styles.detailRow, separated && styles.separator]}>
      <AppText tone="muted">{label}</AppText>
      <AppText tabular style={styles.value}>
        {value}
      </AppText>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1 },
  content: { padding: layout.margin, gap: layout.section },
  hero: { gap: spacing.sm, alignItems: 'flex-start' },
  chip: { borderRadius: radius.pill, backgroundColor: colors.warningTint, paddingHorizontal: spacing.md, paddingVertical: spacing.xs },
  details: { paddingVertical: spacing.xs, gap: 0 },
  detailRow: { flexDirection: 'row', justifyContent: 'space-between', gap: spacing.lg, paddingVertical: spacing.md },
  separator: { borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: colors.separator },
  value: { flexShrink: 1, textAlign: 'right' },
  footer: { padding: layout.margin, gap: spacing.sm },
});
