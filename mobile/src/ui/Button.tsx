import { Pressable, StyleSheet } from 'react-native';

import { AppText } from '@/ui/AppText';
import { colors, radius, spacing } from '@/ui/theme';

type ButtonProps = {
  label: string;
  onPress: () => void;
  disabled?: boolean;
  variant?: 'primary' | 'secondary';
  size?: 'regular' | 'small';
  selected?: boolean;
};

export function Button({ label, onPress, disabled = false, variant = 'primary', size = 'regular', selected }: ButtonProps) {
  const small = size === 'small';
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ disabled, selected }}
      disabled={disabled}
      onPress={onPress}
      hitSlop={small ? { top: spacing.sm, bottom: spacing.sm } : undefined}
      style={({ pressed }) => [
        styles.base,
        small ? styles.small : styles.regular,
        variant === 'primary' ? styles.primary : styles.secondary,
        pressed && (variant === 'primary' ? styles.primaryPressed : styles.secondaryPressed),
        disabled && styles.disabled,
      ]}
    >
      <AppText
        variant={small ? 'subhead' : 'headline'}
        tone={disabled ? 'muted' : variant === 'primary' ? 'onPrimary' : 'link'}
        style={small && styles.smallText}
      >
        {label}
      </AppText>
    </Pressable>
  );
}

export function LinkText({ label, onPress }: { label: string; onPress: () => void }) {
  return (
    <Pressable accessibilityRole="link" accessibilityLabel={label} onPress={onPress} style={styles.link}>
      <AppText variant="subhead" tone="link">
        {label}
      </AppText>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: { paddingHorizontal: spacing.lg, alignItems: 'center', justifyContent: 'center' },
  regular: { minHeight: 50, borderRadius: radius.md },
  small: { minHeight: 36, minWidth: 44, borderRadius: radius.pill, paddingHorizontal: spacing.md },
  smallText: { fontWeight: '600' },
  primary: { backgroundColor: colors.primaryStrong },
  primaryPressed: { backgroundColor: colors.primaryPressed },
  secondary: { backgroundColor: colors.primaryTint },
  secondaryPressed: { backgroundColor: colors.primaryTintPressed },
  disabled: { backgroundColor: colors.track },
  link: { minHeight: 44, minWidth: 44, justifyContent: 'center', alignSelf: 'flex-start' },
});
