import { StyleSheet, View } from 'react-native';

import { AppText } from '@/ui/AppText';
import { Button } from '@/ui/Button';
import { colors, radius, spacing } from '@/ui/theme';

export function LoadError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <View style={styles.box} accessibilityRole="alert">
      <AppText>{message}</AppText>
      <Button label="Try again" onPress={onRetry} />
    </View>
  );
}

const styles = StyleSheet.create({
  box: { padding: spacing.lg, gap: spacing.md, borderRadius: radius.md, backgroundColor: colors.surface },
});
