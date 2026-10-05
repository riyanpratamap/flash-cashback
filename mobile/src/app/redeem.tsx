import { useRouter } from 'expo-router';
import { useEffect, useState } from 'react';
import { ScrollView, StyleSheet, Text, View } from 'react-native';

import { useCampaign, useCashback } from '@/api/hooks';
import { parseRedemptionResult } from '@/api/redemptions';
import { useAttempts } from '@/attempts/AttemptProvider';
import { errorCopy } from '@/copy/codes';
import { formatAsTyped, formatRp, parseDigits } from '@/money/format';
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
  const { state, press, acknowledge } = useAttempts();
  const campaign = useCampaign();
  const cashback = useCashback();
  // A rejection found at launch lands here with the amount it was for.
  const [text, setText] = useState(() =>
    state.phase === 'rejected' && state.attempt.kind === 'redemption' ? formatAsTyped(String(state.attempt.amount)) : '',
  );

  // A rejection that arrives while Redeem is open (a launch resend) fills in the amount it was for, once per new state.
  const [seen, setSeen] = useState(state);
  if (seen !== state) {
    setSeen(state);
    if (state.phase === 'rejected' && state.attempt.kind === 'redemption') {
      setText(formatAsTyped(String(state.attempt.amount)));
    }
  }

  // Leaving by any route ends a definite answer: a launch resend waiting behind it can carry on.
  useEffect(
    () => () => {
      acknowledge('done');
      acknowledge('rejected');
    },
    [acknowledge],
  );

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
  const amount = parseDigits(text);
  const inFlight = state.phase === 'saving' || state.phase === 'sending';
  const canRedeem = amount !== null && amount > 0 && balance > 0 && !paused && !inFlight;
  const rejection =
    state.phase === 'rejected' && state.attempt.kind === 'redemption' && state.attempt.amount === amount
      ? errorCopy(state.code, { balance })
      : null;
  const error = paused ? PAUSED_LINE : rejection;

  return (
    <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
      <View style={styles.row}>
        <Text>Available to redeem</Text>
        <Text style={styles.strong}>{formatRp(balance)}</Text>
      </View>
      <AmountInput
        label="Amount to redeem (IDR)"
        value={text}
        onChangeText={(next) => setText(formatAsTyped(next))}
      />
      <View style={styles.row}>
        <Text>Up to {formatRp(balance)}</Text>
        <Button
          label="Redeem all"
          variant="secondary"
          disabled={balance === 0}
          onPress={() => setText(formatAsTyped(String(balance)))}
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
