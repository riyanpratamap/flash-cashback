import { Text, type TextProps } from 'react-native';

import { colors, tabular, type, type TypeVariant } from '@/ui/theme';

export type Tone = 'text' | 'muted' | 'positive' | 'warning' | 'danger' | 'link' | 'onPrimary';

const toneColor: Record<Tone, string> = {
  text: colors.text,
  muted: colors.textMuted,
  positive: colors.positive,
  warning: colors.warning,
  danger: colors.danger,
  link: colors.primaryStrong,
  onPrimary: colors.surface,
};

type Props = TextProps & { variant?: TypeVariant; tone?: Tone; tabular?: boolean };

/** Every piece of text goes through here, so a screen never sets a font size. Other props pass straight to `Text`. */
export function AppText({ variant = 'body', tone = 'text', tabular: digits = false, style, ...rest }: Props) {
  return <Text {...rest} style={[type[variant], { color: toneColor[tone] }, digits && tabular, style]} />;
}
