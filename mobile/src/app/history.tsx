import { ActivityIndicator, ScrollView, SectionList, StyleSheet, View } from 'react-native';

import { HISTORY_LIMIT, useCampaign, useCashback, useHistoryPages, useRefreshOnFocus } from '@/api/hooks';
import { queryKeys } from '@/api/queries';
import { groupByDay, timeOf } from '@/history/group';
import { ActivityRow, activityDetail, chipOf } from '@/ui/ActivityRow';
import { AppText } from '@/ui/AppText';
import { LoadError } from '@/ui/LoadError';
import { colors, layout, radius, spacing } from '@/ui/theme';
import { useUser } from '@/user/UserProvider';

export default function History() {
  const { user } = useUser();
  const history = useHistoryPages();
  const cashback = useCashback();
  const campaign = useCampaign();

  const refetchAll = useRefreshOnFocus(queryKeys.historyPages(user), queryKeys.cashback(user), queryKeys.campaign());

  const rules = campaign.data?.rules;
  // Group after flattening, so a day that spans two pages gets one header.
  const sections = groupByDay(
    (history.data?.pages ?? []).flatMap((page) => page.items),
    cashback.data?.today.date ?? null,
  ).map((group) => ({ ...group, data: group.items }));

  if (history.data === undefined || sections.length === 0) {
    return (
      <ScrollView contentContainerStyle={styles.content}>
        {history.data === undefined ? (
          history.isError ? (
            <LoadError message="Couldn't load your history." onRetry={() => void refetchAll()} />
          ) : (
            <AppText tone="muted">Loading…</AppText>
          )
        ) : (
          <AppText tone="muted">No activity yet. Make a payment to start earning cashback.</AppText>
        )}
      </ScrollView>
    );
  }

  const loadMore = () => {
    if (history.hasNextPage && !history.isFetchingNextPage && !history.isFetchNextPageError) void history.fetchNextPage();
  };

  return (
    <SectionList
      testID="history-list"
      contentContainerStyle={styles.content}
      sections={sections}
      keyExtractor={(item) => `${item.type}-${item.id}`}
      stickySectionHeadersEnabled={false}
      initialNumToRender={HISTORY_LIMIT * 3}
      renderSectionHeader={({ section }) => (
        <AppText variant="caption" tone="muted" accessibilityRole="header" style={styles.day}>
          {section.label}
        </AppText>
      )}
      renderSectionFooter={() => <View style={styles.gap} />}
      renderItem={({ item, index, section }) => {
        const first = index === 0;
        const last = index === section.data.length - 1;
        return (
          <View style={[styles.surface, first && styles.first, last && styles.last]}>
            <ActivityRow item={item} detail={activityDetail(item, timeOf(item.created_at), chipOf(item, rules))} last={last} />
          </View>
        );
      }}
      onEndReached={loadMore}
      ListFooterComponent={
        history.isFetchNextPageError ? (
          <LoadError message="Couldn't load more." onRetry={() => void history.fetchNextPage()} />
        ) : history.isFetchingNextPage ? (
          <ActivityIndicator accessibilityLabel="Loading more" />
        ) : null
      }
    />
  );
}

const styles = StyleSheet.create({
  content: { padding: layout.margin },
  day: { textTransform: 'uppercase', letterSpacing: 0.5, marginBottom: spacing.sm },
  gap: { height: layout.section },
  surface: { paddingHorizontal: spacing.lg, backgroundColor: colors.surface },
  first: { paddingTop: spacing.xs, borderTopLeftRadius: radius.md, borderTopRightRadius: radius.md },
  last: { paddingBottom: spacing.xs, borderBottomLeftRadius: radius.md, borderBottomRightRadius: radius.md },
});
