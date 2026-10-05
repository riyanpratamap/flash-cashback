import { useRouter } from 'expo-router';
import { ScrollView, StyleSheet, Text, View } from 'react-native';

import { useCampaign, useCashback } from '@/api/hooks';
import { parseRedemptionResult } from '@/api/redemptions';
import { useAttemptState, useAttemptActions } from '@/attempts/AttemptProvider';
import { useAmountForm } from '@/attempts/useAmountForm';
import { errorCopy } from '@/copy/codes';
import { formatRp } from '@/money/format';
import { AmountInput } from '@/ui/AmountInput';
import { Button } from '@/ui/Button';
import { LoadError } from '@/ui/LoadError';
import { spacing } from '@/ui/theme';

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
      <ScrollView contentContainerStyle={styles.content}>
        <Text accessibilityRole="header" style={styles.title}>
          {result === null
            ? 'Your redemption went through.'
            : `${formatRp(result.amount)} sent to your main account. Your balance is now ${formatRp(result.balanceAfter)}.`}
        </Text>
        <Button label="Done" onPress={done} />
      </ScrollView>
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
    <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
      <View style={styles.row}>
        <Text>Available to redeem</Text>
        <Text style={styles.strong}>{formatRp(balance)}</Text>
      </View>
      <AmountInput label="Amount to redeem (IDR)" value={text} onChangeText={setText} />
      <View style={styles.row}>
        <Text>Up to {formatRp(balance)}</Text>
        <Button
          label="Redeem all"
          variant="secondary"
          disabled={balance === 0}
          onPress={() => setText(String(balance))}
        />
      </View>
      <View style={styles.row}>
        <Text>Sent to</Text>
        <Text style={styles.strong}>Main account</Text>
      </View>
      {error === null ? null : <Text accessibilityRole="alert">{error}</Text>}
      <Button
        label={amount !== null && amount > 0 ? `Redeem ${formatRp(amount)}` : 'Redeem'}
        disabled={!canRedeem}
        onPress={() => {
          if (canRedeem) press('redemption', amount);
        }}
      />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  content: { padding: spacing.lg, gap: spacing.md },
  title: { fontSize: 20, fontWeight: '700', textAlign: 'center' },
  strong: { fontWeight: '700' },
  row: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
});
