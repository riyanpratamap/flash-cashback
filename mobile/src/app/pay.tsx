import { useEffect, useState } from 'react';
import { ScrollView, StyleSheet, Text, TextInput, View } from 'react-native';

import { useCampaign, useCashback } from '../api/hooks';
import { useAttempts } from '../attempts/AttemptProvider';
import { errorCopy, MAX_AMOUNT } from '../copy/codes';
import { payInfoLine } from '../copy/payInfo';
import { formatAsTyped, formatRp, parseDigits } from '../money/format';
import { Button } from '../ui/Button';

/** Quick amounts of the wireframe (screen 2). They are conveniences, not rules. */
const CHIPS = [20_000, 50_000, 100_000] as const;

export default function Pay() {
  const { state, press, acknowledge } = useAttempts();
  const campaign = useCampaign();
  const cashback = useCashback();
  // A rejection found at launch lands here with the amount it was for.
  const [text, setText] = useState(() =>
    state.phase === 'rejected' && state.attempt.kind === 'payment' ? formatAsTyped(String(state.attempt.amount)) : '',
  );

  // A rejection that arrives while Pay is open (a launch resend) fills in the amount it was for. Keyed on the state
  // object, adjusted during render: it acts once per new state and never overwrites what is typed afterwards.
  const [seen, setSeen] = useState(state);
  if (seen !== state) {
    setSeen(state);
    if (state.phase === 'rejected' && state.attempt.kind === 'payment') {
      setText(formatAsTyped(String(state.attempt.amount)));
    }
  }

  // Leaving the screen ends a rejection: a launch resend waiting behind it can carry on.
  useEffect(() => () => acknowledge('rejected'), [acknowledge]);

  const amount = parseDigits(text);
  const tooLarge = amount !== null && amount > MAX_AMOUNT;
  const inFlight = state.phase === 'saving' || state.phase === 'sending';
  const canPay = amount !== null && amount > 0 && !tooLarge && !inFlight;
  const rejection =
    state.phase === 'rejected' && state.attempt.kind === 'payment' && state.attempt.amount === amount
      ? errorCopy(state.code, {})
      : null;
  const error = tooLarge ? errorCopy('INVALID_AMOUNT', {}) : rejection;

  const rules = campaign.data?.rules;
  const info =
    amount !== null && amount > 0 && !tooLarge && rules !== undefined && campaign.data !== undefined && cashback.data !== undefined
      ? payInfoLine(amount, campaign.data.status, rules, cashback.data.today.remaining)
      : null;

  return (
    <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
      <Text>Amount (IDR)</Text>
      <TextInput
        accessibilityLabel="Amount (IDR)"
        style={styles.input}
        keyboardType="number-pad"
        value={text}
        onChangeText={(next) => setText(formatAsTyped(next))}
      />
      {error === null ? null : <Text accessibilityRole="alert">{error}</Text>}
      {rules === undefined ? null : <Text>Payments under {formatRp(rules.min_payment)} earn no cashback.</Text>}
      <View style={styles.chips}>
        {CHIPS.map((chip) => (
          <Button key={chip} label={formatRp(chip)} variant="secondary" onPress={() => setText(formatAsTyped(String(chip)))} />
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
  content: { padding: 16, gap: 12 },
  input: { borderWidth: 1, borderColor: '#999999', borderRadius: 8, padding: 12, fontSize: 20 },
  chips: { flexDirection: 'row', gap: 8, flexWrap: 'wrap' },
});
