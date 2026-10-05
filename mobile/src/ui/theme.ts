import type { TextStyle } from 'react-native';

/**
 * Design tokens: the only place a colour or a size is written down.
 *
 * Contrast (WCAG, text needs 4.5:1), measured on white / on background #F2F2F7:
 *   text #1C1C1E 17.01 / 15.25 · textMuted #5F5F66 6.33 / 5.68 · positive #1E7B34 5.33 / 4.78
 *   warning #8A5300 6.33 / 5.67 · danger #C4281C 5.73 / 5.13 · primaryStrong #1869B6 5.62 / 5.04
 *   white on primaryStrong 5.62 · white on primaryPressed #134F8C 8.32
 *   primaryStrong on primaryTint #E4F1FD 4.90 · on warningTint #FBEFD9 4.94 · on primaryTintPressed #D6E9FB 4.53 · textMuted on track #E5E5EA > 5
 * positiveTint #E4EFE7 is positive (#1E7B34) at 12% on white; positive on positiveTint 4.52. It is the SuccessMark disc and the
 * background of the payment result's cashback pill, whose text is positive (4.52); any text on it must be positive or
 * text (text on it is above 14).
 * #208AEF measures 3.53 on white, so it never carries text: it is the tint source, the progress bar and the focus ring.
 * primaryTint is #208AEF at 12% on white.
 */
export const colors = {
  background: '#F2F2F7',
  surface: '#FFFFFF',
  text: '#1C1C1E',
  textMuted: '#5F5F66',
  separator: '#D1D1D6',
  track: '#E5E5EA',
  primary: '#208AEF',
  primaryStrong: '#1869B6',
  primaryPressed: '#134F8C',
  primaryTint: '#E4F1FD',
  primaryTintPressed: '#D6E9FB',
  positive: '#1E7B34',
  positiveTint: '#E4EFE7',
  warning: '#8A5300',
  warningTint: '#FBEFD9',
  danger: '#C4281C',
} as const;

export const spacing = { xs: 4, sm: 8, md: 12, lg: 16, xl: 24, xxl: 32 } as const;

/** Screen margin 16, 24 between sections, 8 or 12 inside one. */
export const layout = { margin: spacing.lg, section: spacing.xl } as const;

export const radius = { md: 12, pill: 999 } as const;

export const type = {
  display: { fontSize: 34, lineHeight: 41, fontWeight: '700' },
  title: { fontSize: 22, lineHeight: 28, fontWeight: '700' },
  headline: { fontSize: 17, lineHeight: 22, fontWeight: '600' },
  body: { fontSize: 17, lineHeight: 22, fontWeight: '400' },
  subhead: { fontSize: 15, lineHeight: 20, fontWeight: '400' },
  caption: { fontSize: 13, lineHeight: 18, fontWeight: '400' },
} as const;

export type TypeVariant = keyof typeof type;

/** Digits of equal width, so amounts line up and do not jitter while typing. */
export const tabular: TextStyle = { fontVariant: ['tabular-nums'] };
