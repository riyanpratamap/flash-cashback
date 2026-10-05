import { useState } from 'react';
import { StyleSheet, TextInput, View } from 'react-native';

import { AppText } from '@/ui/AppText';
import { colors, radius, spacing, tabular, type } from '@/ui/theme';

type Props = { label: string; value: string; onChangeText: (next: string) => void };

/** A label above a number-pad field, for the IDR amount of Pay and Redeem. The caller formats what is typed. */
export function AmountInput({ label, value, onChangeText }: Props) {
  const [focused, setFocused] = useState(false);
  return (
    <View style={styles.box}>
      <AppText variant="subhead" tone="muted">
        {label}
      </AppText>
      <TextInput
        accessibilityLabel={label}
        style={[styles.input, focused && styles.focused]}
        keyboardType="number-pad"
        value={value}
        onChangeText={onChangeText}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        selectionColor={colors.primary}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  box: { gap: spacing.sm },
  input: {
    ...type.title,
    ...tabular,
    color: colors.text,
    backgroundColor: colors.surface,
    borderWidth: 2,
    borderColor: colors.surface,
    borderRadius: radius.md,
    paddingHorizontal: spacing.lg,
    paddingVertical: spacing.md,
    minHeight: 56,
  },
  focused: { borderColor: colors.primary },
});
