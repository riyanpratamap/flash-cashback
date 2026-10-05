import { type Href, useRouter } from 'expo-router';
import { useState } from 'react';
import { RefreshControl, ScrollView, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { HOME_ACTIVITY_LIMIT, useCampaign, useCashback, useHistory, useRefreshOnFocus } from '@/api/hooks';
import { type Campaign, type CashbackSummary, queryKeys } from '@/api/queries';
import { useAttemptLaunch, useAttemptActions } from '@/attempts/AttemptProvider';
import { bannerCopy, zoneLabel } from '@/copy/codes';
import { formatRp } from '@/money/format';
import { useUser } from '@/user/UserProvider';
import { DEMO_USERS, userLabel } from '@/user/users';
import { ActivityRow } from '@/ui/ActivityRow';
import { AppText } from '@/ui/AppText';
import { Button, LinkText } from '@/ui/Button';
import { Card } from '@/ui/Card';
import { LoadError } from '@/ui/LoadError';
import { colors, layout, radius, spacing } from '@/ui/theme';
import { UnconfirmedCard } from '@/ui/UnconfirmedCard';

const LOAD_ERROR = "Couldn't load your cashback. Your balance is safe. Check your connection and try again.";
const ACTIVITY_ERROR = "Couldn't load your recent activity.";
const HISTORY_HINT = 'Check your history before paying again.';
const HOLD_LINE = 'Redemption is on hold and your balance is safe.';

export default function Home() {
  const router = useRouter();
  const { user } = useUser();
  const campaign = useCampaign();
  const cashback = useCashback();
  const activity = useHistory(HOME_ACTIVITY_LIMIT);
  const { unconfirmed } = useAttemptLaunch();
  const { checkNow, dismiss } = useAttemptActions();
  const [refreshing, setRefreshing] = useState(false);
  const [dismissed, setDismissed] = useState(false);

  const refetchAll = useRefreshOnFocus(queryKeys.campaign(), queryKeys.cashback(user), queryKeys.history(user, HOME_ACTIVITY_LIMIT));

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
        <View style={styles.top}>
          <AppText variant="title" accessibilityRole="header">
            Flash Cashback
          </AppText>
          <UserSwitcher />
        </View>
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
        {dismissed ? <AppText tone="muted">{HISTORY_HINT}</AppText> : null}
        {campaign.data !== undefined && cashback.data !== undefined ? (
          <Loaded campaign={campaign.data} cashback={cashback.data} onNavigate={(href) => router.push(href)} />
        ) : campaign.isError || cashback.isError ? (
          <LoadError message={LOAD_ERROR} onRetry={() => void refetchAll()} />
        ) : (
          <AppText tone="muted">Loading…</AppText>
        )}
        {campaign.data !== undefined && cashback.data !== undefined ? (
          <View style={styles.group}>
            <View style={styles.rowBetween}>
              <AppText variant="headline" accessibilityRole="header">
                Recent activity
              </AppText>
              <LinkText label="See all" onPress={() => router.push('/history')} />
            </View>
            <Card style={styles.list}>
              {activity.data !== undefined ? (
                activity.data.items.length === 0 ? (
                  <AppText tone="muted" style={styles.listNote}>No activity yet.</AppText>
                ) : (
                  activity.data.items.map((item, index, items) => (
                    <ActivityRow key={`${item.type}-${item.id}`} item={item} last={index === items.length - 1} />
                  ))
                )
              ) : activity.isError ? (
                <AppText accessibilityRole="alert" tone="danger" style={styles.listNote}>
                  {ACTIVITY_ERROR}
                </AppText>
              ) : (
                <AppText tone="muted" style={styles.listNote}>Loading…</AppText>
              )}
            </Card>
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
      <AppText variant="caption" tone="muted" style={styles.demo}>
        DEMO
      </AppText>
      {DEMO_USERS.map((candidate) => (
        <Button
          key={candidate}
          label={userLabel(candidate)}
          variant={candidate === user ? 'primary' : 'secondary'}
          size="small"
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
      <View style={[styles.banner, campaign.status === 'ACTIVE' ? styles.bannerInfo : styles.bannerWarning]}>
        {banner.title === null ? null : <AppText variant="headline">{banner.title}</AppText>}
        <AppText variant="subhead">{banner.text}</AppText>
        <LinkText label="How it works" onPress={() => onNavigate('/how-it-works')} />
      </View>

      <Card style={styles.hero}>
        <View style={styles.rowBetween}>
          <View style={styles.balance}>
            <AppText variant="subhead" tone="muted">
              Cashback balance
            </AppText>
            <AppText variant="display" tabular>
              {formatRp(cashback.balance)}
            </AppText>
          </View>
          <Button
            label="Redeem"
            disabled={cashback.balance === 0 || redemptionPaused}
            onPress={() => onNavigate('/redeem')}
          />
        </View>
        {redemptionPaused ? (
          <AppText variant="subhead" tone="warning">
            {HOLD_LINE}
          </AppText>
        ) : null}
      </Card>

      {ended ? null : (
        <View style={styles.group}>
          <View style={styles.rowBetween}>
            <AppText variant="headline">Earned today</AppText>
            <AppText variant="subhead" tone="muted" tabular>
              {formatRp(cashback.today.earned)} / {formatRp(rules.daily_cap)}
            </AppText>
          </View>
          <View
            accessible
            accessibilityRole="progressbar"
            accessibilityValue={{ min: 0, max: rules.daily_cap, now: cashback.today.earned }}
            style={styles.track}
          >
            <View style={[styles.fill, { width: `${earnedShare}%` }]} />
          </View>
          <AppText variant="caption" tone="muted">
            {cashback.today.remaining === 0
              ? `You've reached today's limit. ${reset}`
              : `${formatRp(cashback.today.remaining)} left to earn today. ${reset}`}
          </AppText>
        </View>
      )}

      <Button label="Make a payment" variant={ended ? 'secondary' : 'primary'} onPress={() => onNavigate('/pay')} />
    </>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1 },
  content: { padding: layout.margin, gap: layout.section },
  top: { gap: spacing.md },
  switcher: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm, flexWrap: 'wrap' },
  demo: { letterSpacing: 0.5 },
  rowBetween: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', gap: spacing.md },
  banner: { padding: spacing.md, gap: spacing.xs, borderRadius: radius.md },
  bannerInfo: { backgroundColor: colors.primaryTint },
  bannerWarning: { backgroundColor: colors.warningTint },
  hero: { gap: spacing.md },
  balance: { flexShrink: 1 },
  group: { gap: spacing.sm },
  list: { paddingVertical: spacing.xs, gap: 0 },
  listNote: { paddingVertical: spacing.md },
  track: { height: 6, borderRadius: radius.pill, backgroundColor: colors.track, overflow: 'hidden' },
  fill: { height: 6, backgroundColor: colors.primary },
});
