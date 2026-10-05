import { StyleSheet, Text, TextInput } from 'react-native';

import { colors, radius, spacing } from '@/ui/theme';

type Props = { label: string; value: string; onChangeText: (next: string) => void };

/** A label above a number-pad field, for the IDR amount of Pay and Redeem. The caller formats what is typed. */
export function AmountInput({ label, value, onChangeText }: Props) {
  return (
    <>
      <Text>{label}</Text>
      <TextInput
        accessibilityLabel={label}
        style={styles.input}
        keyboardType="number-pad"
        value={value}
        onChangeText={onChangeText}
      />
    </>
  );
}

const styles = StyleSheet.create({
  input: {
    borderWidth: 1,
    borderColor: colors.inputBorder,
    borderRadius: radius.md,
    padding: spacing.md,
    fontSize: 20,
  },
});
