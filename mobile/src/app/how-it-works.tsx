import { ScrollView, StyleSheet, Text, View } from 'react-native';

import { useCampaign } from '@/api/hooks';
import { formatPercent, zoneLabel, type Rules } from '@/copy/codes';
import { formatRp } from '@/money/format';
import { LoadError } from '@/ui/LoadError';

const EXAMPLE_PAYMENT = 100_000;

/** The illustration at the top of the page: what the example payment earns at the rate, rounded down. */
function exampleEarn(rules: Rules): number {
  return Math.floor((EXAMPLE_PAYMENT * rules.rate_bps) / 10_000);
}

export default function HowItWorks() {
  const campaign = useCampaign();
  const { refetch } = campaign;

  if (campaign.data === undefined) {
    return (
      <View style={styles.content}>
        {campaign.isError ? (
          <LoadError message="Couldn't load the campaign rules." onRetry={() => void refetch()} />
        ) : (
          <Text>Loading…</Text>
        )}
      </View>
    );
  }

  const { rules } = campaign.data;
  const percent = formatPercent(rules.rate_bps);
  const min = formatRp(rules.min_payment);
  const sections: readonly { title: string; text: string }[] = [
    { title: `Up to ${percent} cashback`, text: `On every payment of ${min} or more. Payments under ${min} don't earn cashback.` },
    {
      title: `Up to ${formatRp(rules.daily_cap)} per day`,
      text: `The limit resets at 00:00 ${zoneLabel(rules.timezone)}. A payment that reaches the limit earns what is left of it, so it can earn less than ${percent}.`,
    },
    {
      title: 'While cashback lasts',
      text: `The campaign ends when all cashback has been claimed. The last payment may earn less than ${percent}. Payments after that still work, without cashback.`,
    },
    {
      title: 'Redeem anytime',
      text: 'Your balance stays redeemable after the campaign ends. Redeemed cashback goes to your main account.',
    },
    { title: 'Rounded down', text: 'Cashback is rounded down to the nearest rupiah.' },
  ];

  return (
    <ScrollView contentContainerStyle={styles.content}>
      <View style={styles.example}>
        <Text>Pay {formatRp(EXAMPLE_PAYMENT)}</Text>
        <Text style={styles.title}>earn {formatRp(exampleEarn(rules))}</Text>
      </View>
      {sections.map((section) => (
        <View key={section.title} style={styles.section}>
          <Text accessibilityRole="header" style={styles.title}>
            {section.title}
          </Text>
          <Text>{section.text}</Text>
        </View>
      ))}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  content: { padding: 16, gap: 16 },
  example: { flexDirection: 'row', justifyContent: 'space-between', padding: 16, borderRadius: 8, borderWidth: 1, borderColor: '#CCCCCC' },
  section: { gap: 4 },
  title: { fontSize: 16, fontWeight: '600' },
});
