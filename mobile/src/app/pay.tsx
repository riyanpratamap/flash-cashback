import { ScrollView, StyleSheet, Text, View } from 'react-native';

import { useCampaign, useCashback } from '@/api/hooks';
import { useAttemptActions } from '@/attempts/AttemptProvider';
import { useAmountForm } from '@/attempts/useAmountForm';
import { errorCopy, MAX_AMOUNT } from '@/copy/codes';
import { payInfoLine } from '@/copy/payInfo';
import { formatRp } from '@/money/format';
import { AmountInput } from '@/ui/AmountInput';
import { Button } from '@/ui/Button';
import { spacing } from '@/ui/theme';

/** Quick amounts of the wireframe (screen 2). They are conveniences, not rules. */
const CHIPS = [20_000, 50_000, 100_000] as const;

export default function Pay() {
  const { press } = useAttemptActions();
  const campaign = useCampaign();
  const cashback = useCashback();
  const { text, setText, amount, inFlight, rejection: rejected } = useAmountForm('payment');
  const tooLarge = amount !== null && amount > MAX_AMOUNT;
  const canPay = amount !== null && amount > 0 && !tooLarge && !inFlight;
  const rejection = rejected === null ? null : errorCopy(rejected.code, {});
  const error = tooLarge ? errorCopy('INVALID_AMOUNT', {}) : rejection;

  const rules = campaign.data?.rules;
  const info =
    amount !== null &&
    amount > 0 &&
    !tooLarge &&
    rules !== undefined &&
    campaign.data !== undefined &&
    cashback.data !== undefined
      ? payInfoLine(amount, campaign.data.status, rules, cashback.data.today.remaining)
      : null;

  return (
    <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
      <AmountInput label="Amount (IDR)" value={text} onChangeText={setText} />
      {error === null ? null : <Text accessibilityRole="alert">{error}</Text>}
      {rules === undefined ? null : <Text>Payments under {formatRp(rules.min_payment)} earn no cashback.</Text>}
      <View style={styles.chips}>
        {CHIPS.map((chip) => (
          <Button key={chip} label={formatRp(chip)} variant="secondary" onPress={() => setText(String(chip))} />
        ))}
      </View>
      {info === null ? null : <Text>{info}</Text>}
      <Button
        label={canPay ? `Pay ${formatRp(amount)}` : 'Pay'}
        disabled={!canPay}
        onPress={() => {
          if (canPay) press('payment', amount);
        }}
      />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  content: { padding: spacing.lg, gap: spacing.md },
  chips: { flexDirection: 'row', gap: spacing.sm, flexWrap: 'wrap' },
});
