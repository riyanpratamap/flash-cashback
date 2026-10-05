import { useRouter } from 'expo-router';
import { useEffect } from 'react';
import { ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { useCampaign } from '@/api/hooks';
import { parsePaymentResult } from '@/api/payments';
import { useAttempts } from '@/attempts/AttemptProvider';
import { reasonCopy, zoneLabel } from '@/copy/codes';
import { formatStamp } from '@/copy/stamp';
import { formatRp, formatSigned } from '@/money/format';
import { Button, LinkText } from '@/ui/Button';

/** Screen 3. The payment always shows as successful; the cashback is a separate card (wireframe). */
export default function PaymentResult() {
  const router = useRouter();
  const { state, acknowledge } = useAttempts();
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
        <Text accessibilityRole="header" style={styles.title}>
          Payment successful
        </Text>
        <Text style={styles.amount}>{formatRp(result?.amount ?? state.attempt.amount)}</Text>
        {result === null ? null : (
          <Text>
            Ref. {result.reference} · {formatStamp(result.createdAt)}
            {rules === undefined ? '' : ` ${zoneLabel(rules.timezone)}`}
          </Text>
        )}
        {result === null ? null : (
          <View style={styles.card}>
            <View style={styles.row}>
              <Text style={styles.section}>Cashback earned</Text>
              <Text style={styles.section}>{formatSigned(result.awarded, 'earned')}</Text>
            </View>
            {copy === null ? null : (
              <>
                {copy.chip === null ? null : <Text style={styles.chip}>{copy.chip}</Text>}
                <Text>{copy.text}</Text>
                {copy.howItWorks ? <LinkText label="How it works" onPress={() => router.push('/how-it-works')} /> : null}
              </>
            )}
          </View>
        )}
        <Button label="Done" onPress={done} />
        <Button label="Make another payment" variant="secondary" onPress={another} />
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1 },
  content: { padding: 16, gap: 12, alignItems: 'stretch' },
  title: { fontSize: 22, fontWeight: '700', textAlign: 'center' },
  amount: { fontSize: 28, fontWeight: '700', textAlign: 'center' },
  card: { padding: 16, gap: 8, borderRadius: 8, borderWidth: 1, borderColor: '#CCCCCC' },
  row: { flexDirection: 'row', justifyContent: 'space-between' },
  section: { fontSize: 16, fontWeight: '600' },
  chip: { alignSelf: 'flex-start', borderWidth: 1, borderRadius: 4, paddingHorizontal: 6, paddingVertical: 2, fontSize: 12 },
});
