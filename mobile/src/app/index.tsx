import { type Href, useRouter } from 'expo-router';
import { useCallback, useState } from 'react';
import { RefreshControl, ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { HOME_ACTIVITY_LIMIT, useCampaign, useCashback, useHistory, useRefetchOnFocus } from '@/api/hooks';
import type { Campaign, CashbackSummary } from '@/api/queries';
import { useAttempts } from '@/attempts/AttemptProvider';
import { bannerCopy, zoneLabel } from '@/copy/codes';
import { formatRp } from '@/money/format';
import { useUser } from '@/user/UserProvider';
import { DEMO_USERS, userLabel } from '@/user/users';
import { ActivityRow } from '@/ui/ActivityRow';
import { Button, LinkText } from '@/ui/Button';
import { LoadError } from '@/ui/LoadError';
import { UnconfirmedCard } from '@/ui/UnconfirmedCard';

const LOAD_ERROR = "Couldn't load your cashback. Your balance is safe. Check your connection and try again.";
const ACTIVITY_ERROR = "Couldn't load your recent activity.";
const HISTORY_HINT = 'Check your history before paying again.';
const HOLD_LINE = 'Redemption is on hold and your balance is safe.';

export default function Home() {
  const router = useRouter();
  const campaign = useCampaign();
  const cashback = useCashback();
  const activity = useHistory(HOME_ACTIVITY_LIMIT);
  const { unconfirmed, checkNow, dismiss } = useAttempts();
  const [refreshing, setRefreshing] = useState(false);
  const [dismissed, setDismissed] = useState(false);

  const { refetch: refetchCampaign } = campaign;
  const { refetch: refetchCashback } = cashback;
  const { refetch: refetchActivity } = activity;
  const refetchAll = useCallback(
    () => Promise.all([refetchCampaign(), refetchCashback(), refetchActivity()]),
    [refetchCampaign, refetchCashback, refetchActivity],
  );
  useRefetchOnFocus(refetchAll);

  const onRefresh = () => {
    setRefreshing(true);
    void refetchAll().finally(() => setRefreshing(false));
  };

  return (
    <SafeAreaView style={styles.screen}>
      <ScrollView
        testID="home-scroll"
        contentContainerStyle={styles.content}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} />}
      >
        <Text accessibilityRole="header" style={styles.heading}>
          Flash Cashback
        </Text>
        <UserSwitcher />
        {unconfirmed.map((attempt) => (
          <UnconfirmedCard
            key={attempt.key}
            attempt={attempt}
            onCheckNow={() => checkNow(attempt.key)}
            onDismiss={() => {
              void dismiss(attempt.key).then(() => setDismissed(true));
            }}
          />
        ))}
        {dismissed ? <Text>{HISTORY_HINT}</Text> : null}
        {campaign.data !== undefined && cashback.data !== undefined ? (
          <Loaded campaign={campaign.data} cashback={cashback.data} onNavigate={(href) => router.push(href)} />
        ) : campaign.isError || cashback.isError ? (
          <LoadError message={LOAD_ERROR} onRetry={() => void refetchAll()} />
        ) : (
          <Text>Loading…</Text>
        )}
        {campaign.data !== undefined && cashback.data !== undefined ? (
          <View style={styles.card}>
            <View style={styles.rowBetween}>
              <Text accessibilityRole="header" style={styles.section}>
                Recent activity
              </Text>
              <LinkText label="See all" onPress={() => router.push('/history')} />
            </View>
            {activity.data !== undefined ? (
              activity.data.items.length === 0 ? (
                <Text>No activity yet.</Text>
              ) : (
                activity.data.items.map((item) => <ActivityRow key={`${item.type}-${item.id}`} item={item} />)
              )
            ) : activity.isError ? (
              <Text accessibilityRole="alert">{ACTIVITY_ERROR}</Text>
            ) : (
              <Text>Loading…</Text>
            )}
          </View>
        ) : null}
      </ScrollView>
    </SafeAreaView>
  );
}

function UserSwitcher() {
  const { user, setUser } = useUser();
  return (
    <View style={styles.switcher}>
      <Text style={styles.demo}>DEMO</Text>
      {DEMO_USERS.map((candidate) => (
        <Button
          key={candidate}
          label={userLabel(candidate)}
          variant={candidate === user ? 'primary' : 'secondary'}
          selected={candidate === user}
          onPress={() => void setUser(candidate)}
        />
      ))}
    </View>
  );
}

type LoadedProps = { campaign: Campaign; cashback: CashbackSummary; onNavigate: (href: Href) => void };

function Loaded({ campaign, cashback, onNavigate }: LoadedProps) {
  const { rules } = campaign;
  const banner = bannerCopy(campaign.status, campaign.redemption_status, rules);
  const ended = campaign.status === 'ENDED';
  const redemptionPaused = campaign.redemption_status === 'PAUSED';
  const reset = `Resets at ${cashback.today.resets_at.slice(11, 16)} ${zoneLabel(rules.timezone)}.`;
  const earnedShare = Math.min(100, Math.floor((cashback.today.earned * 100) / Math.max(rules.daily_cap, 1)));

  return (
    <>
      <View style={styles.card}>
        {banner.title === null ? null : <Text style={styles.section}>{banner.title}</Text>}
        <Text>{banner.text}</Text>
        <LinkText label="How it works" onPress={() => onNavigate('/how-it-works')} />
      </View>

      <View style={styles.card}>
        <View style={styles.rowBetween}>
          <View>
            <Text>Cashback balance</Text>
            <Text style={styles.amount}>{formatRp(cashback.balance)}</Text>
          </View>
          <Button
            label="Redeem"
            disabled={cashback.balance === 0 || redemptionPaused}
            onPress={() => onNavigate('/redeem')}
          />
        </View>
        {redemptionPaused ? <Text>{HOLD_LINE}</Text> : null}
      </View>

      {ended ? null : (
        <View style={styles.card}>
          <View style={styles.rowBetween}>
            <Text style={styles.section}>Earned today</Text>
            <Text>
              {formatRp(cashback.today.earned)} / {formatRp(rules.daily_cap)}
            </Text>
          </View>
          <View
            accessible
            accessibilityRole="progressbar"
            accessibilityValue={{ min: 0, max: rules.daily_cap, now: cashback.today.earned }}
            style={styles.track}
          >
            <View style={[styles.fill, { width: `${earnedShare}%` }]} />
          </View>
          <Text>
            {cashback.today.remaining === 0
              ? `You've reached today's limit. ${reset}`
              : `${formatRp(cashback.today.remaining)} left to earn today. ${reset}`}
          </Text>
        </View>
      )}

      <Button label="Make a payment" variant={ended ? 'secondary' : 'primary'} onPress={() => onNavigate('/pay')} />
    </>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1 },
  content: { padding: 16, gap: 12 },
  heading: { fontSize: 24, fontWeight: '700' },
  switcher: { flexDirection: 'row', alignItems: 'center', gap: 8, flexWrap: 'wrap' },
  demo: { fontSize: 12, fontWeight: '700', borderWidth: 1, paddingHorizontal: 6, paddingVertical: 2, borderRadius: 4 },
  card: { padding: 16, gap: 8, borderRadius: 8, borderWidth: 1, borderColor: '#CCCCCC' },
  rowBetween: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' },
  section: { fontSize: 16, fontWeight: '600' },
  amount: { fontSize: 22, fontWeight: '700' },
  track: { height: 8, borderRadius: 4, backgroundColor: '#E5E5E5', overflow: 'hidden' },
  fill: { height: 8, backgroundColor: '#208AEF' },
});
