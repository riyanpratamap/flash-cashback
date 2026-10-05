import { Pressable, StyleSheet, Text } from 'react-native';

type ButtonProps = {
  label: string;
  onPress: () => void;
  disabled?: boolean;
  variant?: 'primary' | 'secondary';
  selected?: boolean;
};

export function Button({ label, onPress, disabled = false, variant = 'primary', selected }: ButtonProps) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ disabled, selected }}
      disabled={disabled}
      onPress={onPress}
      style={[styles.base, variant === 'primary' ? styles.primary : styles.secondary, disabled && styles.disabled]}
    >
      <Text style={[styles.text, variant === 'secondary' && styles.secondaryText]}>{label}</Text>
    </Pressable>
  );
}

export function LinkText({ label, onPress }: { label: string; onPress: () => void }) {
  return (
    <Pressable accessibilityRole="link" accessibilityLabel={label} onPress={onPress} style={styles.link}>
      <Text style={styles.linkText}>{label}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: { minHeight: 44, minWidth: 44, paddingHorizontal: 16, borderRadius: 8, alignItems: 'center', justifyContent: 'center' },
  primary: { backgroundColor: '#208AEF' },
  secondary: { backgroundColor: '#FFFFFF', borderWidth: 1, borderColor: '#208AEF' },
  disabled: { opacity: 0.4 },
  text: { color: '#FFFFFF', fontWeight: '600', fontSize: 16 },
  secondaryText: { color: '#208AEF' },
  link: { minHeight: 44, minWidth: 44, justifyContent: 'center' },
  linkText: { color: '#208AEF', fontSize: 14, textDecorationLine: 'underline' },
});
