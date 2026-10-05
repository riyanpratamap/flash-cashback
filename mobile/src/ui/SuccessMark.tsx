import { StyleSheet, View } from 'react-native';

import { colors } from '@/ui/theme';

const SIZE = 72;

/** A check in a soft green disc, drawn from Views (no image, icon or animation). Announced as "Success". */
export function SuccessMark() {
  return (
    <View accessibilityRole="image" accessibilityLabel="Success" style={styles.disc}>
      <View style={styles.check}>
        <View style={styles.short} />
        <View style={styles.long} />
      </View>
    </View>
  );
}

const BAR = 5;
const styles = StyleSheet.create({
  disc: {
    width: SIZE,
    height: SIZE,
    borderRadius: SIZE / 2,
    backgroundColor: colors.positiveTint,
    alignItems: 'center',
    justifyContent: 'center',
  },
  check: { width: 32, height: 24 },
  short: {
    position: 'absolute',
    left: 2,
    top: 12,
    width: 14,
    height: BAR,
    borderRadius: BAR / 2,
    backgroundColor: colors.positive,
    transform: [{ rotate: '45deg' }],
  },
  long: {
    position: 'absolute',
    left: 8,
    top: 10,
    width: 26,
    height: BAR,
    borderRadius: BAR / 2,
    backgroundColor: colors.positive,
    transform: [{ rotate: '-50deg' }],
  },
});
