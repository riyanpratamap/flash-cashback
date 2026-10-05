import type { ReactNode } from 'react';
import { StyleSheet, View, type StyleProp, type ViewStyle } from 'react-native';

import { colors, radius, spacing } from '@/ui/theme';

/** A white surface for content that belongs together. `style` carries the variants that differ. */
export function Card({ children, style }: { children: ReactNode; style?: StyleProp<ViewStyle> }) {
  return <View style={[styles.card, style]}>{children}</View>;
}

const styles = StyleSheet.create({
  card: {
    padding: spacing.lg,
    gap: spacing.sm,
    borderRadius: radius.md,
    backgroundColor: colors.surface,
  },
});
