import { ScrollView, StyleSheet, View } from 'react-native';

import { useCampaign } from '@/api/hooks';
import { formatPercent, zoneLabel, type Rules } from '@/copy/codes';
import { formatRp } from '@/money/format';
import { AppText } from '@/ui/AppText';
import { LoadError } from '@/ui/LoadError';
import { layout, spacing } from '@/ui/theme';

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
          <AppText tone="muted">Loading…</AppText>
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
        <AppText variant="subhead" tone="muted">
          Pay {formatRp(EXAMPLE_PAYMENT)}
        </AppText>
        <AppText variant="title" tabular>
          earn {formatRp(exampleEarn(rules))}
        </AppText>
      </View>
      {sections.map((section) => (
        <View key={section.title} style={styles.section}>
          <AppText variant="headline" accessibilityRole="header">
            {section.title}
          </AppText>
          <AppText>{section.text}</AppText>
        </View>
      ))}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  content: { padding: layout.margin, gap: layout.section },
  example: { gap: spacing.xs },
  section: { gap: spacing.md },
});
