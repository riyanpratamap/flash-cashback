import { StyleSheet, View } from 'react-native';

import { useCampaign, useCashback } from '@/api/hooks';
import { useAttemptActions } from '@/attempts/AttemptProvider';
import { useAmountForm } from '@/attempts/useAmountForm';
import { errorCopy, MAX_AMOUNT } from '@/copy/codes';
import { payInfoLine } from '@/copy/payInfo';
import { formatRp } from '@/money/format';
import { AmountInput } from '@/ui/AmountInput';
import { AppText } from '@/ui/AppText';
import { Button } from '@/ui/Button';
import { FormScreen } from '@/ui/FormScreen';
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
    <FormScreen
      footer={
        <Button
          label={canPay ? `Pay ${formatRp(amount)}` : 'Pay'}
          disabled={!canPay}
          onPress={() => {
            if (canPay) press('payment', amount);
          }}
        />
      }
    >
      <View style={styles.group}>
        <AmountInput label="Amount (IDR)" value={text} onChangeText={setText} />
        {error !== null ? (
          <AppText variant="subhead" tone="danger" accessibilityRole="alert">
            {error}
          </AppText>
        ) : info !== null ? (
          <AppText variant="subhead" tone="muted">
            {info}
          </AppText>
        ) : rules === undefined ? null : (
          <AppText variant="subhead" tone="muted">
            Payments under {formatRp(rules.min_payment)} earn no cashback.
          </AppText>
        )}
      </View>
      <View style={styles.chips}>
        {CHIPS.map((chip) => (
          <Button key={chip} label={formatRp(chip)} variant="secondary" size="small" onPress={() => setText(String(chip))} />
        ))}
      </View>
    </FormScreen>
  );
}

const styles = StyleSheet.create({
  group: { gap: spacing.sm },
  chips: { flexDirection: 'row', gap: spacing.sm, flexWrap: 'wrap' },
});
