import { type ReactNode, useEffect, useRef, useState } from 'react';
import { Keyboard, KeyboardAvoidingView, Platform, ScrollView, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { colors, layout, spacing } from '@/ui/theme';

/**
 * A scrolling form with its action pinned at the bottom, above the keyboard and the home indicator. The offset
 * the keyboard view needs is the distance from the top of the window to this screen (header and, on Android with
 * edge-to-edge, the status bar), measured rather than assumed. The home-indicator inset is dropped while the keyboard
 * covers it.
 */
export function FormScreen({ children, footer }: { children: ReactNode; footer: ReactNode }) {
  const frame = useRef<View>(null);
  const [offset, setOffset] = useState(0);
  const [keyboardShown, setKeyboardShown] = useState(false);
  useEffect(() => {
    const ios = Platform.OS === 'ios';
    const show = Keyboard.addListener(ios ? 'keyboardWillShow' : 'keyboardDidShow', () => setKeyboardShown(true));
    const hide = Keyboard.addListener(ios ? 'keyboardWillHide' : 'keyboardDidHide', () => setKeyboardShown(false));
    return () => {
      show.remove();
      hide.remove();
    };
  }, []);
  return (
    <View
      ref={frame}
      style={styles.fill}
      onLayout={() => frame.current?.measureInWindow((_x, y) => setOffset(y))}
    >
      <KeyboardAvoidingView
        style={styles.fill}
        behavior="padding"
        keyboardVerticalOffset={offset}
      >
        <ScrollView style={styles.fill} contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
          {children}
        </ScrollView>
        <SafeAreaView edges={keyboardShown ? [] : ['bottom']} style={styles.footer}>
          {footer}
        </SafeAreaView>
      </KeyboardAvoidingView>
    </View>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1 },
  content: { padding: layout.margin, gap: layout.section },
  footer: {
    paddingHorizontal: layout.margin,
    paddingTop: spacing.md,
    paddingBottom: spacing.md,
    gap: spacing.sm,
    backgroundColor: colors.background,
  },
});
