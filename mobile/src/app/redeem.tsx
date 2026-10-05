import { useRouter } from 'expo-router';
import { ScrollView, StyleSheet, View } from 'react-native';

import { useCampaign, useCashback } from '@/api/hooks';
import { parseRedemptionResult } from '@/api/redemptions';
import { useAttemptState, useAttemptActions } from '@/attempts/AttemptProvider';
import { useAmountForm } from '@/attempts/useAmountForm';
import { errorCopy } from '@/copy/codes';
import { formatRp } from '@/money/format';
import { AmountInput } from '@/ui/AmountInput';
import { AppText } from '@/ui/AppText';
import { Button } from '@/ui/Button';
import { FormScreen } from '@/ui/FormScreen';
import { LoadError } from '@/ui/LoadError';
import { SuccessMark } from '@/ui/SuccessMark';
import { colors, layout, radius, spacing } from '@/ui/theme';

const PAUSED_LINE = errorCopy('REDEMPTION_PAUSED', {});

/**
 * Screen 5: the form and, once the redemption is done, its confirmation (one route). The balance is refetched on open
 * (the query is stale by default); a value that did not load is never shown as zero.
 */
export default function Redeem() {
  const router = useRouter();
  const state = useAttemptState();
  const { press, acknowledge } = useAttemptActions();
  const campaign = useCampaign();
  const cashback = useCashback();
  const { text, setText, amount, inFlight, rejection: rejected } = useAmountForm('redemption');

  if (state.phase === 'done' && state.attempt.kind === 'redemption') {
    const result = parseRedemptionResult(state.body);
    const done = () => {
      router.dismissTo('/');
      acknowledge();
    };
    return (
      <FormScreen centred footer={<Button label="Done" onPress={done} />}>
        <View style={styles.hero}>
          <SuccessMark />
          {result === null ? (
            <>
              <AppText variant="title" accessibilityRole="header" style={styles.centred}>
                Your redemption went through.
              </AppText>
              <AppText variant="subhead" tone="muted" style={styles.centred}>
                Check your balance on the home screen.
              </AppText>
            </>
          ) : (
            <>
              <AppText variant="title" accessibilityRole="header" style={styles.centred}>
                Redemption successful
              </AppText>
              <AppText variant="display" tabular>
                {formatRp(result.amount)}
              </AppText>
            </>
          )}
        </View>
        {result === null ? null : (
          <View style={styles.details}>
            <View style={[styles.detailRow, styles.separator]}>
              <AppText tone="muted">Sent to</AppText>
              <AppText style={styles.value}>Main account</AppText>
            </View>
            <View style={[styles.detailRow, !state.replayed && styles.separator]}>
              <AppText tone="muted">Reference</AppText>
              <AppText tabular style={styles.value}>
                {result.reference}
              </AppText>
            </View>
            {state.replayed ? null : (
              <View style={styles.detailRow}>
                <AppText tone="muted">Cashback balance</AppText>
                <AppText tabular style={styles.value}>
                  {formatRp(result.balanceAfter)}
                </AppText>
              </View>
            )}
          </View>
        )}
      </FormScreen>
    );
  }

  if (cashback.data === undefined) {
    return (
      <ScrollView contentContainerStyle={styles.content}>
        {cashback.isError ? (
          <LoadError message="Couldn't load your balance." onRetry={() => void cashback.refetch()} />
        ) : null}
      </ScrollView>
    );
  }

  const balance = cashback.data.balance;
  const paused = campaign.data?.redemption_status === 'PAUSED';
  const canRedeem = amount !== null && amount > 0 && balance > 0 && !paused && !inFlight;
  const rejection = rejected === null ? null : errorCopy(rejected.code, { balance });
  const error = paused ? PAUSED_LINE : rejection;

  return (
    <FormScreen
      footer={
        <Button
          label={amount !== null && amount > 0 ? `Redeem ${formatRp(amount)}` : 'Redeem'}
          disabled={!canRedeem}
          onPress={() => {
            if (canRedeem) press('redemption', amount);
          }}
        />
      }
    >
      <View style={styles.group}>
        <AmountInput label="Amount to redeem (IDR)" value={text} onChangeText={setText} />
        {error === null ? null : (
          <AppText variant="subhead" tone="danger" accessibilityRole="alert">
            {error}
          </AppText>
        )}
        <View style={styles.row}>
          <AppText variant="subhead" tone="muted">
            Up to {formatRp(balance)}
          </AppText>
          <Button
            label="Redeem all"
            variant="secondary"
            size="small"
            disabled={balance === 0}
            onPress={() => setText(String(balance))}
          />
        </View>
      </View>
      <View style={styles.details}>
        <View style={[styles.detailRow, styles.separator]}>
          <AppText tone="muted">Available to redeem</AppText>
          <AppText variant="headline" tabular>
            {formatRp(balance)}
          </AppText>
        </View>
        <View style={styles.detailRow}>
          <AppText tone="muted">Sent to</AppText>
          <AppText variant="headline">Main account</AppText>
        </View>
      </View>
    </FormScreen>
  );
}

const styles = StyleSheet.create({
  content: { padding: layout.margin, gap: layout.section },
  group: { gap: spacing.sm },
  hero: { alignItems: 'center', gap: spacing.sm },
  centred: { textAlign: 'center' },
  value: { flexShrink: 1, textAlign: 'right' },
  details: { backgroundColor: colors.surface, borderRadius: radius.md, paddingHorizontal: spacing.lg },
  separator: { borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: colors.separator },
  row: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', gap: spacing.md },
  detailRow: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', gap: spacing.md, paddingVertical: spacing.md },
});
