import { StyleSheet, Text, View } from 'react-native';

import { Button } from '@/ui/Button';
import { colors, fontSize, radius, spacing } from '@/ui/theme';

export function LoadError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <View style={styles.box} accessibilityRole="alert">
      <Text style={styles.text}>{message}</Text>
      <Button label="Try again" onPress={onRetry} />
    </View>
  );
}

const styles = StyleSheet.create({
  box: { padding: spacing.lg, gap: spacing.md, borderRadius: radius.md, borderWidth: 1, borderColor: colors.inputBorder },
  text: { fontSize: fontSize.body },
});
