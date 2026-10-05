import { StyleSheet, Text, View } from 'react-native';

import { Button } from '@/ui/Button';

export function LoadError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <View style={styles.box} accessibilityRole="alert">
      <Text style={styles.text}>{message}</Text>
      <Button label="Try again" onPress={onRetry} />
    </View>
  );
}

const styles = StyleSheet.create({
  box: { padding: 16, gap: 12, borderRadius: 8, borderWidth: 1, borderColor: '#999999' },
  text: { fontSize: 16 },
});
